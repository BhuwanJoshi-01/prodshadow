//go:build integration

package workload

import (
	"context"
	"strings"
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

func TestOpenLoopArrivals(t *testing.T) {
	ctx := context.Background()
	_, srv := pgtest.Start(t)
	admin, err := srv.Connect(ctx, srv.Admin(), isolation.ConnOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "CREATE TABLE kv (k int PRIMARY KEY, v int); INSERT INTO kv SELECT g, g FROM generate_series(1, 1000) g"); err != nil {
		t.Fatal(err)
	}
	spec, err := Parse([]byte("version: 1\nscenarios:\n  - {id: get, weight: 1, mode: read, sql: \"SELECT v FROM kv WHERE k = $1\", parameters: [{generator: int_range, min: 1, max: 1000}]}\n"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := Run(ctx, spec, Options{
		Concurrency: 4, ArrivalRate: 200, Warmup: 200 * time.Millisecond, Measurement: 3 * time.Second, Seed: 1, Stream: "open",
		Connect: func(ctx context.Context, app string) (*pgx.Conn, error) {
			return srv.Connect(ctx, srv.Admin(), isolation.ConnOptions{ApplicationName: app})
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	st := Summarize(res, Window{})[0]
	if res.Mode != "open-loop" {
		t.Fatalf("mode = %s", res.Mode)
	}
	// Poisson arrivals at 200/s for 3 s: about 600 operations.
	if st.Ops < 480 || st.Ops > 720 || st.Errors != 0 {
		t.Fatalf("expected ~600 operations without errors, got %+v", st)
	}
	if st.Throughput < 160 || st.Throughput > 240 {
		t.Fatalf("throughput %.1f/s far from the arrival rate", st.Throughput)
	}
}

// TestOpenLoopCountsQueueing saturates a single session: arrivals that find
// it busy are dropped, and latency includes the time spent waiting to be
// served rather than only the query itself.
func TestOpenLoopCountsQueueing(t *testing.T) {
	ctx := context.Background()
	_, srv := pgtest.Start(t)
	spec, err := Parse([]byte("version: 1\nscenarios:\n  - {id: slow, weight: 1, mode: read, sql: \"SELECT pg_sleep(0.05)\"}\n"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := Run(ctx, spec, Options{
		Concurrency: 1, ArrivalRate: 60, Measurement: 2 * time.Second, Seed: 1, Stream: "saturated",
		Connect: func(ctx context.Context, app string) (*pgx.Conn, error) {
			return srv.Connect(ctx, srv.Admin(), isolation.ConnOptions{ApplicationName: app})
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	st := Summarize(res, Window{})[0]
	if st.ByClass["dropped"] == 0 {
		t.Fatalf("a saturated session must drop arrivals: %+v", st)
	}
	// Each query takes 50 ms; queued arrivals wait for their predecessor.
	if st.Latency.P95MS < 70 {
		t.Fatalf("queueing time is missing from latency: p95 %.1f ms", st.Latency.P95MS)
	}
	t.Logf("dropped %d of %d arrivals, p95 %.1f ms", st.ByClass["dropped"], st.Ops, st.Latency.P95MS)
}

func TestRunProbesAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	_, srv := pgtest.Start(t)
	conn, err := srv.Connect(ctx, srv.Admin(), isolation.ConnOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, "CREATE TABLE products (id int PRIMARY KEY, legacy_sku text); INSERT INTO products SELECT g, 'sku' || g FROM generate_series(1, 10) g"); err != nil {
		t.Fatal(err)
	}
	spec, err := Parse([]byte(`version: 1
scenarios:
  - {id: read, weight: 1, mode: read, sql: "SELECT 1"}
compatibility:
  - version: "1.0"
    probes:
      - id: by_sku
        sql: "SELECT legacy_sku FROM products WHERE id = $1"
        parameters: [{generator: int_range, min: 1, max: 10}]
      - id: insert
        sql: "INSERT INTO products (id) VALUES (100)"
  - version: "2.0"
    probes:
      - id: by_id
        sql: "SELECT id FROM products WHERE id = 1"
`))
	if err != nil {
		t.Fatal(err)
	}
	before, err := RunProbes(ctx, conn, spec, 1, "it")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range before {
		if !r.OK {
			t.Fatalf("probe failed before the change: %+v", r)
		}
	}
	// Probes are rolled back: the insert must not persist, or the second
	// run would fail on the primary key.
	if _, err := conn.Exec(ctx, "ALTER TABLE products DROP COLUMN legacy_sku"); err != nil {
		t.Fatal(err)
	}
	after, err := RunProbes(ctx, conn, spec, 1, "it")
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 3 || after[0].OK || after[0].SQLState != "42703" || !after[1].OK || !after[2].OK {
		t.Fatalf("after dropping legacy_sku: %+v", after)
	}
	var n int
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM products WHERE id = 100").Scan(&n); err != nil || n != 0 {
		t.Fatalf("probe writes must be rolled back: count %d, err %v", n, err)
	}
}

func TestRunKeepsSampleErrors(t *testing.T) {
	ctx := context.Background()
	_, srv := pgtest.Start(t)
	spec, err := Parse([]byte(`version: 1
scenarios:
  - {id: ok, weight: 1, mode: read, sql: "SELECT 1"}
  - {id: broken, weight: 1, mode: read, sql: "SELECT missing_column FROM pg_class LIMIT 1"}
`))
	if err != nil {
		t.Fatal(err)
	}
	res, err := Run(ctx, spec, Options{
		Concurrency: 2, Warmup: 100 * time.Millisecond, Measurement: time.Second, Seed: 1, Stream: "it",
		Connect: func(ctx context.Context, app string) (*pgx.Conn, error) {
			return srv.Connect(ctx, srv.Admin(), isolation.ConnOptions{ApplicationName: app})
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	stats := Summarize(res, Window{})
	if stats[0].SampleError != "" {
		t.Errorf("a scenario without errors has no sample: %q", stats[0].SampleError)
	}
	if !strings.HasPrefix(stats[1].SampleError, "42703: column \"missing_column\" does not exist") {
		t.Errorf("sample error = %q", stats[1].SampleError)
	}
}
