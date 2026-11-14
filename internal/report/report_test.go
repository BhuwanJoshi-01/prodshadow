package report

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/config"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/experiment"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/migrate"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/postgres"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/risk"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/security"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/sqlinspect"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/stats"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/telemetry"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/version"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/workload"
)

var update = flag.Bool("update", false, "rewrite golden files")

const secret = "s3cr3t-token-value-123"

func sampleInput(t *testing.T) Input {
	t.Helper()
	cfg := &config.Config{Project: "orders-api"}
	cfg.ApplyDefaults()
	cfg.Postgres.Image = "postgres:17-alpine"
	cfg.Migration.Files = []string{"migrations/0042_add_note.sql"}
	cfg.Workload.MinSamplesPerScenario = 10

	stmts, err := sqlinspect.ParseSources([]sqlinspect.Source{{Path: "migrations/0042_add_note.sql",
		Content: "-- add a note\nALTER TABLE orders ADD COLUMN note text DEFAULT '" + secret + "';\n"}})
	if err != nil {
		t.Fatal(err)
	}
	wl, err := workload.Parse([]byte(`version: 1
name: orders
scenarios:
  - id: read_order
    weight: 1
    mode: read
    sql: "SELECT * FROM orders WHERE id = $1"
    parameters: [{generator: int_range, min: 1, max: 10}]
`))
	if err != nil {
		t.Fatal(err)
	}
	sc := func(p95, max float64) []workload.ScenarioStats {
		return []workload.ScenarioStats{{ID: "read_order", Mode: "read", Ops: 1000, Throughput: 100, MaxObservedMS: max,
			Latency: stats.Summary{Count: 1000, P50MS: p95 / 2, P95MS: p95, P99MS: p95 * 2, MaxMS: max}}}
	}
	res := &experiment.Result{
		RunID:  "run-1",
		Server: postgres.ServerInfo{Version: "PostgreSQL 17.12 on x86_64", SettingsHash: "sha256:abc"},
		Tables: []postgres.TableSize{{Name: "public.orders", RowsEstimate: 20000, TotalBytes: 1 << 20}},
	}
	for r := 0; r < 3; r++ {
		res.Phases = append(res.Phases,
			experiment.PhaseResult{Step: experiment.Step{Kind: experiment.Baseline, Repeat: r}, Scenarios: sc(1, 4),
				Timeline: []workload.Bucket{{OffsetSec: 0, Ops: 100, P95MS: 1, MaxObservedMS: 4}}, Locks: telemetry.Summary{SampleIntervalMS: 100, Samples: 50}},
			experiment.PhaseResult{Step: experiment.Step{Kind: experiment.Candidate, Repeat: r}, Scenarios: sc(3, 1500),
				DuringMigration: sc(1400, 1500),
				Timeline:        []workload.Bucket{{OffsetSec: 0, Ops: 100, P95MS: 1, MaxObservedMS: 4}, {OffsetSec: 1, Ops: 4, P95MS: 1400, MaxObservedMS: 1500}},
				Locks: telemetry.Summary{SampleIntervalMS: 100, Samples: 50, MaxBlockedByMigration: 4, MaxLockWaitMS: 1450,
					MigrationMaxWaitMS: 1490, MigrationBlockedBy: []string{"prodshadow/background/report/0"}},
				Migration: &migrate.Outcome{Status: migrate.Failed, Started: time.Unix(1, 0), OffsetMS: 1000, DurationMS: 1500, SQLState: "55P03",
					FailedStatement: "1.1", Error: "lock timeout; password=" + secret,
					Statements: []migrate.StatementOutcome{{StatementID: "1.1", File: "migrations/0042_add_note.sql", Line: 2, Tag: "ALTER TABLE", Status: migrate.Failed, SQLState: "55P03", DurationMS: 1500, Error: "x"}}}},
		)
	}
	red, _ := security.NewRedactor(nil)
	red.Add(secret)
	static := sqlinspect.Analyze(stmts, sqlinspect.Options{Now: time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)})
	a := risk.Evaluate(risk.Input{Config: cfg, Result: res, Statements: stmts, Static: static, Now: time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)})
	return Input{
		Config: cfg, RunID: "run-1", Started: time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC), Finished: time.Date(2026, 11, 13, 10, 2, 5, 0, time.UTC),
		Statements: stmts, Workload: wl, WorkloadHash: "sha256:wl", FixtureHash: "sha256:fx", Result: res, Assessment: a, Redactor: red,
		Lookup: func(k string) (string, bool) {
			if k == "GITHUB_SHA" {
				return "0123abcd", true
			}
			return "", false
		},
	}
}

func deterministic(r *Report) *Report {
	r.Tool = version.Info{Version: "v0.1.0-test", Commit: "test", GoVersion: "go1.26", Platform: "linux/amd64"}
	r.Environment = Environment{OS: "linux", Arch: "amd64", HostCPUs: 4}
	return r
}

func TestBuildStatusAndDecision(t *testing.T) {
	r := Build(sampleInput(t))
	if r.Status != StatusCompletedWithFindings || r.CIDecision != "block" || r.Decision.ExitCode != risk.ExitBlock {
		t.Fatalf("status=%s decision=%+v", r.Status, r.Decision)
	}
	if r.GitSHA != "0123abcd" || r.SchemaVersion != SchemaVersion || len(r.Phases) != 6 {
		t.Fatalf("metadata wrong: %+v", r)
	}
	if r.Migration.Statements[0].SQL != "" {
		t.Fatal("raw SQL must not be persisted by default")
	}
}

func TestBuildMarksInfrastructureFailure(t *testing.T) {
	in := sampleInput(t)
	in.Errors = []error{errors.New("docker: daemon not running")}
	r := Build(in)
	if r.Status != StatusFailed || r.Decision.ExitCode != risk.ExitInfra || r.CIDecision != "error" {
		t.Fatalf("failed run not reported: %s %+v", r.Status, r.Decision)
	}
}

func TestSeededSecretNeverReachesArtifacts(t *testing.T) {
	for _, raw := range []bool{false, true} {
		in := sampleInput(t)
		in.Config.Privacy.PersistRawQueryText = raw
		in.Errors = []error{errors.New("connect failed: postgres://app:" + secret + "@db/app")}
		r := Build(in)
		js, err := r.JSON()
		if err != nil {
			t.Fatal(err)
		}
		html, err := r.HTML()
		if err != nil {
			t.Fatal(err)
		}
		for name, out := range map[string]string{"json": string(js), "markdown": r.Markdown(), "html": html} {
			if strings.Contains(out, secret) {
				t.Errorf("raw=%v: secret leaked into %s output", raw, name)
			}
		}
		if raw && !strings.Contains(string(js), "ALTER TABLE orders ADD COLUMN note") {
			t.Error("raw SQL should be included when persistence is enabled")
		}
	}
}

func TestJSONRoundTrip(t *testing.T) {
	r := Build(sampleInput(t))
	js, err := r.JSON()
	if err != nil {
		t.Fatal(err)
	}
	back, err := Read(bytes.NewReader(js))
	if err != nil {
		t.Fatal(err)
	}
	if back.RunID != r.RunID || len(back.Findings) != len(r.Findings) || back.Decision.ExitCode != r.Decision.ExitCode {
		t.Fatal("round trip lost data")
	}
	if _, err := Read(strings.NewReader(`{"schema_version":"2.0.0"}`)); err == nil {
		t.Fatal("a newer major schema must be rejected")
	}
}

func TestMarkdownGolden(t *testing.T) {
	got := deterministic(Build(sampleInput(t))).Markdown()
	path := filepath.Join("testdata", "report.golden.md")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden file (run with -update): %v", err)
	}
	if got != string(want) {
		t.Fatalf("markdown differs from golden file; run go test ./internal/report -update and review the diff")
	}
	for _, want := range []string{"**Decision: BLOCK**", "OBS002", "migrations/0042_add_note.sql:2", "prodshadow/background/report/0", "Timeline (candidate repeat"} {
		if !strings.Contains(got, want) {
			t.Errorf("markdown should contain %q", want)
		}
	}
}

func TestEffectiveConfigIsRecorded(t *testing.T) {
	in := sampleInput(t)
	in.Config.Privacy.RedactPatterns = []string{`customer-\d+`}
	js, err := Build(in).JSON()
	if err != nil {
		t.Fatal(err)
	}
	s := string(js)
	if !strings.Contains(s, `"effective_config"`) || !strings.Contains(s, `"lock_timeout_ms": 1500`) {
		t.Fatal("effective configuration missing from the report")
	}
	if strings.Contains(s, `customer-\\d+`) {
		t.Fatal("redaction patterns must not be echoed")
	}
}

func TestMarkdownCompatibilitySection(t *testing.T) {
	r := Build(sampleInput(t))
	if strings.Contains(r.Markdown(), "Application compatibility") {
		t.Fatal("no section without probes")
	}
	probes := func(broken bool) []experiment.Compatibility {
		old := experiment.Compatibility{Version: "2026.10", Probe: "legacy_sku", BeforeOK: true, AfterOK: !broken}
		if broken {
			old.AfterSQLState, old.AfterError = "42703", `column "legacy_sku" does not exist`
		}
		return []experiment.Compatibility{old, {Version: "2026.11", Probe: "by_id", BeforeOK: true, AfterOK: true}}
	}
	n := 0
	for i := range r.Phases {
		if r.Phases[i].Kind == "candidate" {
			r.Phases[i].Compatibility = probes(n == 0)
			n++
		}
	}
	md := r.Markdown()
	for _, want := range []string{
		"## Application compatibility",
		"| 2026.10 | legacy_sku | ok | failed (42703: column \"legacy_sku\" does not exist) | 1 of " + strconv.Itoa(n) + " |",
		"| 2026.11 | by_id | ok | ok | 0 of " + strconv.Itoa(n) + " |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown lacks %q:\n%s", want, md)
		}
	}
}

func TestMarkdownShowsLockRetries(t *testing.T) {
	r := Build(sampleInput(t))
	for i := range r.Phases {
		if m := r.Phases[i].Migration; m != nil && r.Phases[i].Repeat == 0 {
			m.Retries = []migrate.Retry{{StatementID: "1.1", Attempt: 1, WaitedMS: 1500}, {StatementID: "1.1", Attempt: 2, WaitedMS: 1500}}
		}
	}
	if md := r.Markdown(); !strings.Contains(md, "Retried on lock_timeout (`migration.lock_retries`): repeat 1: 2 retries, 3000 ms waiting for locks.") {
		t.Fatalf("markdown lacks the retries line:\n%s", md)
	}
}
