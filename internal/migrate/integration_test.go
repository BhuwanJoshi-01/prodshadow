//go:build integration

package migrate

import (
	"context"
	"testing"
	"time"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/isolation"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/pgtest"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/sqlinspect"

	"github.com/jackc/pgx/v5"
)

func server(t *testing.T) (*isolation.Server, func(string) *pgx.Conn) {
	t.Helper()
	ctx := context.Background()
	_, srv := pgtest.Start(t)
	connect := func(app string) *pgx.Conn {
		c, err := srv.Connect(ctx, srv.Admin(), isolation.ConnOptions{ApplicationName: app})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.Close(context.Background()) })
		return c
	}
	if _, err := connect("setup").Exec(ctx, "CREATE TABLE orders (id int PRIMARY KEY, amount int); INSERT INTO orders SELECT g, g FROM generate_series(1, 1000) g"); err != nil {
		t.Fatal(err)
	}
	return srv, connect
}

func statements(t *testing.T, files ...string) []*sqlinspect.Statement {
	t.Helper()
	srcs := make([]sqlinspect.Source, 0, len(files))
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
	if err := conn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'orders' AND column_name = 'a')").Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("rolled back column still exists")
	}
}

func TestExecuteReportsLockTimeout(t *testing.T) {
	_, connect := server(t)
	ctx := context.Background()
	holder := connect("holder")
	tx, err := holder.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
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

func TestExecuteRetriesOnLockTimeout(t *testing.T) {
	_, connect := server(t)
	ctx := context.Background()
	holder := connect("holder")
	tx, _ := holder.Begin(ctx)
	if _, err := tx.Exec(ctx, "SELECT count(*) FROM orders"); err != nil {
		t.Fatal(err)
	}
	// The blocker commits after ~1.2s: the first two 500ms attempts time
	// out, a later one succeeds.
	go func() {
		time.Sleep(1200 * time.Millisecond)
		_ = tx.Rollback(context.Background())
	}()
	o := opts()
	o.LockRetries, o.RetryBackoff = 5, 100*time.Millisecond
	out, err := Execute(ctx, connect(AppName), statements(t, "ALTER TABLE orders ADD COLUMN z int; UPDATE orders SET z = 1 WHERE id < 5;"), o)
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != Succeeded || len(out.Retries) < 1 || len(out.Statements) != 2 {
		t.Fatalf("expected success after retries, got %+v", out)
	}
	for i, r := range out.Retries {
		if r.StatementID != "1.1" || r.Attempt != i+1 || r.WaitedMS < 450 {
			t.Errorf("retry %d: %+v", i, r)
		}
	}
	for _, s := range out.Statements {
		if s.Status != Succeeded {
			t.Errorf("only the successful attempt's outcomes are kept: %+v", out.Statements)
		}
	}
}

func TestExecuteStopsRetryingWhenExhausted(t *testing.T) {
	_, connect := server(t)
	ctx := context.Background()
	holder := connect("holder")
	tx, err := holder.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SELECT count(*) FROM orders"); err != nil {
		t.Fatal(err)
	}
	o := opts()
	o.LockRetries, o.RetryBackoff = 2, 50*time.Millisecond
	out, err := Execute(ctx, connect(AppName), statements(t, "ALTER TABLE orders ADD COLUMN z int;"), o)
	if err != nil {
		t.Fatal(err)
	}
	if !out.LockTimedOut() || out.Status != Failed || len(out.Retries) != 2 {
		t.Fatalf("expected failure after 2 retries, got %+v", out)
	}
}
