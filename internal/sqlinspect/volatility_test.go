package sqlinspect

import (
	"strings"
	"testing"
)

func addedColumn(t *testing.T, sql string) (string, bool) {
	t.Helper()
	st := one(t, sql)
	cmd := st.Node().GetAlterTableStmt().Cmds[0].GetAlterTableCmd()
	return columnRewrite(cmd.Def.GetColumnDef())
}

func TestColumnRewrite(t *testing.T) {
	cases := []struct {
		sql      string
		contains string
		certain  bool
	}{
		{"ALTER TABLE t ADD COLUMN c int", "", true},
		{"ALTER TABLE t ADD COLUMN c int DEFAULT 0", "", true},
		{"ALTER TABLE t ADD COLUMN c timestamptz DEFAULT now()", "", true},
		{"ALTER TABLE t ADD COLUMN c text DEFAULT lower('X')", "", true},
		{"ALTER TABLE t ADD COLUMN c uuid DEFAULT gen_random_uuid()", "gen_random_uuid", true},
		{"ALTER TABLE t ADD COLUMN c timestamptz DEFAULT clock_timestamp()", "clock_timestamp", true},
		{"ALTER TABLE t ADD COLUMN c bigserial", "serial", true},
		{"ALTER TABLE t ADD COLUMN c int GENERATED ALWAYS AS IDENTITY", "identity", true},
		{"ALTER TABLE t ADD COLUMN c int GENERATED ALWAYS AS (a * 2) STORED", "generated", true},
		{"ALTER TABLE t ADD COLUMN c text DEFAULT my_func()", "my_func", false},
	}
	for _, c := range cases {
		got, certain := addedColumn(t, c.sql)
		if c.contains == "" && got != "" {
			t.Errorf("%s: unexpected rewrite reason %q", c.sql, got)
		}
		if c.contains != "" && !strings.Contains(got, c.contains) {
			t.Errorf("%s: reason %q should mention %q", c.sql, got, c.contains)
		}
		if certain != c.certain {
			t.Errorf("%s: certain = %v", c.sql, certain)
		}
	}
}
