package sqlsplit

import (
	"strings"
	"testing"
)

func BenchmarkSplit(b *testing.B) {
	stmt := "INSERT INTO orders (id, note) VALUES (1, 'it''s; fine'); -- comment\nCREATE FUNCTION f() RETURNS int AS $$ SELECT 1; $$ LANGUAGE sql;\n"
	script := strings.Repeat(stmt, 2000)
	b.SetBytes(int64(len(script)))
	for b.Loop() {
		if _, err := Split(script); err != nil {
			b.Fatal(err)
		}
	}
}
