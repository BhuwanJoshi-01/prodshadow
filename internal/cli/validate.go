package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/config"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/experiment"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/fixture"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/risk"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/sqlinspect"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/workload"

	pg_query "github.com/pganalyze/pg_query_go/v6"
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
	known := fixtureRelations(cfg)
	warn := func(path, msg string) {
		ps = append(ps, config.Problem{Level: config.LevelWarning, Path: path, Message: msg, Line: cfg.LineOf(path)})
	}
	if f := cfg.Workload.File; f != "" {
		wl, err := workload.Load(cfg.Resolve(f))
		if err != nil {
			add("workload.file", err)
		} else {
			if err := experiment.CheckCapacity(cfg, wl); err != nil {
				add("workload.concurrency", err)
			}
			if known != nil {
				for _, t := range missingRelations(known, workloadSQL(wl)) {
					warn("workload.file", fmt.Sprintf("the workload uses table %s, which the fixture schema does not create; its queries will fail", t))
				}
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
		stmts, err := loadMigrations(cfg.BaseDir(), cfg.Migration.Files)
		if err != nil {
			add("migration.files", err)
		} else if known != nil {
			for _, t := range migrationTablesMissing(known, stmts) {
				warn("migration.files", fmt.Sprintf("the migration changes table %s, which the fixture schema does not create; the rehearsal will fail where production may not", t))
			}
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

// fixtureRelations returns the relations the fixture schema creates, or
// nil when there is no schema or it cannot be read (other checks report
// that).
func fixtureRelations(cfg *config.Config) map[string]bool {
	if cfg.Fixture.Schema == "" {
		return nil
	}
	src, err := sqlinspect.ReadSource(cfg.Resolve(cfg.Fixture.Schema), cfg.Fixture.Schema)
	if err != nil {
		return nil
	}
	stmts, err := sqlinspect.ParseSources([]sqlinspect.Source{src})
	if err != nil {
		return nil
	}
	known := map[string]bool{}
	for _, s := range stmts {
		n := s.Node()
		if n == nil {
			continue
		}
		var rv *pg_query.RangeVar
		switch {
		case n.GetCreateStmt() != nil:
			rv = n.GetCreateStmt().Relation
		case n.GetCreateTableAsStmt() != nil:
			rv = n.GetCreateTableAsStmt().GetInto().GetRel()
		case n.GetViewStmt() != nil:
			rv = n.GetViewStmt().View
		}
		if rv != nil {
			known[rv.Relname] = true
		}
	}
	return known
}

// workloadSQL lists every statement of a workload.
func workloadSQL(wl *workload.Spec) []string {
	var out []string
	for _, sc := range wl.Scenarios {
		out = append(out, sc.SQL)
		if sc.Transaction != nil {
			out = append(out, sc.Transaction.Statements...)
		}
	}
	for _, b := range wl.Background {
		out = append(out, b.Transaction.Statements...)
	}
	for _, v := range wl.Compatibility {
		for _, p := range v.Probes {
			out = append(out, p.SQL)
		}
	}
	return out
}

// missingRelations lists, once each, the tables the statements use that
// are not in known. Catalog relations are ignored.
func missingRelations(known map[string]bool, sqls []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, q := range sqls {
		if strings.TrimSpace(q) == "" {
			continue
		}
		st, err := sqlinspect.ParseOne(q)
		if err != nil {
			continue
		}
		for _, t := range st.Tables() {
			name := t[strings.LastIndex(t, ".")+1:]
			if known[name] || seen[name] || strings.HasPrefix(t, "pg_catalog.") || strings.HasPrefix(name, "pg_") {
				continue
			}
			seen[name] = true
			out = append(out, t)
		}
	}
	return out
}

// migrationTablesMissing lists tables the migration locks that neither the
// fixture schema nor an earlier statement of the migration creates. Index
// names (locks recorded by DROP INDEX) are skipped.
func migrationTablesMissing(known map[string]bool, stmts []*sqlinspect.Statement) []string {
	created := map[string]bool{}
	seen := map[string]bool{}
	var out []string
	for _, s := range stmts {
		if s.Kind != sqlinspect.KindDropIndex {
			for _, t := range s.Tables() {
				name := t[strings.LastIndex(t, ".")+1:]
				if known[name] || created[name] || seen[name] || strings.HasPrefix(name, "pg_") {
					continue
				}
				seen[name] = true
				out = append(out, t)
			}
		}
		if n := s.Node(); n != nil {
			switch {
			case n.GetCreateStmt() != nil:
				created[n.GetCreateStmt().GetRelation().GetRelname()] = true
			case n.GetCreateTableAsStmt() != nil:
				created[n.GetCreateTableAsStmt().GetInto().GetRel().GetRelname()] = true
			case n.GetViewStmt() != nil:
				created[n.GetViewStmt().GetView().GetRelname()] = true
			}
		}
	}
	return out
}
