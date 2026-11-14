package sqlinspect

import (
	"errors"
	"testing"
)

func TestParseOneAndMaxParam(t *testing.T) {
	st, err := ParseOne("UPDATE orders SET status = $1 WHERE id = $2 AND $1 IS NOT NULL")
	if err != nil {
		t.Fatal(err)
	}
	if st.MaxParam() != 2 || st.Kind != KindUpdate {
		t.Fatalf("max=%d kind=%s", st.MaxParam(), st.Kind)
	}
	st, _ = ParseOne("SELECT 1")
	if st.MaxParam() != 0 {
		t.Fatal("no parameters expected")
	}
	if _, err := ParseOne("SELECT 1; SELECT 2"); err == nil {
		t.Fatal("multiple statements must be rejected")
	}
	if _, err := ParseOne("SELEC 1"); err == nil {
		t.Fatal("syntax errors must be reported")
	}
}

func TestRequireDML(t *testing.T) {
	for _, sql := range []string{"SELECT * FROM t WHERE id = $1 FOR UPDATE", "INSERT INTO t VALUES ($1)", "DELETE FROM t WHERE id = $1", "WITH x AS (SELECT 1) SELECT * FROM x", "MERGE INTO t USING s ON s.id = t.id WHEN MATCHED THEN UPDATE SET v = $1"} {
		st, err := ParseOne(sql)
		if err != nil {
			t.Fatal(err)
		}
		if err := st.RequireDML(); err != nil {
			t.Errorf("%s: %v", sql, err)
		}
	}
	for _, sql := range []string{"CREATE TABLE x (id int)", "BEGIN", "SET work_mem = '1GB'", "SELECT 1 INTO newtable", "COPY t FROM STDIN", "LOCK TABLE t"} {
		st, err := ParseOne(sql)
		if err != nil {
			t.Fatal(err)
		}
		if err := st.RequireDML(); !errors.Is(err, ErrNotDML) {
			t.Errorf("%s should be rejected, got %v", sql, err)
		}
	}
}
