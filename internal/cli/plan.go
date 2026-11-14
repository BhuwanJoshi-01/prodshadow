package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/config"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/experiment"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/sqlinspect"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/workload"
)

// phaseOverhead approximates the per-phase cost of copying the template,
// checkpointing and flushing statistics (see docs/benchmarks.md).
const phaseOverhead = 3 * time.Second

type planGroup struct {
	Transaction string          `json:"transaction"`
	Statements  []planStatement `json:"statements"`
}

type planStatement struct {
	ID    string `json:"id"`
	File  string `json:"file"`
	Line  int    `json:"line"`
	Tag   string `json:"tag"`
	Locks string `json:"locks"`
}

type planOutput struct {
	Project          string              `json:"project"`
	Postgres         string              `json:"postgres"`
	Isolation        string              `json:"isolation"`
	Fixture          []string            `json:"fixture"`
	Workload         string              `json:"workload"`
	Compatibility    []string            `json:"compatibility,omitempty"`
	Sessions         int                 `json:"sessions"`
	Schedule         []experiment.Step   `json:"schedule"`
	EstimatedSeconds float64             `json:"estimated_seconds"`
	Migration        []planGroup         `json:"migration"`
	MigrationTiming  string              `json:"migration_timing"`
	Conflicts        []workload.Conflict `json:"conflicts,omitempty"`
	Unblocked        []string            `json:"unblocked,omitempty"`
	Warnings         []string            `json:"warnings,omitempty"`
}

func runPlan(_ context.Context, env *Env, args []string) int {
	fs := newFlags(env, "plan", "[--config prodshadow.yaml] [--migration FILE ...] [--json]")
	cfgPath := fs.String("config", DefaultConfig, "configuration file")
	var files multiFlag
	fs.Var(&files, "migration", "migration file, overriding migration.files (repeatable)")
	asJSON := fs.Bool("json", false, "print machine-readable JSON")
	asMarkdown := fs.Bool("markdown", false, "print Markdown, e.g. for a pull request comment")
	changedSince := fs.String("changed-since", "", "only migrations git reports as added or changed since this ref (e.g. origin/main)")
	if code := parseFlags(fs, args); code >= 0 {
		return code
	}
	p, err := loadProject(env, *cfgPath)
	if err != nil {
		fmt.Fprintf(env.Stderr, "prodshadow: %v\n", err)
		return ExitInvalid
	}
	if p.problems.HasErrors() {
		printProblems(env.Stderr, p.problems.Errors())
		return ExitInvalid
	}
	cfg := p.cfg
	migFiles, base := cfg.Migration.Files, cfg.BaseDir()
	if len(files) > 0 {
		migFiles, base = files, env.Dir
	}
	if *changedSince != "" {
		changed, err := changedMigrations(base, migFiles, *changedSince)
		if err != nil {
			fmt.Fprintf(env.Stderr, "prodshadow: --changed-since: %v\n", err)
			return ExitInvalid
		}
		if len(changed) == 0 {
			fmt.Fprintf(env.Stdout, "No migration changed since %s; verify would have nothing to run.\n", *changedSince)
			return ExitOK
		}
		migFiles = changed
	}
	stmts, err := loadMigrations(base, migFiles)
	if err != nil {
		fmt.Fprintf(env.Stderr, "prodshadow: %v\n", err)
		return ExitInvalid
	}
	resolveFromFixture(cfg, stmts)
	inputs, err := loadExperimentInputs(cfg)
	if err != nil {
		fmt.Fprintf(env.Stderr, "prodshadow: %v\n", err)
		return ExitInvalid
	}

	out := buildPlan(cfg, stmts, inputs)
	for _, w := range p.problems.Warnings() {
		out.Warnings = append(out.Warnings, w.String())
	}
	if err := experiment.CheckCapacity(cfg, inputs.workload); err != nil {
		out.Warnings = append(out.Warnings, err.Error())
	}
	if *asJSON {
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Fprintln(env.Stdout, string(b))
		return ExitOK
	}
	if *asMarkdown {
		printPlanMarkdown(env.Stdout, out)
		return ExitOK
	}
	printPlan(env.Stdout, out)
	return ExitOK
}

// printPlanMarkdown renders the parts of a plan a reviewer needs: what the
// migration locks and which workload sessions it is predicted to conflict
// with, before any experiment runs.
func printPlanMarkdown(w io.Writer, p planOutput) {
	fmt.Fprintf(w, "## ProdShadow plan: %s\n\n", mdCell(p.Project))
	fmt.Fprintf(w, "%s · workload %s · schedule ", mdCell(p.Postgres), mdCell(p.Workload))
	for i, s := range p.Schedule {
		if i > 0 {
			fmt.Fprint(w, " ")
		}
		fmt.Fprintf(w, "%s%d", strings.ToUpper(string(s.Kind[:1])), s.Repeat+1)
	}
	fmt.Fprint(w, "\n\n| ID | Location | Statement | Predicted locks |\n|---|---|---|---|\n")
	for _, g := range p.Migration {
		for _, s := range g.Statements {
			fmt.Fprintf(w, "| %s | %s:%d | %s | %s |\n", s.ID, mdCell(s.File), s.Line, mdCell(s.Tag), mdCell(s.Locks))
		}
	}
	if len(p.Conflicts) > 0 || len(p.Unblocked) > 0 {
		fmt.Fprint(w, "\n**Predicted conflicts** (from lock modes; `verify` measures how long each wait lasts)\n\n| Session | Statement | Table | Migration lock | Session lock |\n|---|---|---|---|---|\n")
		for _, c := range p.Conflicts {
			session := c.Session
			if c.Background {
				session += " (background)"
			}
			fmt.Fprintf(w, "| %s | %s | %s | %s | %s |\n", mdCell(session), c.StatementID, mdCell(c.Table), c.MigrationLock, c.WorkloadLock)
		}
		if len(p.Unblocked) > 0 {
			fmt.Fprintf(w, "\nNo conflict: %s.\n", mdCell(strings.Join(p.Unblocked, ", ")))
		}
	}
	for _, warn := range p.Warnings {
		fmt.Fprintf(w, "\n> %s\n", mdCell(warn))
	}
}

// mdCell escapes text for a Markdown table cell.
func mdCell(s string) string {
	return strings.NewReplacer("|", "\\|", "\n", " ").Replace(s)
}

func buildPlan(cfg *config.Config, stmts []*sqlinspect.Statement, in *experimentInputs) planOutput {
	iso := cfg.Postgres.Isolation
	out := planOutput{
		Project:   cfg.Project,
		Postgres:  fmt.Sprintf("PostgreSQL %s (%s)", cfg.Postgres.Version, cfg.Postgres.Image),
		Isolation: fmt.Sprintf("%s, network %s, %g CPUs, %d MiB memory, %d MiB disk", iso.Provider, iso.Network, iso.CPUs, iso.MemoryMB, iso.EphemeralDiskMB),
		Schedule:  experiment.Schedule(cfg.Workload.Repeats),
		MigrationTiming: fmt.Sprintf("starts %gs into each candidate measurement; lock_timeout %d ms, statement_timeout %d ms; runs as %s",
			cfg.Migration.StartAfterSeconds, cfg.Migration.LockTimeoutMS, cfg.Migration.StatementTimeoutMS, cfg.Migration.RunAs),
	}
	if in.schema != nil {
		out.Fixture = append(out.Fixture, "schema "+in.schema.Path)
	}
	if in.seed != nil {
		out.Fixture = append(out.Fixture, "seed "+in.seed.Path)
	}
	if in.generator != nil {
		out.Fixture = append(out.Fixture, fmt.Sprintf("generator %s (%d rows)", cfg.Fixture.Generator.Path, in.generator.TotalRows()))
	}
	out.Fixture = append(out.Fixture, "hash "+in.fixtureHash)

	if in.workload != nil {
		mode := "closed-loop"
		if cfg.Workload.ArrivalRate > 0 {
			mode = fmt.Sprintf("open-loop at %g ops/s", cfg.Workload.ArrivalRate)
		}
		bg := 0
		for _, b := range in.workload.Background {
			bg += b.Sessions
		}
		out.Sessions = cfg.Workload.Concurrency + bg
		out.Workload = fmt.Sprintf("%s: %s, %d sessions + %d background, %d scenario(s)",
			nameOr(in.workload.Name, cfg.Workload.File), mode, cfg.Workload.Concurrency, bg, len(in.workload.Scenarios))
		for _, v := range in.workload.Compatibility {
			out.Compatibility = append(out.Compatibility, fmt.Sprintf("%s: %d probe(s)", v.Version, len(v.Probes)))
		}
	} else {
		out.Workload = "none (verify would only run static analysis)"
	}

	phase := time.Duration((cfg.Workload.WarmupSeconds+cfg.Workload.MeasurementSeconds)*float64(time.Second)) + phaseOverhead
	if in.noExperiment == "" {
		out.EstimatedSeconds = (time.Duration(len(out.Schedule)) * phase).Seconds()
	}

	for _, g := range sqlinspect.Plan(stmts, sqlinspect.TransactionMode(cfg.Migration.Transaction)) {
		pg := planGroup{Transaction: "autocommit"}
		switch {
		case g.Wrapped:
			pg.Transaction = "wrapped in one transaction"
		case g.Explicit:
			pg.Transaction = "explicit BEGIN/COMMIT in the file"
		}
		for _, s := range g.Statements {
			pg.Statements = append(pg.Statements, planStatement{ID: s.ID, File: s.File, Line: s.StartLine, Tag: s.Tag, Locks: lockSummary(s)})
		}
		out.Migration = append(out.Migration, pg)
	}
	if in.workload != nil {
		out.Conflicts, out.Unblocked = workload.PredictConflicts(in.workload, stmts, fixtureCatalog(cfg))
	}
	return out
}

func nameOr(name, fallback string) string {
	if name != "" {
		return name
	}
	return fallback
}

func printPlan(w io.Writer, p planOutput) {
	fmt.Fprintf(w, "Project    %s\n", p.Project)
	fmt.Fprintf(w, "Server     %s\n", p.Postgres)
	fmt.Fprintf(w, "Isolation  %s\n", p.Isolation)
	fmt.Fprintf(w, "Fixture    %s\n", strings.Join(p.Fixture, "; "))
	fmt.Fprintf(w, "Workload   %s\n", p.Workload)
	if len(p.Compatibility) > 0 {
		fmt.Fprintf(w, "Versions   %s (probed before and after the migration in each candidate)\n", strings.Join(p.Compatibility, "; "))
	}
	order := make([]string, 0, len(p.Schedule))
	for _, s := range p.Schedule {
		order = append(order, fmt.Sprintf("%s%d", strings.ToUpper(string(s.Kind[:1])), s.Repeat+1))
	}
	fmt.Fprintf(w, "Schedule   %s (B = baseline, C = candidate)\n", strings.Join(order, " "))
	if p.EstimatedSeconds > 0 {
		fmt.Fprintf(w, "Estimate   about %s plus server start-up and seeding\n", (time.Duration(p.EstimatedSeconds) * time.Second).String())
	}
	fmt.Fprintf(w, "\nMigration  %s\n", p.MigrationTiming)
	for i, g := range p.Migration {
		fmt.Fprintf(w, "  group %d: %s\n", i+1, g.Transaction)
		for _, s := range g.Statements {
			fmt.Fprintf(w, "    %-5s %s:%d  %s — %s\n", s.ID, s.File, s.Line, s.Tag, s.Locks)
		}
	}
	if len(p.Conflicts) > 0 || len(p.Unblocked) > 0 {
		fmt.Fprintf(w, "\nConflicts  predicted from lock modes: whichever comes second waits; verify measures how long\n")
		for _, c := range p.Conflicts {
			session := c.Session
			if c.Background {
				session += " (background)"
			}
			fmt.Fprintf(w, "  %-28s conflicts with %s (%s on %s vs its %s)\n", session, c.StatementID, c.MigrationLock, c.Table, c.WorkloadLock)
		}
		if len(p.Unblocked) > 0 {
			fmt.Fprintf(w, "  no conflict: %s\n", strings.Join(p.Unblocked, ", "))
		}
	}
	for _, warn := range p.Warnings {
		fmt.Fprintf(w, "\n%s", warn)
	}
	if len(p.Warnings) > 0 {
		fmt.Fprintln(w)
	}
}
