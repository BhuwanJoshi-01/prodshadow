package experiment

import (
	"strings"
	"testing"
	"time"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/config"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/migrate"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/security"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/workload"
)

func TestPairProbes(t *testing.T) {
	r, err := security.NewRedactor(nil)
	if err != nil {
		t.Fatal(err)
	}
	before := []workload.ProbeResult{
		{Version: "v1", ID: "a", OK: true},
		{Version: "v1", ID: "b", OK: false, SQLState: "42P01", Error: "relation \"x\" does not exist"},
		{Version: "v2", ID: "a", OK: true},
	}
	after := []workload.ProbeResult{
		{Version: "v1", ID: "a", OK: false, SQLState: "42703", Error: "column does not exist; password=hunter2secret"},
		{Version: "v1", ID: "b", OK: false, SQLState: "42P01"},
	}
	// After results arrive in a different order and one is missing.
	after[0], after[1] = after[1], after[0]
	got := pairProbes(Plan{Redactor: r}, before, after)
	if len(got) != 2 {
		t.Fatalf("pairs = %+v", got)
	}
	if !got[0].Broken() || got[0].AfterSQLState != "42703" || strings.Contains(got[0].AfterError, "hunter2secret") {
		t.Errorf("first probe: %+v", got[0])
	}
	if got[1].Probe != "b" || got[1].Broken() {
		t.Errorf("a probe that failed before is not broken by the migration: %+v", got[1])
	}
}

func TestRedactOutcome(t *testing.T) {
	r, _ := security.NewRedactor(nil)
	out := &migrate.Outcome{Error: "connect postgres://app:s3cr3tpass@db/app failed",
		Statements: []migrate.StatementOutcome{{Error: "PASSWORD 'p@ss-w0rd!' rejected"}}}
	redactOutcome(Plan{Redactor: r}, out)
	if strings.Contains(out.Error, "s3cr3tpass") || strings.Contains(out.Statements[0].Error, "p@ss-w0rd!") {
		t.Fatalf("secrets survived: %+v", out)
	}
}

func TestHasProbes(t *testing.T) {
	if hasProbes(Plan{}) || hasProbes(Plan{Workload: &workload.Spec{}}) {
		t.Fatal("no workload or no compatibility section means no probes")
	}
	if !hasProbes(Plan{Workload: &workload.Spec{Compatibility: []workload.AppVersion{{Version: "v1"}}}}) {
		t.Fatal("compatibility section means probes")
	}
}

func TestRetryAllowance(t *testing.T) {
	cfg := &config.Config{}
	cfg.ApplyDefaults()
	if got := retryAllowance(cfg); got != 0 {
		t.Fatalf("no retries, no allowance: %v", got)
	}
	cfg.Migration.LockRetries, cfg.Migration.LockTimeoutMS, cfg.Migration.RetryBackoffMS = 3, 1000, 500
	// 3 × 1 s of lock waits + (1 + 2 + 3) × 0.5 s of backoff.
	if got := retryAllowance(cfg); got != 6*time.Second {
		t.Fatalf("allowance = %v", got)
	}
}
