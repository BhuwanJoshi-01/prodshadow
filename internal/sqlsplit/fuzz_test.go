package sqlsplit

import (
	"strings"
	"testing"
)

func FuzzSplit(f *testing.F) {
	seeds := []string{
		"SELECT 1;",
		"INSERT INTO t VALUES ('a;b');",
		"DO $$ BEGIN END $$;",
		"/* a /* b */ c */ SELECT 1",
		"CREATE FUNCTION f() BEGIN ATOMIC SELECT 1; END;",
		"SELECT E'\\';';",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, script string) {
		stmts, err := Split(script)
		if err != nil {
			return
		}
		prevEnd := 0
		for _, st := range stmts {
			if st.Start < prevEnd || st.End < st.Start || st.End > len(script) {
				t.Fatalf("invalid offsets %d-%d after %d", st.Start, st.End, prevEnd)
			}
			if script[st.Start:st.End] != st.Text {
				t.Fatalf("text mismatch")
			}
			if strings.TrimLeft(st.Text, " \t\r\n\f\v") == "" {
				t.Fatalf("empty statement returned")
			}
			if st.StartLine < 1 || st.EndLine < st.StartLine {
				t.Fatalf("invalid lines %d-%d", st.StartLine, st.EndLine)
			}
			prevEnd = st.End
		}
	})
}
