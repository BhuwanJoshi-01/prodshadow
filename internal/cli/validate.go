package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/config"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/experiment"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/fixture"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/risk"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/workload"
)

func runValidate(_ context.Context, env *Env, args []string) int {
	fs := newFlags(env, "validate", "[--config prodshadow.yaml] [--strict] [--json]")
	cfgPath := fs.String("config", DefaultConfig, "configuration file")
	strict := fs.Bool("strict", false, "treat warnings as errors")
	asJSON := fs.Bool("json", false, "print problems as JSON")
	if code := parseFlags(fs, args); code >= 0 {
		return code
	}
	p, err := loadProject(env, *cfgPath)
	if err != nil {
		fmt.Fprintf(env.Stderr, "prodshadow: %v\n", err)
		return ExitInvalid
	}
	problems := append(p.problems, validateReferences(p.cfg)...)
	if *strict {
		problems = problems.StrictErrors()
	}
	if *asJSON {
		out := struct {
			Valid    bool            `json:"valid"`
			Problems config.Problems `json:"problems"`
		}{!problems.HasErrors(), problems.Sorted()}
		if out.Problems == nil {
			out.Problems = config.Problems{}
		}
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Fprintln(env.Stdout, string(b))
	} else {
		printProblems(env.Stdout, problems)
		if !problems.HasErrors() {
			fmt.Fprintf(env.Stdout, "%s is valid (%d warning(s))\n", display(env.Dir, p.cfg.Path()), len(problems.Warnings()))
		}
	}
	if problems.HasErrors() {
		return ExitInvalid
	}
	return ExitOK
}

// validateReferences parses the files the configuration points to, so a
// broken workload or migration is caught before a container is started.
func validateReferences(cfg *config.Config) config.Problems {
	var ps config.Problems
	add := func(path string, err error) {
		ps = append(ps, config.Problem{Level: config.LevelError, Path: path, Message: err.Error(), Line: cfg.LineOf(path)})
	}
	if f := cfg.Workload.File; f != "" {
		wl, err := workload.Load(cfg.Resolve(f))
		if err != nil {
			add("workload.file", err)
		} else {
			if err := experiment.CheckCapacity(cfg, wl); err != nil {
				add("workload.concurrency", err)
			}
			if msg := unreachableQueueBudget(cfg, wl); msg != "" {
				ps = append(ps, config.Problem{Level: config.LevelWarning, Path: "policy.max_queued_sessions", Message: msg, Line: cfg.LineOf("policy.max_queued_sessions")})
			}
		}
	}
	if g := cfg.Fixture.Generator; g != nil && g.Path != "" {
		if _, err := fixture.LoadGeneratorFile(cfg.Resolve(g.Path), cfg.Fixture.MaxRows); err != nil {
			add("fixture.generator.path", err)
		}
	}
	if s := cfg.Policy.Suppressions; s != "" {
		if _, err := risk.LoadSuppressions(cfg.Resolve(s)); err != nil {
			add("policy.suppressions", err)
		}
	}
	if len(cfg.Migration.Files) > 0 {
		if _, err := loadMigrations(cfg.BaseDir(), cfg.Migration.Files); err != nil {
			add("migration.files", err)
		}
	}
	return ps
}

// unreachableQueueBudget explains why policy.max_queued_sessions can never
// be exceeded: the workload does not have more sessions than the budget.
func unreachableQueueBudget(cfg *config.Config, wl *workload.Spec) string {
	limit := cfg.Policy.MaxQueuedSessions
	if limit == nil {
		return ""
	}
	sessions := cfg.Workload.Concurrency
	for _, b := range wl.Background {
		sessions += b.Sessions
	}
	if sessions > *limit {
		return ""
	}
	return fmt.Sprintf("max_queued_sessions is %d but the workload only has %d session(s), so OBS012 can never fire; raise workload.concurrency to at least %d or lower the budget", *limit, sessions, *limit+1)
}
