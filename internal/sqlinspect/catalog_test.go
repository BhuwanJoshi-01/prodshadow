package sqlinspect

import (
	"testing"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/pglock"
)

const catalogSchema = `
CREATE TABLE customers (id int PRIMARY KEY);
CREATE TABLE orders (id int, status text, CONSTRAINT orders_id_key UNIQUE (id));
CREATE INDEX orders_status_idx ON public.orders (status);
CREATE TABLE events (id bigint, at timestamptz) PARTITION BY RANGE (at);
CREATE TABLE events_2026 PARTITION OF events FOR VALUES FROM ('2026-01-01') TO ('2027-01-01') PARTITION BY RANGE (at);
CREATE TABLE events_2026_h1 PARTITION OF events_2026 FOR VALUES FROM ('2026-01-01') TO ('2026-07-01');
CREATE TABLE events_2027 (id bigint, at timestamptz);
ALTER TABLE events ATTACH PARTITION events_2027 FOR VALUES FROM ('2027-01-01') TO ('2028-01-01');
CREATE MATERIALIZED VIEW order_counts AS SELECT status, count(*) FROM orders JOIN customers ON true GROUP BY 1;
`

func resolved(t *testing.T, migration string) []*Statement {
	t.Helper()
	schema := parse(t, catalogSchema)
	stmts := parse(t, migration)
	NewCatalog(schema).Resolve(stmts)
	return stmts
}

func lockOn(s *Statement, table string) (TableLock, bool) {
	for _, l := range s.Locks {
		if l.Table == table {
			return l, true
		}
	}
	return TableLock{}, false
}

func TestCatalogIndexTables(t *testing.T) {
	stmts := resolved(t, `
DROP INDEX orders_status_idx;
DROP INDEX CONCURRENTLY customers_pkey;
REINDEX INDEX orders_id_key;
CREATE INDEX orders_new_idx ON orders (id);
DROP INDEX orders_new_idx;
DROP INDEX unknown_idx;
`)
	cases := []struct {
		stmt  int
		table string
		mode  pglock.Mode
		via   string
	}{
		{0, "public.orders", pglock.AccessExclusive, "index orders_status_idx"},
		{1, "customers", pglock.ShareUpdateExclusive, "index customers_pkey"},
		{2, "orders", pglock.Share, "index orders_id_key"},
		{4, "orders", pglock.AccessExclusive, "index orders_new_idx"},
	}
	for _, c := range cases {
		l, ok := lockOn(stmts[c.stmt], c.table)
		if !ok || l.Mode != c.mode || l.Via != c.via {
			t.Errorf("statement %d: lock on %s = %+v (found %v), want %s via %q", c.stmt, c.table, l, ok, c.mode, c.via)
		}
	}
	if len(stmts[5].Locks) != 1 {
		t.Errorf("an unknown index implies nothing: %+v", stmts[5].Locks)
	}
}

func TestCatalogPartitions(t *testing.T) {
	stmts := resolved(t, `
ALTER TABLE events ADD COLUMN kind text;
CREATE INDEX events_id_idx ON ONLY events (id);
LOCK TABLE events IN SHARE MODE;
ALTER TABLE events DETACH PARTITION events_2027;
TRUNCATE events_2026;
`)
	for _, p := range []string{"events_2026", "events_2026_h1", "events_2027"} {
		if l, ok := lockOn(stmts[0], p); !ok || l.Mode != pglock.AccessExclusive || l.Via != "partition of events" {
			t.Errorf("ALTER TABLE on the parent should lock %s: %+v", p, stmts[0].Locks)
		}
		if l, ok := lockOn(stmts[2], p); !ok || l.Mode != pglock.Share {
			t.Errorf("LOCK TABLE without ONLY should lock %s: %+v", p, stmts[2].Locks)
		}
	}
	if len(stmts[1].Locks) != 1 {
		t.Errorf("ON ONLY must not reach partitions: %+v", stmts[1].Locks)
	}
	if _, ok := lockOn(stmts[3], "events_2026"); ok {
		t.Errorf("DETACH PARTITION locks only the partition it names: %+v", stmts[3].Locks)
	}
	if l, ok := lockOn(stmts[4], "events_2026_h1"); !ok || l.Via != "partition of events_2026" {
		t.Errorf("TRUNCATE of a sub-partitioned table should reach its partitions: %+v", stmts[4].Locks)
	}
}

func TestCatalogMaterializedViewSources(t *testing.T) {
	stmts := resolved(t, "REFRESH MATERIALIZED VIEW CONCURRENTLY order_counts;")
	for _, table := range []string{"orders", "customers"} {
		if l, ok := lockOn(stmts[0], table); !ok || l.Mode != pglock.AccessShare || l.Via != "materialized view order_counts" {
			t.Errorf("refresh should read %s: %+v", table, stmts[0].Locks)
		}
	}
}

func TestCatalogMarksPartitionsCreatedByTheMigration(t *testing.T) {
	stmts := resolved(t, `
CREATE TABLE events_2028 PARTITION OF events FOR VALUES FROM ('2028-01-01') TO ('2029-01-01');
ALTER TABLE events ADD COLUMN source text;
`)
	if l, ok := lockOn(stmts[1], "events_2028"); !ok || l.CreatedBy != "1.1" {
		t.Errorf("the new partition is locked but was created by 1.1: %+v", stmts[1].Locks)
	}
}

func TestNilCatalogIsANoOp(t *testing.T) {
	stmts := parse(t, "DROP INDEX orders_status_idx;")
	var c *Catalog
	c.Resolve(stmts)
	if c.TableOfIndex("x") != "" || c.Partitions("x") != nil || len(stmts[0].Locks) != 1 {
		t.Fatal("a nil catalog must change nothing")
	}
}
