package sqlinspect

import (
	"strings"
	"testing"
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
