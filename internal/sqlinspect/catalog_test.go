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

func TestCatalogPartitionedMaintenanceRunsOutsideTransactions(t *testing.T) {
	stmts := resolved(t, "REINDEX TABLE events;\nCLUSTER events USING events_at_idx;\nREINDEX TABLE orders;\n")
	for _, s := range stmts[:2] {
		if s.Txn != TxnNotInBlock {
			t.Errorf("%s on a partitioned table cannot run in a transaction block", s.Tag)
		}
		if _, ok := lockOn(s, "events_2026_h1"); !ok {
			t.Errorf("%s should lock every partition: %+v", s.Tag, s.Locks)
		}
	}
	if stmts[2].Txn != TxnAny {
		t.Error("REINDEX TABLE on a plain table can run in a transaction block")
	}
}

func TestCatalogInsertSequences(t *testing.T) {
	schema := parse(t, `
CREATE TABLE tickets (id bigserial PRIMARY KEY, title text);
CREATE TABLE invoices (id bigint GENERATED ALWAYS AS IDENTITY, number bigint DEFAULT nextval('invoice_numbers'), note text);
CREATE TABLE notes (id int, body text);
ALTER TABLE notes ALTER COLUMN id SET DEFAULT nextval('public.note_ids'::regclass);
`)
	stmts := parse(t, `
INSERT INTO tickets (title) VALUES ('a');
INSERT INTO tickets (id, title) VALUES (5, 'a');
INSERT INTO tickets VALUES (DEFAULT, 'a');
INSERT INTO tickets VALUES (7, 'a');
INSERT INTO invoices (note) VALUES ('x');
INSERT INTO invoices (number, note) VALUES (1, 'x');
INSERT INTO notes (body) SELECT 'x';
INSERT INTO tickets DEFAULT VALUES;
`)
	NewCatalog(schema).Resolve(stmts)
	cases := []struct {
		stmt int
		seq  string
		want bool
	}{
		{0, "tickets_id_seq", true},
		{1, "tickets_id_seq", false},
		{2, "tickets_id_seq", true},
		{3, "tickets_id_seq", false},
		{4, "invoices_id_seq", true},
		{4, "invoice_numbers", true},
		{5, "invoices_id_seq", true},
		{5, "invoice_numbers", false},
		{6, "public.note_ids", true},
		{7, "tickets_id_seq", true},
	}
	for _, c := range cases {
		l, ok := lockOn(stmts[c.stmt], c.seq)
		if ok != c.want || (ok && l.Mode != pglock.RowExclusive) {
			t.Errorf("statement %d: lock on %s = %+v (found %v), want found=%v", c.stmt, c.seq, l, ok, c.want)
		}
	}
	if l, _ := lockOn(stmts[0], "tickets_id_seq"); l.Via != "column tickets.id" {
		t.Errorf("via = %q", l.Via)
	}
}

const fkSchema = `
CREATE TABLE customers (id int PRIMARY KEY, email text);
CREATE TABLE invoices (id int PRIMARY KEY, customer_id int REFERENCES customers);
CREATE TABLE line_items (id int, invoice_id int, CONSTRAINT line_items_invoice_fk FOREIGN KEY (invoice_id) REFERENCES invoices (id) ON DELETE CASCADE);
CREATE TABLE notes (id int, line_item_id int);
ALTER TABLE line_items ADD PRIMARY KEY (id);
ALTER TABLE notes ADD CONSTRAINT notes_line_item_fk FOREIGN KEY (line_item_id) REFERENCES line_items ON DELETE SET NULL;
`

func TestCatalogForeignKeyDMLLocks(t *testing.T) {
	stmts := parse(t, `
INSERT INTO invoices VALUES (1, 1);
UPDATE invoices SET customer_id = 2 WHERE id = 1;
UPDATE customers SET email = 'x' WHERE id = 1;
UPDATE customers SET id = 7 WHERE id = 1;
DELETE FROM invoices WHERE id = 1;
`)
	NewCatalog(parse(t, fkSchema)).Resolve(stmts)
	cases := []struct {
		stmt  int
		table string
		mode  pglock.Mode
		via   string
	}{
		{0, "customers", pglock.RowShare, "foreign key invoices_customer_id_fkey"},
		{1, "customers", pglock.RowShare, "foreign key invoices_customer_id_fkey"},
		{3, "invoices", pglock.RowShare, "foreign key invoices_customer_id_fkey"},
		{4, "line_items", pglock.RowExclusive, "foreign key line_items_invoice_fk"},
		{4, "notes", pglock.RowExclusive, "foreign key notes_line_item_fk"},
	}
	for _, c := range cases {
		l, ok := lockOn(stmts[c.stmt], c.table)
		if !ok || l.Mode != c.mode || l.Via != c.via {
			t.Errorf("statement %d: lock on %s = %+v (found %v), want %s via %q", c.stmt, c.table, l, ok, c.mode, c.via)
		}
	}
	if _, ok := lockOn(stmts[2], "invoices"); ok {
		t.Error("updating a column no foreign key refers to runs no referential trigger")
	}
}

func TestCatalogForeignKeyDDLLocks(t *testing.T) {
	stmts := parse(t, `
ALTER TABLE invoices VALIDATE CONSTRAINT invoices_customer_id_fkey;
ALTER TABLE line_items DROP COLUMN invoice_id;
DROP TABLE customers CASCADE;
TRUNCATE invoices CASCADE;
ALTER TABLE line_items DROP CONSTRAINT line_items_pkey CASCADE;
ALTER TABLE invoices DROP CONSTRAINT invoices_customer_id_fkey;
DROP TABLE invoices;
`)
	NewCatalog(parse(t, fkSchema)).Resolve(stmts)
	cases := []struct {
		stmt  int
		table string
		mode  pglock.Mode
	}{
		{0, "customers", pglock.RowShare},
		{1, "invoices", pglock.AccessExclusive},
		{2, "invoices", pglock.AccessExclusive},
		{3, "line_items", pglock.AccessExclusive},
		{3, "notes", pglock.AccessExclusive},
		{4, "notes", pglock.AccessExclusive},
		{5, "customers", pglock.AccessExclusive},
	}
	for _, c := range cases {
		if l, ok := lockOn(stmts[c.stmt], c.table); !ok || l.Mode != c.mode {
			t.Errorf("statement %d: lock on %s = %+v (found %v), want %s", c.stmt, c.table, l, ok, c.mode)
		}
	}
	if _, ok := lockOn(stmts[6], "customers"); ok {
		t.Error("the foreign key to customers was dropped by an earlier statement")
	}
}
