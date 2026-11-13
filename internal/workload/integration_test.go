//go:build integration

package workload

import (
	"context"
	"testing"
	"time"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/isolation"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/pgtest"

	"github.com/jackc/pgx/v5"
)

func TestRunAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	_, srv := pgtest.Start(t)
	admin, err := srv.Connect(ctx, srv.Admin(), isolation.ConnOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "CREATE TABLE orders (id bigint PRIMARY KEY, status text); INSERT INTO orders SELECT g, 'new' FROM generate_series(1, 5000) g"); err != nil {
		t.Fatal(err)
	}

	spec, err := Parse([]byte(`version: 1
scenarios:
  - id: read
    weight: 3
    mode: read
    sql: "SELECT status FROM orders WHERE id = $1"
    parameters: [{generator: int_range, min: 1, max: 5000}]
  - id: write
    weight: 1
    mode: write
    sql: "UPDATE orders SET status = $1 WHERE id = $2"
    parameters:
      - {generator: choice, values: [a, b]}
      - {generator: int_range, min: 1, max: 5000}
background:
  - id: report
    sessions: 1
    start_after_seconds: 0.5
    transaction:
      statements: ["SELECT count(*) FROM orders"]
      hold_ms: 2500
`))
	if err != nil {
		t.Fatal(err)
	}

	measureStarted := make(chan time.Time, 1)
	res, err := Run(ctx, spec, Options{
		Concurrency: 4, Warmup: 500 * time.Millisecond, Measurement: 4 * time.Second, Seed: 7, Stream: "it",
		Connect: func(ctx context.Context, app string) (*pgx.Conn, error) {
			return srv.Connect(ctx, srv.Admin(), isolation.ConnOptions{ApplicationName: app})
		},
		MeasureStarted: func(at time.Time) {
			measureStarted <- at
			go func() {
				// While the report holds ACCESS SHARE, this DDL queues
				// for ACCESS EXCLUSIVE and every later query queues
				// behind it until lock_timeout fires.
				time.Sleep(time.Second)
				conn, err := srv.Connect(ctx, srv.Admin(), isolation.ConnOptions{ApplicationName: "prodshadow/migration"})
				if err != nil {
					return
				}
				defer conn.Close(ctx)
				conn.Exec(ctx, "SET lock_timeout = '1200ms'")
				conn.Exec(ctx, "ALTER TABLE orders ADD COLUMN note text")
			}()
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	<-measureStarted
	stats := Summarize(res, Window{})
	if stats[0].Ops < 50 || stats[1].Ops < 10 {
		t.Fatalf("too few operations: %+v", stats)
	}
	if stats[0].Errors != 0 {
		t.Fatalf("reads should not fail: %+v", stats[0])
	}
	// Queries arriving after the DDL started waited roughly lock_timeout.
	if stats[0].MaxObservedMS < 800 {
		t.Fatalf("expected lock-queue amplification on reads, max latency %.1fms", stats[0].MaxObservedMS)
	}
	if len(Timeline(res, time.Second)) != 4 {
		t.Fatal("expected 4 timeline buckets")
	}
	t.Logf("read p50=%.2fms max=%.1fms ops=%d; write ops=%d", stats[0].Latency.P50MS, stats[0].MaxObservedMS, stats[0].Ops, stats[1].Ops)
}
