package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/config"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/experiment"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/fixture"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/isolation"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/report"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/risk"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/security"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/sqlinspect"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/workload"
)

type verifyFlags struct {
	config     string
	migrations multiFlag
	output     string
	formats    string
	repeats    int
	keep       bool
	quiet      bool
	changed    string
}

func runVerify(ctx context.Context, env *Env, args []string) int {
	var vf verifyFlags
	fs := newFlags(env, "verify", "[--config prodshadow.yaml] [--migration FILE ...] [flags]")
	fs.StringVar(&vf.config, "config", DefaultConfig, "configuration file")
	fs.Var(&vf.migrations, "migration", "migration file, overriding migration.files (repeatable)")
	fs.StringVar(&vf.output, "output", "", "report directory, overriding report.directory")
	fs.StringVar(&vf.formats, "format", "", "comma-separated report formats (json, markdown, html, sarif, junit), overriding report.formats")
	fs.IntVar(&vf.repeats, "repeats", 0, "number of baseline/candidate pairs, overriding workload.repeats")
	fs.BoolVar(&vf.keep, "keep", false, "keep the disposable server after the run (local debugging only)")
	fs.BoolVar(&vf.quiet, "quiet", false, "suppress progress output")
	fs.StringVar(&vf.changed, "changed-since", "", "only migrations git reports as added or changed since this ref (e.g. origin/main)")
	if code := parseFlags(fs, args); code >= 0 {
		return code
	}
	started := time.Now()
	if vf.keep {
		if ci, ok := env.Getenv("CI"); ok && ci != "" && ci != "false" {
			fmt.Fprintln(env.Stderr, "prodshadow: warning: --keep leaves the disposable server running; do not use it in CI")
		}
	}

	p, err := loadProject(env, vf.config)
	if err != nil {
		fmt.Fprintf(env.Stderr, "prodshadow: %v\n", err)
		return ExitInvalid
	}
	cfg := p.cfg
	if vf.repeats > 0 {
		cfg.Workload.Repeats = vf.repeats
	}
	if vf.output != "" {
		cfg.Report.Directory = env.abs(vf.output)
	}
	if vf.formats != "" {
		cfg.Report.Formats = strings.Split(vf.formats, ",")
	}
	files, base := cfg.Migration.Files, cfg.BaseDir()
	if len(vf.migrations) > 0 {
		files, base = vf.migrations, env.Dir
		cfg.Migration.Files = nil
		for _, m := range vf.migrations {
			cfg.Migration.Files = append(cfg.Migration.Files, display(cfg.BaseDir(), env.abs(m)))
		}
	}
	if vf.changed != "" {
		changed, err := changedMigrations(base, files, vf.changed)
		if err != nil {
			fmt.Fprintf(env.Stderr, "prodshadow: --changed-since: %v\n", err)
			return ExitInvalid
		}
		if len(changed) == 0 {
			fmt.Fprintf(env.Stdout, "No migration changed since %s; nothing to verify.\n", vf.changed)
			return ExitOK
		}
		files, cfg.Migration.Files = changed, nil
		for _, c := range changed {
			cfg.Migration.Files = append(cfg.Migration.Files, display(cfg.BaseDir(), c))
		}
	}
	problems := append(p.problems, cfg.Validate()...)
	if problems.HasErrors() {
		printProblems(env.Stderr, dedupe(problems).Errors())
		return ExitInvalid
	}
	if !vf.quiet {
		printProblems(env.Stderr, dedupe(problems).Warnings())
	}

	stmts, err := loadMigrations(base, files)
	if err != nil {
		fmt.Fprintf(env.Stderr, "prodshadow: %v\n", err)
		return ExitInvalid
	}
	static := sqlinspect.Analyze(stmts, sqlinspect.Options{
		Transaction: sqlinspect.TransactionMode(cfg.Migration.Transaction),
		Overrides:   cfg.Policy.Rules,
	})

	var sups []risk.Suppression
	if cfg.Policy.Suppressions != "" {
		if sups, err = risk.LoadSuppressions(cfg.Resolve(cfg.Policy.Suppressions)); err != nil {
			fmt.Fprintf(env.Stderr, "prodshadow: %v\n", err)
			return ExitInvalid
		}
	}

	inputs, err := loadExperimentInputs(cfg)
	if err != nil {
		fmt.Fprintf(env.Stderr, "prodshadow: %v\n", err)
		return ExitInvalid
	}
	if err := experiment.CheckCapacity(cfg, inputs.workload); err != nil {
		fmt.Fprintf(env.Stderr, "prodshadow: %v\n", err)
		return ExitInvalid
	}

	redactor, err := security.NewRedactor(cfg.Privacy.RedactPatterns)
	if err != nil {
		fmt.Fprintf(env.Stderr, "prodshadow: %v\n", err)
		return ExitInvalid
	}
	runID, err := security.RunID()
	if err != nil {
		fmt.Fprintf(env.Stderr, "prodshadow: %v\n", err)
		return ExitInternal
	}
	progress := func(format string, args ...any) {
		if !vf.quiet {
			fmt.Fprintf(env.Stderr, "prodshadow: "+format+"\n", args...)
		}
	}

	var res *experiment.Result
	var runErrs []error
	if inputs.noExperiment == "" {
		res, err = runExperiment(ctx, cfg, runID, stmts, inputs, redactor, vf.keep, progress)
		if err != nil {
			runErrs = append(runErrs, err)
		}
	} else {
		progress("skipping the experiment: %s", inputs.noExperiment)
	}

	assessment := risk.Evaluate(risk.Input{Config: cfg, Result: res, Statements: stmts, Static: static,
		Suppressions: sups, NoExperiment: inputs.noExperiment, Workload: inputs.workload})
	rep := report.Build(report.Input{
		Config: cfg, RunID: runID, Started: started, Finished: time.Now(), Statements: stmts,
		Workload: inputs.workload, WorkloadHash: inputs.workloadHash, FixtureHash: inputs.fixtureHash,
		Result: res, Assessment: assessment, Redactor: redactor, EnvOverrides: p.envApplied,
		Errors: runErrs, Lookup: env.Getenv,
	})
	written, err := writeReports(env, cfg, rep)
	if err != nil {
		fmt.Fprintf(env.Stderr, "prodshadow: write report: %v\n", err)
		return ExitInfra
	}
	printSummary(env, rep, written)
	if inGitHubActions(env) {
		writeAnnotations(env.Stdout, rep.Findings)
	}
	return rep.Decision.ExitCode
}

func dedupe(ps config.Problems) config.Problems {
	seen := map[string]bool{}
	var out config.Problems
	for _, p := range ps {
		k := p.String()
		if !seen[k] {
			seen[k] = true
			out = append(out, p)
		}
	}
	return out
}

type experimentInputs struct {
	workload     *workload.Spec
	workloadHash string
	schema       *sqlinspect.Source
	seed         *sqlinspect.Source
	generator    *fixture.GeneratorFile
	fixtureHash  string
	noExperiment string
}

func loadExperimentInputs(cfg *config.Config) (*experimentInputs, error) {
	in := &experimentInputs{}
	h := fixture.NewHash()
	h.Add("postgres", []byte(cfg.Postgres.Version))
	read := func(name, ref string) (*sqlinspect.Source, error) {
		if ref == "" {
			return nil, nil
		}
		src, err := sqlinspect.ReadSource(cfg.Resolve(ref), ref)
		if err != nil {
			return nil, err
		}
		h.Add(name, []byte(src.Content))
		return &src, nil
	}
	var err error
	if in.schema, err = read("schema", cfg.Fixture.Schema); err != nil {
		return nil, err
	}
	if in.seed, err = read("seed", cfg.Fixture.SeedSQL); err != nil {
		return nil, err
	}
	if g := cfg.Fixture.Generator; g != nil {
		raw, err := os.ReadFile(cfg.Resolve(g.Path))
		if err != nil {
			return nil, err
		}
		h.Add("generator", raw)
		h.AddSeed(g.Seed)
		if in.generator, err = fixture.LoadGeneratorFile(cfg.Resolve(g.Path), cfg.Fixture.MaxRows); err != nil {
			return nil, err
		}
	}
	in.fixtureHash = h.String()

	if cfg.Workload.File != "" {
		raw, err := os.ReadFile(cfg.Resolve(cfg.Workload.File))
		if err != nil {
			return nil, err
		}
		if in.workload, err = workload.Parse(raw); err != nil {
			return nil, fmt.Errorf("%s: %w", cfg.Workload.File, err)
		}
		in.workloadHash = security.HashIdentifier(string(raw))
	}
	switch {
	case in.workload == nil:
		in.noExperiment = "no workload is configured"
	case in.schema == nil && in.seed == nil && in.generator == nil:
		in.noExperiment = "no fixture is configured"
	}
	return in, nil
}

func runExperiment(ctx context.Context, cfg *config.Config, runID string, stmts []*sqlinspect.Statement, in *experimentInputs,
	redactor *security.Redactor, keep bool, progress func(string, ...any)) (*experiment.Result, error) {
	provider, err := isolation.New(cfg.Postgres.Isolation.Provider, cfg.Postgres.Isolation.Binary)
	if err != nil {
		return nil, err
	}
	if err := provider.Check(ctx); err != nil {
		return nil, fmt.Errorf("%s provider is not usable: %w (run 'prodshadow doctor')", provider.Name(), err)
	}
	res, err := experiment.Run(ctx, experiment.Plan{
		Config: cfg, RunID: runID, Provider: provider, Redactor: redactor, Migration: stmts,
		Workload: in.workload, Schema: in.schema, Seed: in.seed, Generator: in.generator,
		Keep: keep, Progress: progress,
	})
	if errors.Is(err, context.Canceled) {
		return res, errors.New("interrupted; disposable resources were cleaned up")
	}
	return res, err
}

func writeReports(env *Env, cfg *config.Config, rep *report.Report) ([]string, error) {
	dir := cfg.Report.Directory
	if !filepath.IsAbs(dir) {
		dir = cfg.Resolve(dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	var written []string
	for _, format := range cfg.Report.Formats {
		var data []byte
		var name string
		switch strings.TrimSpace(format) {
		case "json":
			b, err := rep.JSON()
			if err != nil {
				return written, err
			}
			data, name = b, "report.json"
		case "markdown":
			data, name = []byte(rep.Markdown()), "report.md"
		case "html":
			h, err := rep.HTML()
			if err != nil {
				return written, err
			}
			data, name = []byte(h), "report.html"
		case "sarif":
			s, err := rep.SARIF()
			if err != nil {
				return written, err
			}
			data, name = s, "report.sarif"
		case "junit":
			j, err := rep.JUnit()
			if err != nil {
				return written, err
			}
			data, name = j, "report.junit.xml"
		default:
			return written, fmt.Errorf("unknown report format %q", format)
		}
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return written, err
		}
		written = append(written, display(env.Dir, path))
	}
	if summary, ok := env.Getenv("GITHUB_STEP_SUMMARY"); ok && summary != "" {
		if err := appendFile(summary, rep.Markdown()); err != nil {
			fmt.Fprintf(env.Stderr, "prodshadow: could not write the job summary: %v\n", err)
		}
	}
	return written, nil
}

// appendFile appends text, creating the file privately if needed.
func appendFile(path, text string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(text); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func printSummary(env *Env, rep *report.Report, written []string) {
	w := env.Stdout
	fmt.Fprintf(w, "ProdShadow: %s (exit %d) — %s\n", strings.ToUpper(rep.Decision.Outcome), rep.Decision.ExitCode, rep.Decision.Message)
	for _, e := range rep.Errors {
		fmt.Fprintf(w, "  error: %s\n", e)
	}
	for _, f := range rep.Findings {
		if f.Suppressed || f.Severity == evidence.Info {
			continue
		}
		fmt.Fprintf(w, "  %-12s %s %-34s %s\n    %s\n", f.Severity, f.RuleID, f.Rule, f.Where("-"), f.Message)
	}
	if len(written) > 0 {
		fmt.Fprintf(w, "Reports: %s\n", strings.Join(written, ", "))
	}
}
