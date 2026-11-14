package sqlinspect

import (
	"strings"
	"testing"

	"github.com/BhuwanJoshi-01/prodshadow/internal/evidence"
)

func TestPS124CascadeDrop(t *testing.T) {
	cases := []struct {
		sql  string
		want int
	}{
		{"DROP TABLE accounts CASCADE;", 1},
		{"DROP TABLE accounts, ledgers CASCADE;", 2},
		{"DROP TABLE accounts;", 0},
		{"DROP VIEW account_totals CASCADE;", 1},
		{"DROP SCHEMA legacy CASCADE;", 1},
		{"DROP TYPE mood CASCADE;", 1},
		{"DROP FUNCTION f(int) CASCADE;", 1},
		{"TRUNCATE accounts CASCADE;", 1},
		{"TRUNCATE accounts;", 0},
		{"ALTER TABLE accounts DROP COLUMN legacy_id CASCADE;", 1},
		{"ALTER TABLE accounts DROP CONSTRAINT accounts_owner_fk CASCADE;", 1},
		{"ALTER TABLE accounts DROP COLUMN legacy_id;", 0},
		{"CREATE TABLE scratch (id int);\nDROP TABLE scratch CASCADE;", 0},
	}
	for _, tc := range cases {
		got := findings(t, "PS124", Options{}, tc.sql)
		if len(got) != tc.want {
			t.Errorf("%q: expected %d findings, got %d: %+v", tc.sql, tc.want, len(got), got)
		}
	}
	got := findings(t, "PS124", Options{}, "TRUNCATE accounts CASCADE;")
	if !strings.Contains(got[0].Message, "foreign key") {
		t.Errorf("truncate message should explain the foreign key cascade: %s", got[0].Message)
	}
}

func TestPS157ReferencedTableRemoved(t *testing.T) {
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
	got := analyse("DROP TABLE customers;")
	if len(got) != 1 || got[0].Severity != evidence.Critical || !strings.Contains(got[0].Message, "invoices.invoices_customer_id_fkey still references") {
		t.Fatalf("PS157 = %+v", got)
	}
	expectCount(t, analyse("TRUNCATE invoices;"), 1)
	expectCount(t, analyse("TRUNCATE invoices, line_items, notes;"), 0)
	expectCount(t, analyse("DROP TABLE customers CASCADE;"), 0)
	expectCount(t, analyse("ALTER TABLE invoices DROP CONSTRAINT invoices_customer_id_fkey;\nDROP TABLE customers;"), 0)
	expectCount(t, analyse("DROP TABLE notes;"), 0)
	// Without the schema nothing is known.
	expectCount(t, findings(t, "PS157", Options{}, "DROP TABLE customers;"), 0)
}
