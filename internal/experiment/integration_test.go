//go:build integration

package experiment

import (
	"context"
	"testing"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/config"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/isolation"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/migrate"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/security"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/sqlinspect"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/workload"
)

const schema = `CREATE TABLE orders (id bigint PRIMARY KEY, customer_id int NOT NULL, status text NOT NULL);
INSERT INTO orders SELECT g, g % 500, 'new' FROM generate_series(1, 20000) g;`

const workloadDoc = `version: 1
scenarios:
  - id: read_order
    weight: 3
    mode: read
    sql: "SELECT status FROM orders WHERE id = $1"
    parameters: [{generator: int_range, min: 1, max: 20000}]
  - id: update_status
    weight: 1
    mode: write
    sql: "UPDATE orders SET status = $1 WHERE id = $2"
    parameters:
      - {generator: choice, values: [paid, shipped]}
      - {generator: int_range, min: 1, max: 20000}
background:
  - id: report
    sessions: 1
    start_after_seconds: 0.5
    transaction:
      statements: ["SELECT count(*) FROM orders"]
      hold_ms: 3000
`

func plan(t *testing.T, migration string) Plan {
	t.Helper()
	p, _ := isolation.New("docker", "")
	if err := p.Check(context.Background()); err != nil {
		t.Skip(err)
	}
	cfg := &config.Config{Project: "it"}
	cfg.ApplyDefaults()
	cfg.Postgres.Isolation.MemoryMB = 1024
	cfg.Workload.WarmupSeconds = 1
	cfg.Workload.MeasurementSeconds = 6
	cfg.Workload.Concurrency = 4
	cfg.Workload.Repeats = 2
	cfg.Migration.StartAfterSeconds = 1
	cfg.Migration.LockTimeoutMS = 1500
	stmts, err := sqlinspect.ParseSources([]sqlinspect.Source{{Path: "m.sql", Content: migration}})
	if err != nil {
		t.Fatal(err)
	}
	wl, err := workload.Parse([]byte(workloadDoc))
	if err != nil {
		t.Fatal(err)
	}
	runID, _ := security.RunID()
	r, _ := security.NewRedactor(nil)
	return Plan{Config: cfg, RunID: runID, Provider: p, Redactor: r, Migration: stmts, Workload: wl,
		Schema: &sqlinspect.Source{Path: "schema.sql", Content: schema}, Progress: t.Logf}
}

func TestBlockingMigrationIsObserved(t *testing.T) {
	res, err := Run(context.Background(), plan(t, "ALTER TABLE orders ADD COLUMN note text;"))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Phases) != 4 || res.Tables[0].RowsEstimate < 19000 {
		t.Fatalf("unexpected result: %d phases, tables %+v", len(res.Phases), res.Tables)
	}
	for _, ph := range res.Phases {
		if ph.Kind != Candidate {
			if ph.Migration != nil || ph.Scenarios[0].MaxObservedMS > 1000 {
				t.Fatalf("baseline should not stall: %+v", ph.Scenarios[0])
			}
			continue
		}
		if ph.Migration == nil || ph.Migration.Status != migrate.Failed || !ph.Migration.LockTimedOut() {
			t.Fatalf("migration should hit lock_timeout behind the report: %+v", ph.Migration)
		}
		if ph.DuringMigration[0].MaxObservedMS < 1000 {
			t.Fatalf("reads should queue behind the migration: %+v", ph.DuringMigration[0])
		}
		if ph.Locks.MaxBlockedByMigration < 1 || len(ph.Locks.MigrationBlockedBy) == 0 {
			t.Fatalf("lock attribution missing: %+v", ph.Locks)
		}
		t.Logf("candidate repeat %d: migration waited %.0fms, %d sessions queued behind it, read max %.0fms",
			ph.Repeat, ph.Locks.MigrationMaxWaitMS, ph.Locks.MaxBlockedByMigration, ph.DuringMigration[0].MaxObservedMS)
	}
}

func TestHarmlessMigrationDoesNotStall(t *testing.T) {
	p := plan(t, "CREATE INDEX CONCURRENTLY orders_customer_idx ON orders (customer_id);")
	p.Workload.Background = nil
	res, err := Run(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	for _, ph := range res.Phases {
		if ph.Kind == Candidate && ph.Migration.Status != migrate.Succeeded {
			t.Fatalf("migration failed: %+v", ph.Migration)
		}
		if ph.Locks.MaxBlockedByMigration != 0 {
			t.Fatalf("concurrent index build should not block the workload: %+v", ph.Locks)
		}
	}
}
