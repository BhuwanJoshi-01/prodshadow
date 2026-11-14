package sqlinspect

import (
	"fmt"
	"strings"
	"testing"
)

// largeMigration builds a migration of n statements mixing the shapes the
// rules look at: new tables with foreign keys, indexes, constraints added
// NOT VALID and validated, column changes and batched updates.
func largeMigration(n int) string {
	var b strings.Builder
	b.WriteString("SET lock_timeout = '2s';\n")
	for i := range n / 6 {
		fmt.Fprintf(&b, "CREATE TABLE t%d (id bigint PRIMARY KEY, parent_id bigint REFERENCES t%d (id), note text);\n", i, max(i-1, 0))
		fmt.Fprintf(&b, "CREATE INDEX CONCURRENTLY t%d_parent_idx ON t%d (parent_id);\n", i, i)
		fmt.Fprintf(&b, "ALTER TABLE orders ADD CONSTRAINT c%d CHECK (total >= %d) NOT VALID;\n", i, i)
		fmt.Fprintf(&b, "ALTER TABLE orders VALIDATE CONSTRAINT c%d;\n", i)
		fmt.Fprintf(&b, "ALTER TABLE orders ADD COLUMN col%d text;\n", i)
		fmt.Fprintf(&b, "UPDATE orders SET col%d = note WHERE id BETWEEN %d AND %d;\n", i, i*1000, i*1000+999)
	}
	return b.String()
}

func BenchmarkParseLargeMigration(b *testing.B) {
	src := []Source{{Path: "big.sql", Content: largeMigration(600)}}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := ParseSources(src); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAnalyzeLargeMigration(b *testing.B) {
	stmts, err := ParseSources([]Source{{Path: "big.sql", Content: largeMigration(600)}})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		Analyze(stmts, Options{Transaction: TxnModeNone})
	}
}

// TestLargeMigrationIsAnalysed guards the benchmark input and checks that
// every statement is classified.
func TestLargeMigrationIsAnalysed(t *testing.T) {
	stmts := parse(t, largeMigration(60))
	if len(stmts) != 61 {
		t.Fatalf("%d statements", len(stmts))
	}
	for _, s := range stmts {
		if !s.Parsed {
			t.Fatalf("statement %s not parsed: %s", s.ID, s.ParseError)
		}
	}
	if len(Analyze(stmts, Options{Transaction: TxnModeNone})) == 0 {
		t.Fatal("expected findings")
	}
}

// largeSchema builds a schema of n tables, each referencing the previous
// one, with an index, a view and an audit trigger, and a partitioned table
// with n/10 partitions.
func largeSchema(n int) string {
	var b strings.Builder
	b.WriteString("CREATE TABLE audit (t text, at timestamptz);\n")
	b.WriteString("CREATE FUNCTION audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN INSERT INTO audit VALUES (TG_TABLE_NAME, now()); RETURN NEW; END $$;\n")
	b.WriteString("CREATE TABLE events (id bigint, at int) PARTITION BY RANGE (at);\n")
	for i := range n {
		fmt.Fprintf(&b, "CREATE TABLE t%d (id bigint PRIMARY KEY, parent_id bigint REFERENCES t%d (id), note text);\n", i, max(i-1, 0))
		fmt.Fprintf(&b, "CREATE INDEX t%d_note_idx ON t%d (note);\n", i, i)
		fmt.Fprintf(&b, "CREATE VIEW v%d AS SELECT id, note FROM t%d;\n", i, i)
		fmt.Fprintf(&b, "CREATE TRIGGER t%d_audit AFTER INSERT ON t%d FOR EACH ROW EXECUTE FUNCTION audit();\n", i, i)
		if i%10 == 0 {
			fmt.Fprintf(&b, "CREATE TABLE events_%d PARTITION OF events FOR VALUES FROM (%d) TO (%d);\n", i, i, i+10)
		}
	}
	return b.String()
}

func BenchmarkCatalogResolve(b *testing.B) {
	schema, err := ParseSources([]Source{{Path: "schema.sql", Content: largeSchema(500)}})
	if err != nil {
		b.Fatal(err)
	}
	var m strings.Builder
	for i := range 100 {
		fmt.Fprintf(&m, "INSERT INTO t%d (id, note) VALUES (1, 'x');\n", i)
		fmt.Fprintf(&m, "DELETE FROM t%d WHERE id = 1;\n", i)
		fmt.Fprintf(&m, "DROP INDEX t%d_note_idx;\n", i)
	}
	m.WriteString("ALTER TABLE events ADD COLUMN source text;\nSELECT * FROM v42;\n")
	src := []Source{{Path: "m.sql", Content: m.String()}}
	b.ReportAllocs()
	for b.Loop() {
		b.StopTimer()
		stmts, err := ParseSources(src)
		if err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
		NewCatalog(schema).Resolve(stmts)
	}
}
