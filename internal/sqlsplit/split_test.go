package sqlsplit

import (
	"errors"
	"reflect"
	"testing"
)

func texts(t *testing.T, script string) []string {
	t.Helper()
	stmts, err := Split(script)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	out := make([]string, len(stmts))
	for i, s := range stmts {
		out[i] = s.Text
		if script[s.Start:s.End] != s.Text {
			t.Fatalf("offsets do not match text for %q", s.Text)
		}
	}
	return out
}

func TestSplitBasic(t *testing.T) {
	got := texts(t, "SELECT 1; SELECT 2 ;\n\n;;SELECT 3")
	want := []string{"SELECT 1", "SELECT 2", "SELECT 3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
}

func TestSplitIgnoresSemicolonsInLiterals(t *testing.T) {
	script := `INSERT INTO t VALUES ('a;b', 'it''s; fine');
SELECT "weird;name" FROM t;
SELECT E'esc\'; still';
CREATE FUNCTION f() RETURNS int AS $body$ SELECT 1; SELECT 2; $body$ LANGUAGE sql;
DO $$ BEGIN PERFORM 1; END $$;
SELECT 1 -- trailing; comment
;
SELECT /* a; /* nested; */ b; */ 2;`
	got := texts(t, script)
	if len(got) != 7 {
		t.Fatalf("expected 7 statements, got %d: %q", len(got), got)
	}
	if got[3] != "CREATE FUNCTION f() RETURNS int AS $body$ SELECT 1; SELECT 2; $body$ LANGUAGE sql" {
		t.Fatalf("dollar body split incorrectly: %q", got[3])
	}
	if got[5] != "SELECT 1" {
		t.Fatalf("trailing comment should not be part of text: %q", got[5])
	}
}

func TestSplitPositionalParametersAreNotDollarQuotes(t *testing.T) {
	got := texts(t, "UPDATE t SET a = $1 WHERE id = $2; SELECT 1")
	if len(got) != 2 {
		t.Fatalf("got %q", got)
	}
}

func TestSplitLineNumbersAndLeadingComments(t *testing.T) {
	script := "-- migration 42\n-- prodshadow:ignore PS101 reason=\"small table\"\nCREATE INDEX idx\n  ON t (a);\n\n/* second */\nALTER TABLE t\nADD COLUMN b int;\n"
	stmts, err := Split(script)
	if err != nil {
		t.Fatal(err)
	}
	if len(stmts) != 2 {
		t.Fatalf("got %d statements", len(stmts))
	}
	first := stmts[0]
	if first.StartLine != 3 || first.EndLine != 4 {
		t.Fatalf("first lines = %d-%d", first.StartLine, first.EndLine)
	}
	if !reflect.DeepEqual(first.LeadingComments, []string{"migration 42", `prodshadow:ignore PS101 reason="small table"`}) {
		t.Fatalf("comments = %q", first.LeadingComments)
	}
	if stmts[1].StartLine != 7 || stmts[1].EndLine != 8 || stmts[1].LeadingComments[0] != "second" {
		t.Fatalf("second = %+v", stmts[1])
	}
}

func TestSplitBeginAtomic(t *testing.T) {
	script := `CREATE FUNCTION f(x int) RETURNS int LANGUAGE sql
BEGIN ATOMIC
  SELECT CASE WHEN x > 0 THEN 1 ELSE 0 END;
  SELECT 2;
END;
SELECT 3;`
	got := texts(t, script)
	if len(got) != 2 || got[1] != "SELECT 3" {
		t.Fatalf("got %q", got)
	}
}

func TestSplitErrors(t *testing.T) {
	cases := map[string]error{
		"SELECT 'open":             ErrUnterminated,
		"SELECT \"open":            ErrUnterminated,
		"SELECT 1 /* open":         ErrUnterminated,
		"DO $x$ BEGIN END":         ErrUnterminated,
		"SELECT 1;\n\\i other.sql": ErrMetaCommand,
	}
	for script, want := range cases {
		_, err := Split(script)
		if !errors.Is(err, want) {
			t.Errorf("Split(%q) error = %v, want %v", script, err, want)
		}
		var se *SyntaxError
		if !errors.As(err, &se) || se.Line < 1 {
			t.Errorf("Split(%q) should return a located SyntaxError, got %v", script, err)
		}
	}
}

func TestSplitEmpty(t *testing.T) {
	for _, s := range []string{"", "   \n", "-- only a comment\n", ";;;"} {
		stmts, err := Split(s)
		if err != nil || len(stmts) != 0 {
			t.Errorf("Split(%q) = %v, %v", s, stmts, err)
		}
	}
}
