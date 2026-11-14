package risk

import (
	"strings"
	"testing"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/experiment"
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
