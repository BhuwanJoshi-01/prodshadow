package sqlinspect

import (
	"slices"
	"testing"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/pglock"
)

func one(t *testing.T, sql string) *Statement {
	t.Helper()
	stmts, err := ParseSources([]Source{{Path: "m.sql", Content: sql}})
	if err != nil {
		t.Fatal(err)
	}
	if len(stmts) != 1 {
		t.Fatalf("expected one statement, got %d", len(stmts))
	}
	if !stmts[0].Parsed {
		t.Fatalf("parse failed: %s", stmts[0].ParseError)
	}
	return stmts[0]
}

func TestClassifyLocks(t *testing.T) {
	cases := []struct {
		sql   string
		kind  Kind
		table string
		mode  pglock.Mode
		txn   TxnRequirement
	}{
		{"CREATE INDEX idx ON orders (customer_id)", KindCreateIndex, "orders", pglock.Share, TxnAny},
		{"CREATE INDEX CONCURRENTLY idx ON orders (customer_id)", KindCreateIndex, "orders", pglock.ShareUpdateExclusive, TxnNotInBlock},
		{"ALTER TABLE orders ADD COLUMN note text", KindAlterTable, "orders", pglock.AccessExclusive, TxnAny},
		{"ALTER TABLE orders VALIDATE CONSTRAINT fk", KindAlterTable, "orders", pglock.ShareUpdateExclusive, TxnAny},
		{"ALTER TABLE orders ADD CONSTRAINT fk FOREIGN KEY (customer_id) REFERENCES customers (id) NOT VALID", KindAlterTable, "customers", pglock.ShareRowExclusive, TxnAny},
		{"ALTER TABLE orders SET (fillfactor = 70)", KindAlterTable, "orders", pglock.ShareUpdateExclusive, TxnAny},
		{"ALTER TABLE orders DISABLE TRIGGER audit", KindAlterTable, "orders", pglock.ShareRowExclusive, TxnAny},
		{"ALTER TABLE events DETACH PARTITION events_2024 CONCURRENTLY", KindAlterTable, "events", pglock.ShareUpdateExclusive, TxnNotInBlock},
		{"ALTER TABLE events ATTACH PARTITION events_2026 FOR VALUES FROM ('2026-01-01') TO ('2027-01-01')", KindAlterTable, "events_2026", pglock.AccessExclusive, TxnAny},
		{"ALTER TABLE public.orders RENAME COLUMN note TO memo", KindRename, "public.orders", pglock.AccessExclusive, TxnAny},
		{"ALTER INDEX idx RENAME TO idx2", KindRename, "idx", pglock.ShareUpdateExclusive, TxnAny},
		{"DROP TABLE legacy", KindDropTable, "legacy", pglock.AccessExclusive, TxnAny},
		{"DROP INDEX CONCURRENTLY idx", KindDropIndex, "idx", pglock.ShareUpdateExclusive, TxnNotInBlock},
		{"CREATE TABLE refunds (id int, order_id int REFERENCES orders (id))", KindCreateTable, "orders", pglock.ShareRowExclusive, TxnAny},
		{"CREATE TABLE events_2027 PARTITION OF events FOR VALUES FROM ('2027-01-01') TO ('2028-01-01')", KindCreateTable, "events", pglock.AccessExclusive, TxnAny},
		{"CREATE TABLE events_2027 (LIKE events INCLUDING ALL)", KindCreateTable, "events", pglock.AccessShare, TxnAny},
		{"CREATE TABLE archived_orders (archived_at timestamptz) INHERITS (orders)", KindCreateTable, "orders", pglock.ShareUpdateExclusive, TxnAny},
		{"UPDATE orders SET status = 'x' WHERE id = 1", KindUpdate, "orders", pglock.RowExclusive, TxnAny},
		{"DELETE FROM orders", KindDelete, "orders", pglock.RowExclusive, TxnAny},
		{"INSERT INTO orders SELECT * FROM staging", KindInsert, "orders", pglock.RowExclusive, TxnAny},
		{"TRUNCATE orders, customers", KindTruncate, "customers", pglock.AccessExclusive, TxnAny},
		{"VACUUM FULL orders", KindVacuum, "orders", pglock.AccessExclusive, TxnNotInBlock},
		{"VACUUM (ANALYZE) orders", KindVacuum, "orders", pglock.ShareUpdateExclusive, TxnNotInBlock},
		{"ANALYZE orders", KindVacuum, "orders", pglock.ShareUpdateExclusive, TxnAny},
		{"CLUSTER orders USING idx", KindCluster, "orders", pglock.AccessExclusive, TxnAny},
		{"REINDEX TABLE orders", KindReindex, "orders", pglock.Share, TxnAny},
		{"REINDEX INDEX CONCURRENTLY idx", KindReindex, "idx", pglock.ShareUpdateExclusive, TxnNotInBlock},
		{"REFRESH MATERIALIZED VIEW mv", KindRefreshMatView, "mv", pglock.AccessExclusive, TxnAny},
		{"REFRESH MATERIALIZED VIEW CONCURRENTLY mv", KindRefreshMatView, "mv", pglock.Exclusive, TxnAny},
		{"LOCK TABLE orders IN SHARE MODE", KindLockTable, "orders", pglock.Share, TxnAny},
		{"LOCK TABLE orders", KindLockTable, "orders", pglock.AccessExclusive, TxnAny},
		{"CREATE TRIGGER t BEFORE UPDATE ON orders FOR EACH ROW EXECUTE FUNCTION f()", KindCreateTrigger, "orders", pglock.ShareRowExclusive, TxnAny},
		{"ALTER TABLE orders SET SCHEMA archive", KindAlterTable, "orders", pglock.AccessExclusive, TxnAny},
	}
	for _, c := range cases {
		t.Run(c.sql, func(t *testing.T) {
			st := one(t, c.sql)
			if st.Kind != c.kind {
				t.Errorf("kind = %s, want %s", st.Kind, c.kind)
			}
			if got := st.LockOn(c.table); got != c.mode {
				t.Errorf("lock on %s = %s, want %s (locks %v)", c.table, got, c.mode, st.Locks)
			}
			if st.Txn != c.txn {
				t.Errorf("txn = %s, want %s", st.Txn, c.txn)
			}
		})
	}
}

func TestAlterTableSubcommandsAndStrongestLock(t *testing.T) {
	st := one(t, "ALTER TABLE orders ALTER COLUMN customer_id SET STATISTICS 100, ADD COLUMN x int, ADD CONSTRAINT c CHECK (x > 0) NOT VALID")
	want := []string{"set_statistics", "add_column", "add_check_not_valid"}
	if !slices.Equal(st.Subcommands, want) {
		t.Fatalf("subcommands = %v, want %v", st.Subcommands, want)
	}
	if st.StrongestLock() != pglock.AccessExclusive {
		t.Fatalf("strongest = %s", st.StrongestLock())
	}
}

func TestClassifyMiscStatements(t *testing.T) {
	cases := map[string]Kind{
		"BEGIN":                       KindTransaction,
		"COMMIT":                      KindTransaction,
		"SET lock_timeout = '2s'":     KindSet,
		"DO $$ BEGIN NULL; END $$":    KindProcedural,
		"CREATE EXTENSION pg_trgm":    KindCreateExtension,
		"COMMENT ON TABLE t IS 'x'":   KindComment,
		"CREATE SEQUENCE s":           KindOtherDDL,
		"SELECT 1":                    KindSelect,
		"COPY orders FROM STDIN":      KindCopy,
		"CREATE VIEW v AS SELECT 1":   KindOtherDDL,
		"DROP FUNCTION IF EXISTS f()": KindDropOther,
	}
	for sql, want := range cases {
		if got := one(t, sql).Kind; got != want {
			t.Errorf("%s: kind = %s, want %s", sql, got, want)
		}
	}
	if st := one(t, "SET lock_timeout = '2s'"); !slices.Equal(st.Subcommands, []string{"lock_timeout"}) {
		t.Errorf("SET subcommands = %v", st.Subcommands)
	}
	if one(t, "BEGIN").Txn != TxnControl {
		t.Error("BEGIN should be transaction control")
	}
}

func TestSnakeLabel(t *testing.T) {
	if got := snakeLabel("AT_EnableTrigUser"); got != "enable_trig_user" {
		t.Fatalf("got %s", got)
	}
}

func TestClassifyWideReindex(t *testing.T) {
	cases := map[string]string{
		"REINDEX SCHEMA public":                "REINDEX SCHEMA",
		"REINDEX DATABASE app":                 "REINDEX DATABASE",
		"REINDEX SYSTEM":                       "REINDEX SYSTEM",
		"REINDEX (CONCURRENTLY) SCHEMA public": "REINDEX SCHEMA CONCURRENTLY",
		"REINDEX SCHEMA CONCURRENTLY public":   "REINDEX SCHEMA CONCURRENTLY",
	}
	for sql, tag := range cases {
		st := one(t, sql)
		if st.Kind != KindReindex || st.Tag != tag {
			t.Errorf("%s: kind %s tag %q, want reindex %q", sql, st.Kind, st.Tag, tag)
		}
		if st.Txn != TxnNotInBlock {
			t.Errorf("%s: txn = %s, want %s", sql, st.Txn, TxnNotInBlock)
		}
		if len(st.Locks) != 0 {
			t.Errorf("%s: unexpected locks %v", sql, st.Locks)
		}
	}
}

func TestClassifyStatementsOutsideTransactions(t *testing.T) {
	for sql, tag := range map[string]string{
		"ALTER SYSTEM SET work_mem = '64MB'":          "ALTER SYSTEM",
		"CREATE DATABASE scratch":                     "CREATE DATABASE",
		"DROP DATABASE IF EXISTS scratch":             "DROP DATABASE",
		"CREATE TABLESPACE fast LOCATION '/mnt/fast'": "CREATE TABLESPACE",
		"DROP TABLESPACE fast":                        "DROP TABLESPACE",
		"CLUSTER":                                     "CLUSTER",
	} {
		st := one(t, sql)
		if st.Tag != tag || st.Txn != TxnNotInBlock {
			t.Errorf("%s: tag %q txn %s, want %q %s", sql, st.Tag, st.Txn, tag, TxnNotInBlock)
		}
	}
	if st := one(t, "CLUSTER orders USING orders_pkey"); st.Txn != TxnAny {
		t.Errorf("CLUSTER of one table may run in a transaction block, got %s", st.Txn)
	}
}

func TestClassifyReadLocks(t *testing.T) {
	cases := []struct {
		sql   string
		tag   string
		locks map[string]pglock.Mode
	}{
		{"SELECT o.id FROM orders o JOIN customers c ON c.id = o.customer_id", "SELECT",
			map[string]pglock.Mode{"orders": pglock.AccessShare, "customers": pglock.AccessShare}},
		{"SELECT id FROM orders WHERE id = 1 FOR UPDATE", "SELECT ... FOR UPDATE", map[string]pglock.Mode{"orders": pglock.RowShare}},
		{"SELECT id FROM orders FOR NO KEY UPDATE SKIP LOCKED", "SELECT ... FOR NO KEY UPDATE", map[string]pglock.Mode{"orders": pglock.RowShare}},
		{"WITH recent AS (SELECT * FROM orders) SELECT count(*) FROM recent", "SELECT", map[string]pglock.Mode{"orders": pglock.AccessShare}},
		{"INSERT INTO archive SELECT * FROM orders WHERE created_at < now() - interval '1 year'", "INSERT",
			map[string]pglock.Mode{"archive": pglock.RowExclusive, "orders": pglock.AccessShare}},
		{"UPDATE orders o SET region = c.region FROM customers c WHERE c.id = o.customer_id", "UPDATE",
			map[string]pglock.Mode{"orders": pglock.RowExclusive, "customers": pglock.AccessShare}},
		{"SELECT 1", "SELECT", map[string]pglock.Mode{}},
	}
	for _, tc := range cases {
		st := one(t, tc.sql)
		if st.Tag != tc.tag {
			t.Errorf("%s: tag %q, want %q", tc.sql, st.Tag, tc.tag)
		}
		if len(st.Locks) != len(tc.locks) {
			t.Errorf("%s: locks %v, want %v", tc.sql, st.Locks, tc.locks)
		}
		for table, mode := range tc.locks {
			if got := st.LockOn(table); got != mode {
				t.Errorf("%s: lock on %s = %s, want %s", tc.sql, table, got, mode)
			}
		}
	}
}

func TestDefElemTrueReadsOptionWords(t *testing.T) {
	// In storage parameters, unreserved words such as on and off parse as
	// type names rather than strings.
	cases := map[string]bool{
		"ALTER TABLE t SET (autovacuum_enabled = off)":   false,
		"ALTER TABLE t SET (autovacuum_enabled = on)":    true,
		"ALTER TABLE t SET (autovacuum_enabled = false)": false,
		"ALTER TABLE t SET (autovacuum_enabled = true)":  true,
		"ALTER TABLE t SET (autovacuum_enabled = 0)":     false,
		"ALTER TABLE t SET (autovacuum_enabled)":         true,
	}
	for sql, want := range cases {
		st := one(t, sql)
		cmd := st.Node().GetAlterTableStmt().Cmds[0].GetAlterTableCmd()
		d := cmd.Def.GetList().Items[0].GetDefElem()
		if got := defElemTrue(d); got != want {
			t.Errorf("%s: defElemTrue = %v, want %v", sql, got, want)
		}
	}
}
