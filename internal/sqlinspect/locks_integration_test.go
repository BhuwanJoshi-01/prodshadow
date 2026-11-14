//go:build integration

package sqlinspect

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/isolation"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/pglock"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/pgtest"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// The lock oracle runs each statement inside a transaction on a real
// server and compares the relation locks it holds with the classifier's
// prediction. Only tables, partitioned tables, views and materialized
// views that exist before the statement are compared: indexes and catalogs
// are out of the classifier's scope, and new relations cannot block anyone.
const oracleSchema = `
CREATE TABLE customers (id int PRIMARY KEY, email text);
CREATE TABLE orders (id int PRIMARY KEY, customer_id int, status text, amount int, note text);
CREATE INDEX orders_status_idx ON orders (status);
ALTER TABLE orders ADD CONSTRAINT orders_amount_check CHECK (amount >= 0) NOT VALID;
CREATE TABLE staging (id int, customer_id int, status text, amount int, note text);
CREATE TABLE events (id bigint, at timestamptz NOT NULL) PARTITION BY RANGE (at);
CREATE TABLE events_2026 PARTITION OF events FOR VALUES FROM ('2026-01-01') TO ('2027-01-01');
CREATE TABLE events_2027 (id bigint, at timestamptz NOT NULL);
CREATE MATERIALIZED VIEW order_totals AS SELECT customer_id, sum(amount) AS total FROM orders GROUP BY 1;
CREATE UNIQUE INDEX order_totals_customer ON order_totals (customer_id);
CREATE TABLE legacy (id int);
CREATE VIEW recent_orders AS SELECT id, status FROM orders;
INSERT INTO customers SELECT g, 'c' || g FROM generate_series(1, 100) g;
INSERT INTO orders SELECT g, 1 + g % 100, 'new', g, NULL FROM generate_series(1, 1000) g;
`

var oracleStatements = []string{
	"CREATE INDEX orders_customer_idx ON orders (customer_id)",
	"CREATE UNIQUE INDEX orders_id_amount_idx ON orders (id, amount)",
	"ALTER TABLE orders ADD COLUMN memo text",
	"ALTER TABLE orders ADD COLUMN created_at timestamptz DEFAULT now()",
	"ALTER TABLE orders DROP COLUMN note",
	"ALTER TABLE orders ALTER COLUMN status SET NOT NULL",
	"ALTER TABLE orders ALTER COLUMN note TYPE varchar(200)",
	"ALTER TABLE orders ALTER COLUMN status SET DEFAULT 'new'",
	"ALTER TABLE orders ALTER COLUMN status SET STATISTICS 500",
	"ALTER TABLE orders VALIDATE CONSTRAINT orders_amount_check",
	"ALTER TABLE orders ADD CONSTRAINT orders_customer_fk FOREIGN KEY (customer_id) REFERENCES customers (id) NOT VALID",
	"ALTER TABLE orders ADD CONSTRAINT orders_customer_fk FOREIGN KEY (customer_id) REFERENCES customers (id)",
	"ALTER TABLE orders ADD CONSTRAINT orders_status_check CHECK (status <> '') NOT VALID",
	"ALTER TABLE orders SET (fillfactor = 70)",
	"ALTER TABLE orders SET (autovacuum_vacuum_scale_factor = 0.01)",
	"ALTER TABLE orders CLUSTER ON orders_status_idx",
	"ALTER TABLE orders RENAME COLUMN note TO memo",
	"ALTER TABLE orders RENAME TO purchase_orders",
	"ALTER TABLE events ATTACH PARTITION events_2027 FOR VALUES FROM ('2027-01-01') TO ('2028-01-01')",
	"ALTER TABLE events DETACH PARTITION events_2026",
	"CREATE TABLE events_2028 PARTITION OF events FOR VALUES FROM ('2028-01-01') TO ('2029-01-01')",
	"CREATE TABLE refunds (id int, order_id int REFERENCES orders (id))",
	"CREATE TABLE orders_copy (LIKE orders INCLUDING ALL)",
	"CREATE TABLE archived_orders (archived_at timestamptz) INHERITS (orders)",
	"CREATE TABLE order_snapshot AS SELECT * FROM orders",
	"CREATE MATERIALIZED VIEW customer_totals AS SELECT customer_id, count(*) FROM orders GROUP BY 1 WITH NO DATA",
	"CREATE VIEW paid_orders AS SELECT * FROM orders WHERE status = 'paid'",
	"DROP TABLE legacy",
	"TRUNCATE staging",
	"INSERT INTO staging SELECT * FROM orders",
	"INSERT INTO orders (id, customer_id, status, amount) VALUES (5000, 1, 'new', 1)",
	"UPDATE orders SET status = 'paid' WHERE id = 1",
	"UPDATE orders SET status = s.status FROM staging s WHERE s.id = orders.id",
	"DELETE FROM orders WHERE id = 1",
	"SELECT * FROM orders WHERE id = 1 FOR UPDATE",
	"SELECT count(*) FROM orders",
	"ANALYZE orders",
	"CLUSTER orders USING orders_status_idx",
	"REINDEX TABLE orders",
	"REFRESH MATERIALIZED VIEW order_totals",
	"LOCK TABLE orders IN SHARE MODE",
	"LOCK TABLE orders",
	"COMMENT ON TABLE orders IS 'customer orders'",
	"CREATE TRIGGER orders_touch BEFORE UPDATE ON orders FOR EACH ROW EXECUTE FUNCTION suppress_redundant_updates_trigger()",
	"ALTER TABLE orders ENABLE ROW LEVEL SECURITY",
	"ALTER TABLE orders REPLICA IDENTITY FULL",
	"GRANT SELECT ON orders TO PUBLIC",
	"CREATE OR REPLACE VIEW recent_orders AS SELECT id, status FROM orders",
	"DROP VIEW recent_orders",
}

// oracleKnownGaps lists observed locks the classifier does not predict,
// keyed by statement and relation. REFRESH MATERIALIZED VIEW reads the
// tables of the view's stored query, which a migration file does not show.
var oracleKnownGaps = map[string]map[string]string{
	"REFRESH MATERIALIZED VIEW order_totals":              {"orders": "ACCESS SHARE"},
	"CREATE TABLE order_snapshot AS SELECT * FROM orders": {"orders": "ACCESS SHARE"},
	"COMMENT ON TABLE orders IS 'customer orders'":        {"orders": "SHARE UPDATE EXCLUSIVE"},
}

// oracleVersionGaps adds gaps that only some PostgreSQL majors show.
// Since PostgreSQL 18, GRANT and REVOKE on a table take ACCESS SHARE on it.
var oracleVersionGaps = map[string]map[string]map[string]string{
	"18": {"GRANT SELECT ON orders TO PUBLIC": {"orders": "ACCESS SHARE"}},
}

func knownGap(sql, rel string) string {
	if g, ok := oracleKnownGaps[sql][rel]; ok {
		return g
	}
	_, major := pgtest.Image()
	return oracleVersionGaps[major][sql][rel]
}

func TestLockOracle(t *testing.T) {
	ctx := context.Background()
	_, srv := pgtest.Start(t)
	conn, err := srv.Connect(ctx, srv.Admin(), isolation.ConnOptions{ApplicationName: "lock-oracle"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	if _, err := conn.Exec(ctx, oracleSchema); err != nil {
		t.Fatal(err)
	}
	// Relations are identified by OID: a statement may rename or drop the
	// relation it locks.
	type relation struct {
		OID  uint32
		Name string
	}
	rows, err := conn.Query(ctx, `SELECT c.oid, c.relname FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p', 'm', 'v')`)
	if err != nil {
		t.Fatal(err)
	}
	rels, err := pgx.CollectRows(rows, pgx.RowToStructByPos[relation])
	if err != nil {
		t.Fatal(err)
	}
	existing := map[uint32]string{}
	byName := map[string]bool{}
	for _, r := range rels {
		existing[r.OID] = r.Name
		byName[r.Name] = true
	}

	for _, sql := range oracleStatements {
		t.Run(sql, func(t *testing.T) {
			stmts, err := ParseSources([]Source{{Path: "oracle.sql", Content: sql + ";"}})
			if err != nil || len(stmts) != 1 || !stmts[0].Parsed {
				t.Fatalf("parse: %v", err)
			}
			s := stmts[0]
			predicted := map[string]pglock.Mode{}
			for _, l := range s.Locks {
				name := strings.TrimPrefix(l.Table, "public.")
				if byName[name] {
					predicted[name] = pglock.Strongest(predicted[name], l.Mode)
				}
			}
			observed := observeLocks(ctx, t, conn, sql, existing)
			for name, mode := range observed {
				if _, predictedToo := predicted[name]; !predictedToo && knownGap(sql, name) == mode.String() {
					delete(observed, name)
				}
			}
			if diff := compareLocks(predicted, observed); diff != "" {
				t.Errorf("%s\n%s", sql, diff)
			}
		})
	}
}

// TestNotInBlockOracle checks that statements the classifier says cannot
// run in a transaction block are refused inside one.
func TestNotInBlockOracle(t *testing.T) {
	ctx := context.Background()
	_, srv := pgtest.Start(t)
	conn, err := srv.Connect(ctx, srv.Admin(), isolation.ConnOptions{ApplicationName: "lock-oracle"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	if _, err := conn.Exec(ctx, oracleSchema); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		"CREATE INDEX CONCURRENTLY orders_customer_idx ON orders (customer_id)",
		"DROP INDEX CONCURRENTLY orders_status_idx",
		"REINDEX TABLE CONCURRENTLY orders",
		"ALTER TABLE events DETACH PARTITION events_2026 CONCURRENTLY",
		"VACUUM orders",
		"CLUSTER",
		"REINDEX DATABASE postgres",
		"CREATE DATABASE oracle_db",
		"ALTER SYSTEM SET work_mem = '8MB'",
	} {
		t.Run(sql, func(t *testing.T) {
			stmts, err := ParseSources([]Source{{Path: "oracle.sql", Content: sql + ";"}})
			if err != nil || len(stmts) != 1 {
				t.Fatalf("parse: %v", err)
			}
			if stmts[0].Txn != TxnNotInBlock {
				t.Errorf("classifier says %v, want TxnNotInBlock", stmts[0].Txn)
			}
			tx, err := conn.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback(ctx) }()
			_, err = tx.Exec(ctx, sql)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "25001" {
				t.Errorf("PostgreSQL accepted it inside a transaction block (err %v)", err)
			}
		})
	}
}

func observeLocks(ctx context.Context, t *testing.T, conn *pgx.Conn, sql string, existing map[uint32]string) map[string]pglock.Mode {
	t.Helper()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, sql); err != nil {
		t.Fatalf("execute: %v", err)
	}
	rows, err := tx.Query(ctx, `SELECT l.relation, l.mode FROM pg_locks l
		WHERE l.pid = pg_backend_pid() AND l.locktype = 'relation' AND l.granted`)
	if err != nil {
		t.Fatal(err)
	}
	type held struct {
		Rel  uint32
		Mode string
	}
	locks, err := pgx.CollectRows(rows, pgx.RowToStructByPos[held])
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]pglock.Mode{}
	for _, l := range locks {
		name, ok := existing[l.Rel]
		if !ok {
			continue
		}
		m, err := pglock.Parse(l.Mode)
		if err != nil {
			t.Fatalf("lock mode %q: %v", l.Mode, err)
		}
		out[name] = pglock.Strongest(out[name], m)
	}
	return out
}

func compareLocks(predicted, observed map[string]pglock.Mode) string {
	names := map[string]bool{}
	for n := range predicted {
		names[n] = true
	}
	for n := range observed {
		names[n] = true
	}
	var diffs []string
	for n := range names {
		if predicted[n] != observed[n] {
			diffs = append(diffs, "  "+n+": predicted "+predicted[n].String()+", observed "+observed[n].String())
		}
	}
	sort.Strings(diffs)
	return strings.Join(diffs, "\n")
}
