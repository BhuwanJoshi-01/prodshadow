package risk

import (
	"strings"
	"testing"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/experiment"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/migrate"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/sqlinspect"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/workload"
)

func failingWith(state, msg string) *experiment.Result {
	res := quietResult()
	for i := range res.Phases {
		if m := res.Phases[i].Migration; m != nil {
			m.Status, m.SQLState, m.Error, m.FailedStatement = "failed", state, msg, "1.1"
		}
	}
	return res
}

func TestMigrationFailureRules(t *testing.T) {
	a := Evaluate(Input{Config: testConfig(), Result: failingWith("23502", "column \"region\" contains null values"), Statements: migration(t)})
	f := byRule(a, "OBS003")
	if f == nil || !strings.Contains(f.Message, "23502") || f.Line != 1 || f.Confidence != evidence.High {
		t.Fatalf("OBS003 = %+v", f)
	}
	a = Evaluate(Input{Config: testConfig(), Result: failingWith("57014", "canceling statement due to statement timeout"), Statements: migration(t)})
	if f := byRule(a, "OBS004"); f == nil || f.Severity != evidence.Critical {
		t.Fatalf("OBS004 = %+v", f)
	}
	if a.Decision.Outcome != "block" {
		t.Fatalf("a failing migration must block: %+v", a.Decision)
	}
}

func TestOptionalBudgets(t *testing.T) {
	cfg := testConfig()
	drop, dur := 5.0, 0.05
	cfg.Policy.MaxThroughputDropPct = &drop
	cfg.Policy.MaxMigrationSeconds = &dur
	a := Evaluate(Input{Config: cfg, Result: blockingResult(), Statements: migration(t)})
	if f := byRule(a, "OBS008"); f == nil || *f.Observed != 10 {
		t.Fatalf("OBS008 = %+v", f)
	}
	if f := byRule(a, "OBS009"); f == nil || *f.Observed != 1.5 {
		t.Fatalf("OBS009 = %+v", f)
	}
	// Without the optional budgets neither rule is evaluated.
	a = Evaluate(Input{Config: testConfig(), Result: blockingResult(), Statements: migration(t)})
	if byRule(a, "OBS008") != nil || byRule(a, "OBS009") != nil {
		t.Fatal("optional budgets must be opt-in")
	}
}

func TestQueuedSessionBudget(t *testing.T) {
	cfg := testConfig()
	limit := 2
	cfg.Policy.MaxQueuedSessions = &limit
	a := Evaluate(Input{Config: cfg, Result: blockingResult(), Statements: migration(t)})
	f := byRule(a, "OBS012")
	if f == nil || *f.Observed != 4 || *f.Threshold != 2 || f.Severity != evidence.Critical || f.Line == 0 {
		t.Fatalf("OBS012 = %+v", f)
	}
	limit = 4
	a = Evaluate(Input{Config: cfg, Result: blockingResult(), Statements: migration(t)})
	if byRule(a, "OBS012") != nil {
		t.Fatal("a queue equal to the limit is within budget")
	}
	if a = Evaluate(Input{Config: testConfig(), Result: blockingResult(), Statements: migration(t)}); byRule(a, "OBS012") != nil {
		t.Fatal("the budget must be opt-in")
	}
}

func TestDeadlocksAreReported(t *testing.T) {
	res := quietResult()
	res.Phases[1].Counters.Deadlocks = 2
	a := Evaluate(Input{Config: testConfig(), Result: res})
	if f := byRule(a, "OBS010"); f == nil || *f.Observed != 2 || f.Severity != evidence.Warning {
		t.Fatalf("OBS010 = %+v", f)
	}
}

func TestErrorRateIncrease(t *testing.T) {
	res := quietResult()
	for i := range res.Phases {
		if res.Phases[i].Kind == experiment.Candidate {
			res.Phases[i].Scenarios[0] = scen("read", 2000, 100, 1.0, 2.0, 6)
		}
	}
	a := Evaluate(Input{Config: testConfig(), Result: res})
	if f := byRule(a, "OBS007"); f == nil || *f.Observed != 5 || f.Scenario != "read" {
		t.Fatalf("OBS007 = %+v", f)
	}
}

func TestSamplerStarvedAndConnectFailures(t *testing.T) {
	res := quietResult()
	res.Phases[0].Locks.MaxGapMS = 900
	res.Phases[1].ConnectFailures = 3
	a := Evaluate(Input{Config: testConfig(), Result: res})
	if byRule(a, "QUAL005") == nil || byRule(a, "QUAL006") == nil {
		t.Fatalf("quality findings missing: %+v", a.Findings)
	}
}

func TestCompatibilityFindings(t *testing.T) {
	res := quietResult()
	for i := range res.Phases {
		if res.Phases[i].Kind != experiment.Candidate {
			continue
		}
		res.Phases[i].Compatibility = []experiment.Compatibility{
			{Version: "2026.10", Probe: "legacy_sku", BeforeOK: true, AfterOK: false, AfterSQLState: "42703", AfterError: `column "legacy_sku" does not exist`},
			{Version: "2026.10", Probe: "by_id", BeforeOK: true, AfterOK: true},
			{Version: "2026.11", Probe: "by_id", BeforeOK: true, AfterOK: true},
			{Version: "2026.09", Probe: "typo", BeforeOK: false, BeforeSQLState: "42P01", BeforeError: `relation "prodcts" does not exist`},
		}
	}
	a := Evaluate(Input{Config: testConfig(), Result: res})
	f := byRule(a, "OBS013")
	if f == nil || f.Severity != evidence.Critical || f.Confidence != evidence.High || *f.Observed != 1 || f.AppVersion != "2026.10" || f.Where("-") != "version 2026.10" ||
		!strings.Contains(f.Message, "2026.10") || !strings.Contains(f.Message, "legacy_sku (42703") || strings.Contains(f.Message, "by_id") {
		t.Fatalf("OBS013 = %+v", f)
	}
	q := byRule(a, "QUAL009")
	if q == nil || q.Severity != evidence.Inconclusive || !strings.Contains(q.Message, "typo of version 2026.09") {
		t.Fatalf("QUAL009 = %+v", q)
	}
	if a.Decision.Outcome != "block" {
		t.Fatalf("a broken version must block: %+v", a.Decision)
	}
	if byRule(Evaluate(Input{Config: testConfig(), Result: quietResult()}), "OBS013") != nil {
		t.Fatal("no probes, no finding")
	}
}

func TestLockRetries(t *testing.T) {
	res := quietResult()
	for i := range res.Phases {
		if m := res.Phases[i].Migration; m != nil {
			m.Retries = []migrate.Retry{{StatementID: "1.1", Attempt: 1, WaitedMS: 1500}, {StatementID: "1.1", Attempt: 2, WaitedMS: 1500}}
		}
	}
	a := Evaluate(Input{Config: testConfig(), Result: res, Statements: migration(t)})
	f := byRule(a, "OBS014")
	if f == nil || *f.Observed != 2 || f.Severity != evidence.Info || f.Line != 1 || f.Confidence != evidence.High {
		t.Fatalf("OBS014 = %+v", f)
	}
	// Retries that ran out are part of OBS002.
	res = failingWith("55P03", "canceling statement due to lock timeout")
	for i := range res.Phases {
		if m := res.Phases[i].Migration; m != nil {
			m.Retries = []migrate.Retry{{StatementID: "1.1", Attempt: 1}}
		}
	}
	a = Evaluate(Input{Config: testConfig(), Result: res, Statements: migration(t)})
	if f := byRule(a, "OBS002"); f == nil || !strings.Contains(f.Message, "retried 1 time(s)") {
		t.Fatalf("OBS002 = %+v", f)
	}
	if byRule(a, "OBS014") != nil {
		t.Fatal("a migration that never succeeded is not OBS014")
	}
}

func TestExtraWALBudget(t *testing.T) {
	res := quietResult()
	for i := range res.Phases {
		res.Phases[i].Counters.WALBytes = 1 << 20
		if res.Phases[i].Kind == experiment.Candidate {
			res.Phases[i].Counters.WALBytes = 65 << 20
		}
	}
	cfg := testConfig()
	limit := 32.0
	cfg.Policy.MaxExtraWALMB = &limit
	a := Evaluate(Input{Config: cfg, Result: res})
	f := byRule(a, "OBS015")
	if f == nil || *f.Observed != 64 || *f.Threshold != 32 || f.Severity != evidence.Critical || f.Confidence != evidence.High {
		t.Fatalf("OBS015 = %+v", f)
	}
	limit = 100
	if byRule(Evaluate(Input{Config: cfg, Result: res}), "OBS015") != nil {
		t.Fatal("within budget")
	}
	if byRule(Evaluate(Input{Config: testConfig(), Result: res}), "OBS015") != nil {
		t.Fatal("the budget must be opt-in")
	}
}

func TestTempFileSpill(t *testing.T) {
	res := quietResult()
	for i := range res.Phases {
		if res.Phases[i].Kind == experiment.Candidate {
			res.Phases[i].Counters.TempBytes = 48 << 20
		}
	}
	f := byRule(Evaluate(Input{Config: testConfig(), Result: res}), "OBS016")
	if f == nil || *f.Observed != 48 || f.Severity != evidence.Info {
		t.Fatalf("OBS016 = %+v", f)
	}
	if byRule(Evaluate(Input{Config: testConfig(), Result: quietResult()}), "OBS016") != nil {
		t.Fatal("no spill, no finding")
	}
}

func TestWriteBlockingNotExercised(t *testing.T) {
	stmts, err := sqlinspect.ParseSources([]sqlinspect.Source{{Path: "m.sql", Content: "CREATE INDEX orders_status_idx ON orders (status);\n"}})
	if err != nil {
		t.Fatal(err)
	}
	readOnly := &workload.Spec{Scenarios: []workload.Scenario{{ID: "read", Mode: "read"}}}
	a := Evaluate(Input{Config: testConfig(), Result: quietResult(), Statements: stmts, Workload: readOnly})
	if f := byRule(a, "QUAL010"); f == nil || f.Line != 1 || f.Lock != "SHARE" || f.Severity != evidence.Info {
		t.Fatalf("QUAL010 = %+v", f)
	}
	withWrites := &workload.Spec{Scenarios: []workload.Scenario{{ID: "read", Mode: "read"}, {ID: "write", Mode: "write"}}}
	if byRule(Evaluate(Input{Config: testConfig(), Result: quietResult(), Statements: stmts, Workload: withWrites}), "QUAL010") != nil {
		t.Fatal("a write scenario exercises the lock")
	}
	alter, _ := sqlinspect.ParseSources([]sqlinspect.Source{{Path: "m.sql", Content: "ALTER TABLE orders ADD COLUMN note text;\n"}})
	if byRule(Evaluate(Input{Config: testConfig(), Result: quietResult(), Statements: alter, Workload: readOnly}), "QUAL010") != nil {
		t.Fatal("ACCESS EXCLUSIVE blocks reads too, which the read scenario shows")
	}
}
