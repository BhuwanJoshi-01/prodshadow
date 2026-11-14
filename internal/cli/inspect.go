package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/report"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/risk"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/sqlinspect"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/version"
)

func runInspect(_ context.Context, env *Env, args []string) int {
	fs := newFlags(env, "inspect", "--migration FILE [--migration FILE ...] [flags]")
	var files multiFlag
	fs.Var(&files, "migration", "migration file (repeatable, executed in order); - reads one migration from standard input")
	cfgPath := fs.String("config", "", "optional configuration for rule overrides and transaction mode")
	txn := fs.String("transaction", "", "transaction mode: auto, per-file or none (default from config, else auto)")
	format := fs.String("format", "text", "output format: text, json, markdown, sarif or junit")
	failOn := fs.String("fail-on", "", "exit 2 if an unsuppressed finding has at least this severity (info, warning, critical)")
	quiet := fs.Bool("quiet", false, "text format: print only findings, not the analysed statements")
	changedSince := fs.String("changed-since", "", "only migrations git reports as added or changed since this ref (e.g. origin/main)")
	if code := parseFlags(fs, args); code >= 0 {
		return code
	}

	opts := sqlinspect.Options{Transaction: sqlinspect.TxnModeAuto}
	base := env.Dir
	if *cfgPath != "" {
		p, err := loadProject(env, *cfgPath)
		if err != nil {
			fmt.Fprintf(env.Stderr, "prodshadow: %v\n", err)
			return ExitInvalid
		}
		if p.problems.HasErrors() {
			printProblems(env.Stderr, p.problems.Errors())
			return ExitInvalid
		}
		opts.Transaction = sqlinspect.TransactionMode(p.cfg.Migration.Transaction)
		opts.Overrides = p.cfg.Policy.Rules
		if len(files) == 0 {
			files = p.cfg.Migration.Files
			base = p.cfg.BaseDir()
		}
	}
	if *txn != "" {
		switch sqlinspect.TransactionMode(*txn) {
		case sqlinspect.TxnModeAuto, sqlinspect.TxnModePerFile, sqlinspect.TxnModeNone:
			opts.Transaction = sqlinspect.TransactionMode(*txn)
		default:
			fmt.Fprintf(env.Stderr, "prodshadow: invalid --transaction %q\n", *txn)
			return ExitInvalid
		}
	}
	threshold := 0
	var failAt evidence.Severity
	if *failOn != "" {
		sev, err := evidence.ParseSeverity(*failOn)
		if err != nil || sev == evidence.Inconclusive {
			fmt.Fprintf(env.Stderr, "prodshadow: invalid --fail-on %q\n", *failOn)
			return ExitInvalid
		}
		threshold, failAt = sev.Rank(), sev
	}

	if *changedSince != "" {
		changed, err := changedMigrations(base, files, *changedSince)
		if err != nil {
			fmt.Fprintf(env.Stderr, "prodshadow: --changed-since: %v\n", err)
			return ExitInvalid
		}
		if len(changed) == 0 {
			fmt.Fprintf(env.Stdout, "No migration changed since %s; nothing to inspect.\n", *changedSince)
			return ExitOK
		}
		files = changed
	}
	var stmts []*sqlinspect.Statement
	var err error
	if len(files) == 1 && files[0] == "-" {
		stmts, err = readStdinMigration(env)
	} else {
		stmts, err = loadMigrations(base, files)
	}
	if err != nil {
		fmt.Fprintf(env.Stderr, "prodshadow: %v\n", err)
		return ExitInvalid
	}
	findings := sqlinspect.Analyze(stmts, opts)

	switch *format {
	case "text":
		if *quiet {
			printFindingsText(env.Stdout, findings)
		} else {
			printInspectText(env.Stdout, stmts, findings)
		}
	case "markdown":
		printInspectMarkdown(env.Stdout, stmts, findings)
	case "sarif":
		fallback := ""
		if len(stmts) > 0 {
			fallback = stmts[0].File
		}
		out, err := report.SARIF(risk.FromStatic(findings), version.Get().Version, fallback)
		if err != nil {
			fmt.Fprintf(env.Stderr, "prodshadow: %v\n", err)
			return ExitInternal
		}
		if _, err := env.Stdout.Write(out); err != nil {
			return ExitInternal
		}
	case "junit":
		out, err := report.StaticJUnit(risk.FromStatic(findings), failAt)
		if err != nil {
			fmt.Fprintf(env.Stderr, "prodshadow: %v\n", err)
			return ExitInternal
		}
		if _, err := env.Stdout.Write(out); err != nil {
			return ExitInternal
		}
	case "json":
		out := struct {
			Parser     string                  `json:"parser_grammar"`
			Statements []*sqlinspect.Statement `json:"statements"`
			Findings   []sqlinspect.Finding    `json:"findings"`
		}{sqlinspect.ParserGrammar, stmts, findings}
		if out.Findings == nil {
			out.Findings = []sqlinspect.Finding{}
		}
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Fprintln(env.Stdout, string(b))
	default:
		fmt.Fprintf(env.Stderr, "prodshadow: invalid --format %q\n", *format)
		return ExitInvalid
	}

	if inGitHubActions(env) && *format == "text" {
		writeAnnotations(env.Stdout, risk.FromStatic(findings))
	}
	if threshold > 0 {
		for _, f := range findings {
			if !f.Suppressed && f.Severity.Rank() >= threshold {
				return ExitBlock
			}
		}
	}
	return ExitOK
}

// readStdinMigration parses a single migration read from standard input,
// for editor integrations and pipelines that do not write a file.
func readStdinMigration(env *Env) ([]*sqlinspect.Statement, error) {
	if env.Stdin == nil {
		return nil, fmt.Errorf("--migration -: no standard input")
	}
	src, err := sqlinspect.ReadSourceFrom(env.Stdin, "<stdin>")
	if err != nil {
		return nil, err
	}
	return sqlinspect.ParseSources([]sqlinspect.Source{src})
}

// lockSummary lists the locks s takes. Locks on relations an earlier
// statement of the migration created are marked as such: nothing in the
// application uses those relations yet.
func lockSummary(s *sqlinspect.Statement, created *sqlinspect.Created) string {
	if !s.Parsed {
		return "not analysed"
	}
	var parts []string
	for _, l := range s.Locks {
		if by := created.By(s, l.Table); by != nil {
			parts = append(parts, fmt.Sprintf("%s on %s (created by %s)", l.Mode, l.Table, by.ID))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s on %s (%s)", l.Mode, l.Table, l.Mode.Impact()))
	}
	if len(parts) == 0 {
		return "no table locks on existing relations"
	}
	return strings.Join(parts, "; ")
}

func printInspectText(w io.Writer, stmts []*sqlinspect.Statement, findings []sqlinspect.Finding) {
	fmt.Fprintf(w, "%d statement(s) analysed with the %s grammar\n\n", len(stmts), sqlinspect.ParserGrammar)
	created := sqlinspect.NewCreated(stmts)
	for _, s := range stmts {
		fmt.Fprintf(w, "  %-5s %s:%d  %s\n        locks: %s\n", s.ID, s.File, s.StartLine, s.Tag, lockSummary(s, created))
		if m := created.ExistingLock(s).Mode; m != 0 {
			if blocked := m.BlockedStatements(); len(blocked) > 0 {
				fmt.Fprintf(w, "        while held or queued, blocks: %s\n", strings.Join(blocked, ", "))
			}
		}
		if s.Txn == sqlinspect.TxnNotInBlock {
			fmt.Fprintln(w, "        runs outside a transaction block")
		}
	}
	fmt.Fprintln(w)
	printFindingsText(w, findings)
}

func printFindingsText(w io.Writer, findings []sqlinspect.Finding) {
	if len(findings) == 0 {
		fmt.Fprintln(w, "No static findings. Static analysis cannot see lock queues; run 'prodshadow verify' to measure them.")
		return
	}
	for _, f := range findings {
		mark := ""
		if f.Suppressed {
			mark = " [suppressed: " + f.SuppressionReason + "]"
		}
		fmt.Fprintf(w, "%s %s %s (%s:%d)%s\n  %s\n", strings.ToUpper(string(f.Severity)), f.RuleID, f.Rule, f.File, f.Line, mark, f.Message)
		if f.Remediation != "" {
			fmt.Fprintf(w, "  fix: %s\n", f.Remediation)
		}
		fmt.Fprintln(w)
	}
}

func printInspectMarkdown(w io.Writer, stmts []*sqlinspect.Statement, findings []sqlinspect.Finding) {
	fmt.Fprintf(w, "## ProdShadow static analysis\n\n| ID | Location | Statement | Locks |\n|---|---|---|---|\n")
	created := sqlinspect.NewCreated(stmts)
	for _, s := range stmts {
		fmt.Fprintf(w, "| %s | %s:%d | %s | %s |\n", s.ID, s.File, s.StartLine, s.Tag, strings.ReplaceAll(lockSummary(s, created), "|", "\\|"))
	}
	fmt.Fprintf(w, "\n| Severity | Rule | Location | Finding |\n|---|---|---|---|\n")
	for _, f := range findings {
		msg := strings.ReplaceAll(f.Message, "|", "\\|")
		if f.Suppressed {
			msg = "~~" + msg + "~~ (suppressed: " + f.SuppressionReason + ")"
		}
		fmt.Fprintf(w, "| %s | [%s](%s) %s | %s:%d | %s |\n", f.Severity, f.RuleID, report.RulePageURL(f.RuleID), f.Rule, f.File, f.Line, msg)
	}
	fmt.Fprintln(w)
}
