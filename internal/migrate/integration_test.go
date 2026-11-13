//go:build integration

package migrate

import (
	"context"
	"testing"
	"time"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/isolation"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/security"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/sqlinspect"

	"github.com/jackc/pgx/v5"
)

func server(t *testing.T) (*isolation.Server, func(string) *pgx.Conn) {
	t.Helper()
	ctx := context.Background()
	p, _ := isolation.New("docker", "")
	if err := p.Check(ctx); err != nil {
		t.Skip(err)
	}
	runID, _ := security.RunID()
	srv, err := p.Start(ctx, isolation.Spec{RunID: runID, PostgresMajor: "17", Image: "postgres:17-alpine", Network: "none",
		MemoryMB: 1024, CPUs: 2, DiskMB: 1024, PidsLimit: 256, StartupTimeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Destroy(context.Background(), srv) })
	connect := func(app string) *pgx.Conn {
		c, err := srv.Connect(ctx, srv.Admin(), isolation.ConnOptions{ApplicationName: app})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { c.Close(context.Background()) })
		return c
	}
	if _, err := connect("setup").Exec(ctx, "CREATE TABLE orders (id int PRIMARY KEY, amount int); INSERT INTO orders SELECT g, g FROM generate_series(1, 1000) g"); err != nil {
		t.Fatal(err)
	}
	return srv, connect
}

func statements(t *testing.T, files ...string) []*sqlinspect.Statement {
	t.Helper()
	var srcs []sqlinspect.Source
	for i, f := range files {
		srcs = append(srcs, sqlinspect.Source{Path: string(rune('a'+i)) + ".sql", Content: f})
	}
	stmts, err := sqlinspect.ParseSources(srcs)
	if err != nil {
		t.Fatal(err)
	}
	return stmts
}

func opts() Options {
	return Options{LockTimeout: 500 * time.Millisecond, StatementTimeout: 10 * time.Second, Transaction: sqlinspect.TxnModeAuto}
}

func TestExecuteMixedTransactionalRequirements(t *testing.T) {
	_, connect := server(t)
	ctx := context.Background()
	out, err := Execute(ctx, connect(AppName), statements(t,
		"ALTER TABLE orders ADD COLUMN note text; UPDATE orders SET note = 'x' WHERE id < 10;",
		"CREATE INDEX CONCURRENTLY orders_amount_idx ON orders (amount);",
		"BEGIN; ALTER TABLE orders ADD COLUMN flag bool; COMMIT;",
	), opts())
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != Succeeded || len(out.Statements) != 6 {
		t.Fatalf("unexpected outcome %+v", out)
	}
	for _, s := range out.Statements {
		if s.Status != Succeeded {
			t.Fatalf("statement %s: %+v", s.StatementID, s)
		}
	}
}

func TestExecuteRollsBackWrappedFileOnFailure(t *testing.T) {
	_, connect := server(t)
	ctx := context.Background()
	conn := connect(AppName)
	out, err := Execute(ctx, conn, statements(t,
		"ALTER TABLE orders ADD COLUMN a int; ALTER TABLE orders ADD COLUMN amount int; ALTER TABLE orders ADD COLUMN b int;",
		"ALTER TABLE orders ADD COLUMN c int;",
	), opts())
	if err != nil {
		t.Fatal(err)
	}
	want := []Status{RolledBack, Failed, Skipped, Skipped}
	for i, s := range out.Statements {
		if s.Status != want[i] {
			t.Fatalf("statement %d status %s, want %s", i, s.Status, want[i])
		}
	}
	if out.Status != Failed || out.FailedStatement != "1.2" || out.SQLState != "42701" {
		t.Fatalf("unexpected failure details %+v", out)
	}
	var exists bool
	conn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'orders' AND column_name = 'a')").Scan(&exists)
	if exists {
		t.Fatal("rolled back column still exists")
	}
}

func TestExecuteReportsLockTimeout(t *testing.T) {
	_, connect := server(t)
	ctx := context.Background()
	holder := connect("holder")
	tx, _ := holder.Begin(ctx)
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT count(*) FROM orders"); err != nil {
		t.Fatal(err)
	}
	out, err := Execute(ctx, connect(AppName), statements(t, "ALTER TABLE orders ADD COLUMN z int;"), opts())
	if err != nil {
		t.Fatal(err)
	}
	if !out.LockTimedOut() || out.Statements[0].DurationMS < 450 {
		t.Fatalf("expected lock timeout after ~500ms, got %+v", out)
	}
}
