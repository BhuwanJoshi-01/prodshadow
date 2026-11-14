package sqlinspect

import (
	"slices"
	"strings"
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
	analyze := resolved(t, "ANALYZE events;\nVACUUM (FULL) events_2026;")
	if l, ok := lockOn(analyze[0], "events_2026_h1"); !ok || l.Mode != pglock.ShareUpdateExclusive {
		t.Errorf("ANALYZE should reach every partition: %+v", analyze[0].Locks)
	}
	if l, ok := lockOn(analyze[1], "events_2026_h1"); !ok || l.Mode != pglock.AccessExclusive {
		t.Errorf("VACUUM FULL should reach every partition: %+v", analyze[1].Locks)
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
	// Each statement on the original schema, except the last two, which
	// test that a dropped foreign key is forgotten.
	for _, st := range stmts[:5] {
		NewCatalog(parse(t, fkSchema)).Resolve([]*Statement{st})
	}
	NewCatalog(parse(t, fkSchema)).Resolve(stmts[5:])
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

func TestPS158ConcurrentRefresh(t *testing.T) {
	schema := parse(t, `
CREATE TABLE orders (id int, status text);
CREATE MATERIALIZED VIEW status_counts AS SELECT status, count(*) AS n FROM orders GROUP BY 1;
CREATE MATERIALIZED VIEW keyed_counts AS SELECT status, count(*) AS n FROM orders GROUP BY 1;
CREATE UNIQUE INDEX keyed_counts_status ON keyed_counts (status);
CREATE MATERIALIZED VIEW partial_counts AS SELECT status, count(*) AS n FROM orders GROUP BY 1;
CREATE UNIQUE INDEX partial_counts_status ON partial_counts (lower(status));
CREATE MATERIALIZED VIEW empty_counts AS SELECT status, count(*) AS n FROM orders GROUP BY 1 WITH NO DATA;
CREATE UNIQUE INDEX empty_counts_status ON empty_counts (status);
`)
	analyse := func(migration string) []Finding {
		stmts := parse(t, migration)
		NewCatalog(schema).Resolve(stmts)
		var out []Finding
		for _, f := range Analyze(stmts, Options{}) {
			if f.RuleID == "PS158" {
				out = append(out, f)
			}
		}
		return out
	}
	if got := analyse("REFRESH MATERIALIZED VIEW CONCURRENTLY status_counts;"); len(got) != 1 || !strings.Contains(got[0].Message, "no unique index") {
		t.Fatalf("no unique index: %+v", got)
	}
	expectCount(t, analyse("REFRESH MATERIALIZED VIEW CONCURRENTLY keyed_counts;"), 0)
	expectCount(t, analyse("REFRESH MATERIALIZED VIEW CONCURRENTLY partial_counts;"), 1)
	if got := analyse("REFRESH MATERIALIZED VIEW CONCURRENTLY empty_counts;"); len(got) != 1 || !strings.Contains(got[0].Message, "not populated") {
		t.Fatalf("unpopulated: %+v", got)
	}
	expectCount(t, analyse("REFRESH MATERIALIZED VIEW empty_counts;\nREFRESH MATERIALIZED VIEW CONCURRENTLY empty_counts;"), 0)
	expectCount(t, analyse("CREATE UNIQUE INDEX CONCURRENTLY status_counts_status ON status_counts (status);\nREFRESH MATERIALIZED VIEW CONCURRENTLY status_counts;"), 0)
	expectCount(t, analyse("REFRESH MATERIALIZED VIEW CONCURRENTLY unknown_view;"), 0)
	expectCount(t, analyse("REFRESH MATERIALIZED VIEW status_counts;"), 0)
}

func TestPS159DependentView(t *testing.T) {
	schema := parse(t, `
CREATE TABLE orders (id int, status text, amount int, note text);
CREATE TABLE customers (id int, name text);
CREATE VIEW recent_orders AS SELECT o.id, o.status FROM orders o;
CREATE MATERIALIZED VIEW totals AS SELECT status, sum(amount) FROM orders GROUP BY status;
CREATE VIEW everything AS SELECT * FROM customers;
`)
	analyse := func(migration string) []Finding {
		stmts := parse(t, migration)
		NewCatalog(schema).Resolve(stmts)
		var out []Finding
		for _, f := range Analyze(stmts, Options{}) {
			if f.RuleID == "PS159" {
				out = append(out, f)
			}
		}
		return out
	}
	if got := analyse("ALTER TABLE orders DROP COLUMN status;"); len(got) != 1 || !strings.Contains(got[0].Message, "view recent_orders, totals reads it") {
		t.Fatalf("drop column: %+v", got)
	}
	if got := analyse("ALTER TABLE orders ALTER COLUMN amount TYPE bigint;"); len(got) != 1 || !strings.Contains(got[0].Message, "Changing the type of column amount of orders fails: view totals") {
		t.Fatalf("alter type: %+v", got)
	}
	expectCount(t, analyse("ALTER TABLE orders DROP COLUMN note;"), 0)
	expectCount(t, analyse("ALTER TABLE orders DROP COLUMN status CASCADE;"), 0)
	expectCount(t, analyse("ALTER TABLE customers DROP COLUMN name;"), 1)
	expectCount(t, analyse("DROP TABLE customers;"), 1)
	expectCount(t, analyse("DROP VIEW recent_orders;\nDROP MATERIALIZED VIEW totals;\nALTER TABLE orders DROP COLUMN status;"), 0)
	expectCount(t, analyse("ALTER TABLE orders RENAME COLUMN status TO state;"), 0)
}

func TestPS160ViewReplace(t *testing.T) {
	schema := parse(t, `
CREATE TABLE orders (id int, status text, amount int);
CREATE VIEW recent_orders AS SELECT o.id, o.status AS state, count(*) FROM orders o GROUP BY 1, 2;
CREATE VIEW all_orders AS SELECT * FROM orders;
`)
	analyse := func(migration string) []Finding {
		stmts := parse(t, migration)
		NewCatalog(schema).Resolve(stmts)
		var out []Finding
		for _, f := range Analyze(stmts, Options{}) {
			if f.RuleID == "PS160" {
				out = append(out, f)
			}
		}
		return out
	}
	if got := analyse("CREATE OR REPLACE VIEW recent_orders AS SELECT o.id, o.status AS state FROM orders o;"); len(got) != 1 || !strings.Contains(got[0].Message, "drops column count") {
		t.Fatalf("dropped column: %+v", got)
	}
	if got := analyse("CREATE OR REPLACE VIEW recent_orders AS SELECT o.status AS state, o.id, count(*) FROM orders o GROUP BY 1, 2;"); len(got) != 1 || !strings.Contains(got[0].Message, "column 1 is state instead of id") {
		t.Fatalf("reordered: %+v", got)
	}
	expectCount(t, analyse("CREATE OR REPLACE VIEW recent_orders AS SELECT o.id, o.status AS state, count(*), max(o.amount) FROM orders o GROUP BY 1, 2;"), 0)
	expectCount(t, analyse("CREATE OR REPLACE VIEW all_orders AS SELECT id, status FROM orders;"), 1)
	expectCount(t, analyse("CREATE OR REPLACE VIEW all_orders (id, status, amount, note) AS SELECT id, status, amount, 'x' FROM orders;"), 0)
	expectCount(t, analyse("CREATE VIEW new_view AS SELECT id FROM orders;"), 0)
	expectCount(t, analyse("DROP VIEW recent_orders;\nCREATE OR REPLACE VIEW recent_orders AS SELECT id FROM orders;"), 0)
}

func TestCatalogDefaultPartition(t *testing.T) {
	schema := parse(t, `
CREATE TABLE events (id bigint, at timestamptz) PARTITION BY RANGE (at);
CREATE TABLE events_2026 PARTITION OF events FOR VALUES FROM ('2026-01-01') TO ('2027-01-01');
CREATE TABLE events_other (id bigint, at timestamptz);
ALTER TABLE events ATTACH PARTITION events_other DEFAULT;
CREATE TABLE logs (id bigint, at timestamptz) PARTITION BY RANGE (at);
`)
	stmts := parse(t, `
CREATE TABLE events_2027 PARTITION OF events FOR VALUES FROM ('2027-01-01') TO ('2028-01-01');
ALTER TABLE events DETACH PARTITION events_2026;
ALTER TABLE events DETACH PARTITION events_2027 CONCURRENTLY;
CREATE TABLE logs_2027 PARTITION OF logs FOR VALUES FROM ('2027-01-01') TO ('2028-01-01');
`)
	NewCatalog(schema).Resolve(stmts)
	for _, i := range []int{0, 1} {
		if l, ok := lockOn(stmts[i], "events_other"); !ok || l.Mode != pglock.AccessExclusive || l.Via != "default partition of events" {
			t.Errorf("statement %d should lock the default partition: %+v", i, stmts[i].Locks)
		}
	}
	if len(stmts[3].Locks) != 1 {
		t.Errorf("logs has no default partition: %+v", stmts[3].Locks)
	}
	var got []Finding
	for _, f := range Analyze(stmts, Options{}) {
		if f.RuleID == "PS161" {
			got = append(got, f)
		}
	}
	if len(got) != 1 || got[0].StatementID != "1.3" || !strings.Contains(got[0].Message, "DEFAULT partition (events_other)") {
		t.Fatalf("PS161 = %+v", got)
	}
}

func TestCatalogTriggerLocks(t *testing.T) {
	schema := parse(t, `
CREATE TABLE orders (id int, status text, customer_id int);
CREATE TABLE order_audit (order_id int, at timestamptz);
CREATE TABLE customers (id int, paid_orders int);
CREATE FUNCTION audit_orders() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  INSERT INTO order_audit (order_id, at) VALUES (NEW.id, now());
  IF NEW.status = 'paid' THEN
    UPDATE customers SET paid_orders = paid_orders + 1 WHERE id = NEW.customer_id;
  END IF;
  PERFORM 1 FROM customers WHERE id = NEW.customer_id FOR UPDATE;
  RETURN NEW;
END $$;
CREATE FUNCTION log_truncate() RETURNS trigger LANGUAGE sql AS 'INSERT INTO order_audit VALUES (0, now())';
CREATE TRIGGER orders_audit AFTER INSERT OR UPDATE ON orders FOR EACH ROW EXECUTE FUNCTION audit_orders();
CREATE TRIGGER orders_truncated AFTER TRUNCATE ON orders FOR EACH STATEMENT EXECUTE FUNCTION log_truncate();
`)
	stmts := parse(t, `
INSERT INTO orders VALUES (1, 'new', 1);
UPDATE orders SET status = 'paid' WHERE id = 1;
DELETE FROM orders WHERE id = 1;
TRUNCATE orders;
DROP TRIGGER orders_audit ON orders;
INSERT INTO orders VALUES (2, 'new', 1);
`)
	NewCatalog(schema).Resolve(stmts)
	for _, i := range []int{0, 1} {
		if l, ok := lockOn(stmts[i], "order_audit"); !ok || l.Mode != pglock.RowExclusive || l.Via != "trigger orders_audit" {
			t.Errorf("statement %d should write order_audit through the trigger: %+v", i, stmts[i].Locks)
		}
		if l, ok := lockOn(stmts[i], "customers"); !ok || l.Mode != pglock.RowExclusive {
			t.Errorf("statement %d: the trigger updates customers: %+v", i, stmts[i].Locks)
		}
	}
	if _, ok := lockOn(stmts[2], "order_audit"); ok {
		t.Error("no trigger fires on DELETE")
	}
	if _, ok := lockOn(stmts[3], "order_audit"); !ok {
		t.Errorf("the TRUNCATE trigger writes order_audit: %+v", stmts[3].Locks)
	}
	if _, ok := lockOn(stmts[5], "customers"); ok {
		t.Error("the dropped trigger no longer fires")
	}
}

func TestCatalogViewBaseLocks(t *testing.T) {
	schema := parse(t, `
CREATE TABLE orders (id int, status text, customer_id int);
CREATE TABLE customers (id int, name text);
CREATE VIEW open_orders AS SELECT id, status FROM orders WHERE status <> 'closed';
CREATE VIEW urgent_orders AS SELECT id FROM open_orders WHERE status = 'urgent';
CREATE VIEW order_names AS SELECT o.id, c.name FROM orders o JOIN customers c ON c.id = o.customer_id;
`)
	stmts := parse(t, `
SELECT * FROM urgent_orders;
UPDATE open_orders SET status = 'x' WHERE id = 1;
SELECT * FROM order_names;
LOCK TABLE order_names IN SHARE MODE;
DROP VIEW open_orders CASCADE;
`)
	NewCatalog(schema).Resolve(stmts)
	cases := []struct {
		stmt  int
		table string
		mode  pglock.Mode
		via   string
	}{
		{0, "open_orders", pglock.AccessShare, "view urgent_orders"},
		{0, "orders", pglock.AccessShare, "view open_orders"},
		{1, "orders", pglock.RowExclusive, "view open_orders"},
		{2, "customers", pglock.AccessShare, "view order_names"},
		{3, "customers", pglock.Share, "view order_names"},
	}
	for _, c := range cases {
		if l, ok := lockOn(stmts[c.stmt], c.table); !ok || l.Mode != c.mode || l.Via != c.via {
			t.Errorf("statement %d: lock on %s = %+v (found %v), want %s via %q", c.stmt, c.table, l, ok, c.mode, c.via)
		}
	}
	if _, ok := lockOn(stmts[4], "orders"); ok {
		t.Error("dropping a view does not lock its tables")
	}
}

func TestCatalogForeignKeyColumnType(t *testing.T) {
	stmts := parse(t, `
ALTER TABLE customers ALTER COLUMN id TYPE bigint;
ALTER TABLE invoices ALTER COLUMN customer_id TYPE bigint;
ALTER TABLE customers ALTER COLUMN email TYPE varchar(320);
`)
	NewCatalog(parse(t, fkSchema)).Resolve(stmts)
	if l, ok := lockOn(stmts[0], "invoices"); !ok || l.Mode != pglock.AccessExclusive {
		t.Errorf("retyping the referenced key re-creates the foreign key on invoices: %+v", stmts[0].Locks)
	}
	if l, ok := lockOn(stmts[1], "customers"); !ok || l.Mode != pglock.AccessExclusive {
		t.Errorf("retyping the referencing column re-creates the triggers on customers: %+v", stmts[1].Locks)
	}
	if len(stmts[2].Locks) != 1 {
		t.Errorf("a column outside any foreign key changes nothing else: %+v", stmts[2].Locks)
	}
}

func TestCatalogQueryPartitions(t *testing.T) {
	stmts := resolved(t, `
SELECT * FROM events WHERE at >= '2026-03-01';
INSERT INTO events VALUES (1, '2026-03-01');
SELECT * FROM ONLY events;
ALTER TABLE events_2026_h1 ADD COLUMN note text;
`)
	if l, ok := lockOn(stmts[0], "events_2026_h1"); !ok || l.Mode != pglock.AccessShare || l.Via != "partition of events" {
		t.Errorf("a query may read any partition: %+v", stmts[0].Locks)
	}
	if l, ok := lockOn(stmts[1], "events_2027"); !ok || l.Mode != pglock.RowExclusive {
		t.Errorf("an insert may be routed to any partition: %+v", stmts[1].Locks)
	}
	if len(stmts[2].Locks) != 1 {
		t.Errorf("ONLY reads the parent alone: %+v", stmts[2].Locks)
	}
	// A query on the parent conflicts with DDL on one partition.
	if got := PredictConflicts(stmts[3:], stmts[0]); len(got) != 1 || got[0].Table != "events_2026_h1" {
		t.Errorf("conflicts = %+v", got)
	}
}

func TestCatalogFunctionCalls(t *testing.T) {
	schema := parse(t, `
CREATE TABLE tags (id int, name text);
CREATE TABLE staging (id int);
CREATE FUNCTION touch_tags() RETURNS int LANGUAGE sql AS 'UPDATE tags SET name = name WHERE id = 0; SELECT 1';
CREATE PROCEDURE clear_staging() LANGUAGE plpgsql AS $$ BEGIN DELETE FROM staging WHERE id < 0; END $$;
CREATE PROCEDURE run_dynamic(t text) LANGUAGE plpgsql AS $$ BEGIN EXECUTE 'TRUNCATE ' || t; END $$;
`)
	stmts := parse(t, `
SELECT touch_tags();
CALL clear_staging();
CALL run_dynamic('staging');
CALL unknown_proc();
ALTER TABLE staging ADD COLUMN n int DEFAULT touch_tags();
`)
	NewCatalog(schema).Resolve(stmts)
	if l, ok := lockOn(stmts[0], "tags"); !ok || l.Mode != pglock.RowExclusive || l.Via != "function touch_tags" {
		t.Errorf("SELECT touch_tags() updates tags: %+v", stmts[0].Locks)
	}
	if l, ok := lockOn(stmts[1], "staging"); !ok || l.Mode != pglock.RowExclusive {
		t.Errorf("CALL clear_staging() deletes from staging: %+v", stmts[1].Locks)
	}
	if _, ok := lockOn(stmts[4], "tags"); ok {
		t.Errorf("a DEFAULT only refers to the function: %+v", stmts[4].Locks)
	}
	var ps117 []string
	for _, f := range Analyze(stmts, Options{}) {
		if f.RuleID == "PS117" {
			ps117 = append(ps117, f.StatementID)
		}
	}
	if !slices.Equal(ps117, []string{"1.3", "1.4"}) {
		t.Errorf("PS117 should report only the dynamic and the unknown procedure: %v", ps117)
	}
}

func TestCatalogRuleLocks(t *testing.T) {
	schema := parse(t, `
CREATE TABLE staging (id int);
CREATE TABLE staging_log (id int);
CREATE TABLE archive (id int);
CREATE RULE staging_logged AS ON INSERT TO staging DO ALSO INSERT INTO staging_log VALUES (NEW.id);
CREATE RULE staging_archived AS ON DELETE TO staging DO INSTEAD INSERT INTO archive VALUES (OLD.id);
`)
	stmts := parse(t, "INSERT INTO staging VALUES (1);\nDELETE FROM staging WHERE id = 1;\nUPDATE staging SET id = 2;\n")
	NewCatalog(schema).Resolve(stmts)
	if l, ok := lockOn(stmts[0], "staging_log"); !ok || l.Mode != pglock.RowExclusive || l.Via != "rule staging_logged" {
		t.Errorf("the INSERT rule writes staging_log: %+v", stmts[0].Locks)
	}
	if l, ok := lockOn(stmts[1], "archive"); !ok || l.Via != "rule staging_archived" {
		t.Errorf("the DELETE rule writes archive: %+v", stmts[1].Locks)
	}
	if len(stmts[2].Locks) != 1 {
		t.Errorf("no rule on UPDATE: %+v", stmts[2].Locks)
	}
}

func TestCatalogFollowsRenames(t *testing.T) {
	schema := parse(t, `
CREATE TABLE customers (id int PRIMARY KEY);
CREATE TABLE invoices (id int, customer_id int CONSTRAINT invoices_customer_fk REFERENCES customers);
CREATE TABLE events (id int, at date) PARTITION BY RANGE (at);
CREATE TABLE events_2026 PARTITION OF events FOR VALUES FROM ('2026-01-01') TO ('2027-01-01');
CREATE TABLE orders (id int, status text);
CREATE INDEX orders_status_idx ON orders (status);
CREATE VIEW open_orders AS SELECT id, status FROM orders;
`)
	stmts := parse(t, `
ALTER TABLE customers RENAME TO clients;
ALTER TABLE invoices DROP CONSTRAINT invoices_customer_fk;
ALTER TABLE events RENAME TO events_v2;
ALTER TABLE events_v2 ADD COLUMN source text;
ALTER INDEX orders_status_idx RENAME TO orders_state_idx;
DROP INDEX orders_state_idx;
ALTER TABLE orders RENAME COLUMN status TO state;
ALTER TABLE orders DROP COLUMN state;
`)
	NewCatalog(schema).Resolve(stmts)
	if l, ok := lockOn(stmts[1], "clients"); !ok || l.Mode != pglock.AccessExclusive {
		t.Errorf("the foreign key now references clients: %+v", stmts[1].Locks)
	}
	if _, ok := lockOn(stmts[3], "events_2026"); !ok {
		t.Errorf("the renamed table keeps its partitions: %+v", stmts[3].Locks)
	}
	if _, ok := lockOn(stmts[5], "orders"); !ok {
		t.Errorf("the renamed index still belongs to orders: %+v", stmts[5].Locks)
	}
	var ps159 int
	for _, f := range Analyze(stmts, Options{}) {
		if f.RuleID == "PS159" && f.StatementID == "1.8" {
			ps159++
		}
	}
	if ps159 != 1 {
		t.Error("the view reads the renamed column, so dropping it fails")
	}
}

func TestPerTableRulesIgnoreImpliedLocks(t *testing.T) {
	stmts := parse(t, "TRUNCATE invoices CASCADE;\nDROP TABLE line_items;\nLOCK TABLE invoices IN SHARE MODE;\n")
	NewCatalog(parse(t, fkSchema)).Resolve(stmts)
	var ps109 []string
	locks := 0
	for _, f := range Analyze(stmts, Options{}) {
		switch f.RuleID {
		case "PS109":
			ps109 = append(ps109, f.StatementID+" "+f.Message[:strings.Index(f.Message, " permanently")])
		case "PS112":
			locks++
		}
	}
	want := []string{"1.1 Truncating invoices", "1.1 Truncating line_items (through CASCADE, foreign key line_items_invoice_fk)", "1.1 Truncating notes (through CASCADE, foreign key notes_line_item_fk)", "1.2 Dropping line_items"}
	if !slices.Equal(ps109, want) {
		t.Errorf("PS109 = %q, want %q", ps109, want)
	}
	if locks != 1 {
		t.Errorf("LOCK TABLE names one table: %d findings", locks)
	}
}

func TestCatalogForgetsDroppedObjects(t *testing.T) {
	analyse := func(migration string) []Finding {
		stmts := parse(t, migration)
		NewCatalog(parse(t, fkSchema)).Resolve(stmts)
		var out []Finding
		for _, f := range Analyze(stmts, Options{}) {
			if f.RuleID == "PS157" {
				out = append(out, f)
			}
		}
		return out
	}
	expectCount(t, analyse("DROP TABLE notes;\nDROP TABLE line_items;\nDROP TABLE invoices;\nDROP TABLE customers;"), 0)
	if got := analyse("DROP TABLE customers;"); len(got) != 1 || !strings.Contains(got[0].Message, "refuses to drop a table") {
		t.Fatalf("PS157 = %+v", got)
	}
	stmts := parse(t, "DROP INDEX invoices_pkey;\nDROP INDEX invoices_pkey;")
	NewCatalog(parse(t, fkSchema)).Resolve(stmts)
	if len(stmts[1].Locks) != 1 {
		t.Errorf("a dropped index has no table any more: %+v", stmts[1].Locks)
	}
}

func TestCatalogDropColumnAndSetSchema(t *testing.T) {
	schema := parse(t, `
CREATE TABLE customers (id int PRIMARY KEY);
CREATE TABLE tickets (id bigserial, customer_id int REFERENCES customers, title text);
`)
	stmts := parse(t, `
ALTER TABLE tickets DROP COLUMN id;
INSERT INTO tickets (title) VALUES ('x');
ALTER TABLE tickets DROP COLUMN customer_id;
ALTER TABLE customers SET SCHEMA archive;
DROP TABLE archive.customers;
`)
	NewCatalog(schema).Resolve(stmts)
	if _, ok := lockOn(stmts[1], "tickets_id_seq"); ok {
		t.Errorf("the serial column was dropped: %+v", stmts[1].Locks)
	}
	if _, ok := lockOn(stmts[1], "customers"); !ok {
		t.Errorf("the foreign key still exists at statement 1.2: %+v", stmts[1].Locks)
	}
	var ps157 int
	for _, f := range Analyze(stmts, Options{}) {
		if f.RuleID == "PS157" {
			ps157++
		}
	}
	if ps157 != 0 {
		t.Error("the foreign key went with its column, so archive.customers can be dropped")
	}
}

// A PL/pgSQL function without a body makes libpg_query abort the process
// (found by FuzzCatalogResolve); the catalog must not hand it over.
func TestCatalogSkipsFunctionsWithoutBody(t *testing.T) {
	c := NewCatalog(parse(t, "CREATE FUNCTION f() RETURNS trigger LANGUAGE plpgsql;\nCREATE PROCEDURE p() LANGUAGE plpgsql;"))
	if len(c.functionLocks) != 0 {
		t.Fatalf("functions without a body have no locks: %+v", c.functionLocks)
	}
}
