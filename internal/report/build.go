package report

import (
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/config"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/experiment"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/migrate"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/risk"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/security"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/sqlinspect"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/stats"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/version"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/workload"
)

// Input collects everything a report is built from.
type Input struct {
	Config       *config.Config
	RunID        string
	Started      time.Time
	Finished     time.Time
	Statements   []*sqlinspect.Statement
	Workload     *workload.Spec
	WorkloadHash string
	FixtureHash  string
	Result       *experiment.Result
	Assessment   risk.Assessment
	Redactor     *security.Redactor
	EnvOverrides []string
	// Errors are infrastructure or setup errors. Any error marks the run
	// as failed.
	Errors []error
	// Lookup reads environment variables; os.LookupEnv when nil.
	Lookup func(string) (string, bool)
	// Catalog describes the fixture schema, for predicting the workload's
	// lock conflicts; nil without a schema.
	Catalog *sqlinspect.Catalog
}

// Build assembles a report, applying privacy settings and redaction.
func Build(in Input) *Report {
	cfg := in.Config
	lookup := in.Lookup
	if lookup == nil {
		lookup = os.LookupEnv
	}
	red := in.Redactor
	rawSQL := cfg.Privacy.PersistRawQueryText

	r := &Report{
		SchemaVersion: SchemaVersion,
		RunID:         in.RunID,
		Project:       cfg.Project,
		Tool:          version.Get(),
		GitSHA:        gitSHA(lookup),
		StartedAt:     in.Started.UTC(),
		FinishedAt:    in.Finished.UTC(),
		DurationSec:   stats.Round(in.Finished.Sub(in.Started).Seconds(), 3),
		Engine: Engine{
			PostgresMajor: cfg.Postgres.Version,
			Runtime:       cfg.Postgres.Isolation.Provider,
			Image:         cfg.Postgres.Image,
			Network:       cfg.Postgres.Isolation.Network,
			MemoryMB:      cfg.Postgres.Isolation.MemoryMB,
			CPUs:          cfg.Postgres.Isolation.CPUs,
			DiskMB:        cfg.Postgres.Isolation.EphemeralDiskMB,
			ParserGrammar: sqlinspect.ParserGrammar,
		},
		Representativeness: Representativeness{Level: cfg.Fixture.Representation, Limitations: in.Assessment.Quality.Limitations},
		Fixture:            Fixture{Hash: in.FixtureHash},
		Workload: Workload{
			Hash: in.WorkloadHash, Mode: "closed-loop", Concurrency: cfg.Workload.Concurrency,
			ArrivalRate: cfg.Workload.ArrivalRate, WarmupSec: cfg.Workload.WarmupSeconds,
			MeasurementSec: cfg.Workload.MeasurementSeconds, Repeats: cfg.Workload.Repeats, Seed: cfg.Workload.Seed,
			Scenarios: []ScenarioDescription{},
		},
		Migration: Migration{
			Files: cfg.Migration.Files, LockTimeoutMS: cfg.Migration.LockTimeoutMS,
			StatementTimeoutMS: cfg.Migration.StatementTimeoutMS, Transaction: cfg.Migration.Transaction,
			RunAs: cfg.Migration.RunAs, Statements: []Statement{},
		},
		Comparisons:     in.Assessment.Comparisons,
		Phases:          []Phase{},
		Quality:         in.Assessment.Quality,
		Findings:        in.Assessment.Findings,
		Decision:        in.Assessment.Decision,
		Environment:     Environment{OS: runtime.GOOS, Arch: runtime.GOARCH, HostCPUs: runtime.NumCPU(), EnvOverrides: in.EnvOverrides, CI: ciName(lookup)},
		EffectiveConfig: redactedConfig(cfg, red),
	}
	if cfg.Workload.ArrivalRate > 0 {
		r.Workload.Mode = "open-loop"
	}
	if in.Workload != nil {
		r.PredictedConflicts, _ = workload.PredictConflicts(in.Workload, in.Statements, in.Catalog)
	}
	if r.Comparisons == nil {
		r.Comparisons = []risk.Comparison{}
	}
	if r.Findings == nil {
		r.Findings = []risk.Finding{}
	}

	if in.Workload != nil {
		r.Workload.Name = in.Workload.Name
		for _, sc := range in.Workload.Scenarios {
			stmts := []string{sc.SQL}
			d := ScenarioDescription{ID: sc.ID, Mode: sc.Mode, Weight: sc.Weight}
			if sc.Transaction != nil {
				stmts, d.HoldMS = sc.Transaction.Statements, sc.Transaction.HoldMS
			}
			d.TemplateID = security.HashIdentifier(strings.Join(stmts, ";\n"))
			if rawSQL {
				d.SQL = red.Strings(append([]string(nil), stmts...))
			}
			r.Workload.Scenarios = append(r.Workload.Scenarios, d)
		}
		for _, b := range in.Workload.Background {
			r.Workload.Background = append(r.Workload.Background, workload.ScenarioInfo{ID: b.ID, Mode: "background", Background: true})
		}
	}

	for _, s := range in.Statements {
		st := Statement{ID: s.ID, File: s.File, StartLine: s.StartLine, EndLine: s.EndLine, Kind: string(s.Kind), Tag: s.Tag,
			Transaction: string(s.Txn), Parsed: s.Parsed, ParseError: red.String(s.ParseError), Fingerprint: s.Fingerprint}
		for _, l := range s.Locks {
			st.Locks = append(st.Locks, lockOf(l))
		}
		if rawSQL {
			st.SQL = red.String(s.SQL)
		}
		r.Migration.Statements = append(r.Migration.Statements, st)
	}

	if res := in.Result; res != nil {
		r.Engine.ServerVersion = res.Server.Version
		r.Engine.SettingsHash = res.Server.SettingsHash
		r.Engine.Settings = res.Server.Settings
		r.Fixture.Tables = res.Tables
		r.Fixture.SeedSeconds = stats.Round(res.SeedSeconds, 3)
		for _, rows := range res.FixtureRows {
			r.Fixture.GeneratedRows += rows.Rows
		}
		for _, p := range res.Phases {
			ph := phaseOf(p)
			if ph.Migration != nil {
				// Copy before redacting so the caller's result is untouched;
				// server error messages can echo SQL literals.
				m := *ph.Migration
				m.Error = red.String(m.Error)
				m.Statements = append([]migrate.StatementOutcome(nil), m.Statements...)
				for i := range m.Statements {
					m.Statements[i].Error = red.String(m.Statements[i].Error)
				}
				ph.Migration = &m
			}
			r.Phases = append(r.Phases, ph)
		}
		if res.CleanupError != "" {
			r.Errors = append(r.Errors, "cleanup: "+red.String(res.CleanupError))
		}
	}

	for i := range r.Findings {
		r.Findings[i].Message = red.String(r.Findings[i].Message)
	}
	for _, err := range in.Errors {
		if err != nil {
			r.Errors = append(r.Errors, red.String(err.Error()))
		}
	}

	r.CIDecision = r.Decision.Outcome
	switch {
	case len(r.Errors) > 0:
		r.Status = StatusFailed
		r.CIDecision = "error"
		r.Decision.ExitCode = risk.ExitInfra
		r.Decision.Outcome = "error"
		r.Decision.Message = "The run failed before evidence could be evaluated; see errors."
	case r.Decision.Outcome == "block":
		r.Status = StatusCompletedWithFindings
	case r.Decision.Outcome == "inconclusive":
		r.Status = StatusInconclusive
	default:
		r.Status = StatusCompleted
	}
	return r
}

func gitSHA(lookup func(string) (string, bool)) string {
	for _, k := range []string{"GITHUB_SHA", "CI_COMMIT_SHA", "BUILDKITE_COMMIT", "CIRCLE_SHA1", "GIT_COMMIT"} {
		if v, ok := lookup(k); ok && v != "" {
			return v
		}
	}
	return ""
}

func ciName(lookup func(string) (string, bool)) string {
	checks := []struct{ env, name string }{
		{"GITHUB_ACTIONS", "github-actions"}, {"GITLAB_CI", "gitlab-ci"}, {"BUILDKITE", "buildkite"},
		{"CIRCLECI", "circleci"}, {"JENKINS_URL", "jenkins"}, {"CI", "generic"},
	}
	for _, c := range checks {
		if v, ok := lookup(c.env); ok && v != "" && v != "false" {
			return c.name
		}
	}
	return ""
}

// redactedConfig copies the configuration for the report. User redaction
// patterns are dropped (they can describe what is secret), and free-text
// values pass through the redactor.
func redactedConfig(cfg *config.Config, red *security.Redactor) *config.Config {
	c := *cfg
	c.Privacy.RedactPatterns = nil
	if len(cfg.Postgres.Settings) > 0 {
		c.Postgres.Settings = map[string]string{}
		for k, v := range cfg.Postgres.Settings {
			c.Postgres.Settings[k] = red.String(v)
		}
	}
	return &c
}
