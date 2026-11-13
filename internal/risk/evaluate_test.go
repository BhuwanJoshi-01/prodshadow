package risk

import (
	"strings"
	"testing"
	"time"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/config"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/experiment"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/migrate"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/sqlinspect"
)

func testConfig() *config.Config {
	c := &config.Config{Project: "t"}
	c.ApplyDefaults()
	c.Workload.MinSamplesPerScenario = 20
	return c
}

func byRule(a Assessment, id string) *Finding {
	for i := range a.Findings {
		if a.Findings[i].RuleID == id {
			return &a.Findings[i]
		}
	}
	return nil
}

func migration(t *testing.T) []*sqlinspect.Statement {
	stmts, err := sqlinspect.ParseSources([]sqlinspect.Source{{Path: "m.sql", Content: "ALTER TABLE orders ADD COLUMN note text;"}})
	if err != nil {
		t.Fatal(err)
	}
	return stmts
}

func TestBlockingExperimentBlocks(t *testing.T) {
	a := Evaluate(Input{Config: testConfig(), Result: blockingResult(), Statements: migration(t)})
	if a.Decision.Outcome != "block" || a.Decision.ExitCode != ExitBlock {
		t.Fatalf("decision = %+v", a.Decision)
	}
	lt := byRule(a, "OBS002")
	if lt == nil || lt.Confidence != evidence.High || lt.Line != 1 || lt.Table != "orders" {
		t.Fatalf("lock timeout finding = %+v", lt)
	}
	if !strings.Contains(lt.Message, "prodshadow/background/report/0") {
		t.Fatalf("message should name the blocker: %s", lt.Message)
	}
	lw := byRule(a, "OBS001")
	if lw != nil {
		t.Fatalf("1.5 s is within the 2 s default budget, got %+v", lw)
	}
	if q := byRule(a, "OBS011"); q == nil || *q.Observed != 4 {
		t.Fatalf("queued sessions finding = %+v", q)
	}
	if p95 := byRule(a, "OBS005"); p95 == nil || p95.Severity != evidence.Critical {
		t.Fatalf("p95 regression finding = %+v", p95)
	}
}

func TestLockWaitBudget(t *testing.T) {
	cfg := testConfig()
	budget := 1000.0
	cfg.Policy.MaxLockWaitMS = &budget
	a := Evaluate(Input{Config: cfg, Result: blockingResult(), Statements: migration(t)})
	f := byRule(a, "OBS001")
	if f == nil || *f.Observed != 1518 || *f.Threshold != 1000 || f.Confidence != evidence.High {
		t.Fatalf("lock wait finding = %+v", f)
	}
	if f.StatementID != "1.1" || f.Line != 1 || f.Table != "orders" {
		t.Fatalf("lock wait finding should point at the migration statement: %+v", f)
	}
}

func TestQuietExperimentPasses(t *testing.T) {
	a := Evaluate(Input{Config: testConfig(), Result: quietResult(), Statements: migration(t)})
	if a.Decision.Outcome != "pass" || a.Decision.ExitCode != ExitPass {
		t.Fatalf("decision = %+v findings %+v", a.Decision, a.Findings)
	}
	if !strings.Contains(a.Decision.Message, "not a guarantee") {
		t.Fatal("pass message must not claim safety")
	}
	if byRule(a, "QUAL003") == nil {
		t.Fatal("representativeness must always be reported")
	}
}

func TestNoExperimentIsInconclusive(t *testing.T) {
	cfg := testConfig()
	a := Evaluate(Input{Config: cfg, NoExperiment: "no workload configured"})
	if a.Decision.Outcome != "inconclusive" || a.Decision.ExitCode != ExitInconclusive {
		t.Fatalf("decision = %+v", a.Decision)
	}
	cfg.Policy.Inconclusive = "pass"
	a = Evaluate(Input{Config: cfg, NoExperiment: "no workload configured"})
	if a.Decision.ExitCode != ExitPass || a.Decision.Outcome != "inconclusive" {
		t.Fatalf("inconclusive pass policy: %+v", a.Decision)
	}
}

func TestNoisyBaselineMakesLatencyInconclusive(t *testing.T) {
	res := blockingResult()
	// Make baseline p95 swing wildly between repeats.
	for i, p95 := range []float64{0.3, 2.4, 0.6} {
		res.Phases[i*2].Scenarios[0].Latency.P95MS = p95
	}
	a := Evaluate(Input{Config: testConfig(), Result: res, Statements: migration(t)})
	if f := byRule(a, "QUAL002"); f == nil {
		t.Fatal("expected unstable baseline finding")
	}
	if f := byRule(a, "OBS005"); f == nil || f.Severity != evidence.Inconclusive {
		t.Fatalf("latency regression should be inconclusive within noise: %+v", f)
	}
}

func TestUndersampledScenario(t *testing.T) {
	cfg := testConfig()
	cfg.Workload.MinSamplesPerScenario = 5000
	a := Evaluate(Input{Config: cfg, Result: quietResult()})
	if f := byRule(a, "QUAL001"); f == nil || f.Severity != evidence.Inconclusive {
		t.Fatalf("expected insufficient samples finding, got %+v", f)
	}
}

func TestOverridesAndSuppressions(t *testing.T) {
	cfg := testConfig()
	cfg.Policy.Rules = map[string]string{"OBS005": "off", "OBS002": "warning"}
	a := Evaluate(Input{Config: cfg, Result: blockingResult(), Statements: migration(t)})
	if byRule(a, "OBS005") != nil {
		t.Fatal("rule turned off must not appear")
	}
	if f := byRule(a, "OBS002"); f.Severity != evidence.Warning {
		t.Fatalf("override not applied: %+v", f)
	}
	if a.Decision.Outcome != "pass" {
		t.Fatalf("no critical findings left, decision %+v", a.Decision)
	}

	sups := []Suppression{
		{Rule: "OBS002", Reason: "accepted for this release", Expires: "2026-12-31"},
		{Rule: "OBS005", Reason: "expired waiver from last year", Expires: "2025-01-01"},
	}
	a = Evaluate(Input{Config: testConfig(), Result: blockingResult(), Statements: migration(t), Suppressions: sups,
		Now: time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)})
	if f := byRule(a, "OBS002"); !f.Suppressed {
		t.Fatal("suppression not applied")
	}
	if len(a.Decision.ExpiredSuppressions) != 1 || a.Decision.Outcome != "block" {
		t.Fatalf("decision = %+v", a.Decision)
	}
}

var _ = experiment.Baseline

func TestResponsibleStatementPrefersBlockingStatements(t *testing.T) {
	stmts, err := sqlinspect.ParseSources([]sqlinspect.Source{{Path: "m.sql", Content: "SET lock_timeout = '1s';\nCREATE INDEX i ON orders (status);\nANALYZE orders;"}})
	if err != nil {
		t.Fatal(err)
	}
	res := quietResult()
	res.Phases[1].Migration.Statements = []migrate.StatementOutcome{
		{StatementID: "1.1", DurationMS: 1}, {StatementID: "1.2", DurationMS: 800}, {StatementID: "1.3", DurationMS: 900},
	}
	if got := responsibleStatement(candidates(res), stmts); got != "1.2" {
		t.Fatalf("got %s, want the blocking CREATE INDEX (1.2)", got)
	}
}

func TestInlineAnnotationSuppressesObservedFinding(t *testing.T) {
	stmts, err := sqlinspect.ParseSources([]sqlinspect.Source{{Path: "m.sql",
		Content: "-- prodshadow:ignore OBS002 reason=\"deployed during the maintenance window\"\nALTER TABLE orders ADD COLUMN note text;"}})
	if err != nil {
		t.Fatal(err)
	}
	a := Evaluate(Input{Config: testConfig(), Result: blockingResult(), Statements: stmts, Now: time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)})
	f := byRule(a, "OBS002")
	if f == nil || !f.Suppressed || f.SuppressionReason != "deployed during the maintenance window" {
		t.Fatalf("inline annotation did not apply to the observed finding: %+v", f)
	}
}
