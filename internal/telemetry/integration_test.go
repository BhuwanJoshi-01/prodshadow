//go:build integration

package telemetry

import (
	"context"
	"testing"
	"time"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/isolation"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/pgtest"

	"github.com/jackc/pgx/v5"
)

func TestSamplerSeesLockQueue(t *testing.T) {
	ctx := context.Background()
	_, srv := pgtest.Start(t)
	connect := func(app string) *pgx.Conn {
		c, err := srv.Connect(ctx, srv.Admin(), isolation.ConnOptions{ApplicationName: app})
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	monitor := connect("prodshadow/telemetry")
	holder := connect("prodshadow/background/report/0")
	migration := connect(AppMigration)
	worker := connect("prodshadow/worker/0")
	if _, err := monitor.Exec(ctx, "CREATE TABLE orders (id int)"); err != nil {
		t.Fatal(err)
	}
	before, err := Snapshot(ctx, monitor)
	if err != nil {
		t.Fatal(err)
	}

	tx, err := holder.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "SELECT count(*) FROM orders"); err != nil {
		t.Fatal(err)
	}
	// Both statements wait on locks; their own results do not matter.
	go func() { _, _ = migration.Exec(ctx, "ALTER TABLE orders ADD COLUMN x int") }()
	time.Sleep(200 * time.Millisecond)
	go func() { _, _ = worker.Exec(ctx, "INSERT INTO orders VALUES (1)") }()

	sctx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	samples, failed := NewSampler(monitor, 50*time.Millisecond).Run(sctx)
	cancel()
	_ = tx.Rollback(ctx)
	time.Sleep(300 * time.Millisecond)

	s := Summarize(samples, failed, 50*time.Millisecond, time.Now().Add(-time.Second), time.Second)
	if failed != 0 || s.Samples < 5 {
		t.Fatalf("sampling problems: %+v", s)
	}
	if s.MigrationMaxWaitMS < 200 || len(s.MigrationBlockedBy) != 1 || s.MigrationBlockedBy[0] != "prodshadow/background/report/0" {
		t.Fatalf("migration wait not attributed: %+v", s)
	}
	if s.MaxBlockedByMigration != 1 || s.MaxWaitingSessions != 1 {
		t.Fatalf("worker queued behind migration not detected: %+v", s)
	}
	after, err := Snapshot(ctx, monitor)
	if err != nil {
		t.Fatalf("monitor connection must survive the sampler: %v", err)
	}
	d, err := Delta(ctx, monitor, before, after)
	if err != nil || d.WALBytes <= 0 {
		t.Fatalf("WAL delta = %d, %v (before %s after %s)", d.WALBytes, err, before.WALLSN, after.WALLSN)
	}
}
