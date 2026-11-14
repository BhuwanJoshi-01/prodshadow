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
