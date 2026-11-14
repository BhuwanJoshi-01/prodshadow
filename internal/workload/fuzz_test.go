package workload

import (
	"bytes"
	"testing"
)

func FuzzParse(f *testing.F) {
	f.Add([]byte(blueprintWorkload))
	f.Add([]byte("version: 1\nscenarios:\n  - {id: a, weight: 1, mode: read, sql: \"SELECT $1\", parameters: [{generator: zipf, min: 1, max: 2, s: 2}]}\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		s, err := Parse(data)
		if err != nil {
			return
		}
		if _, _, err := compile(s); err != nil {
			t.Fatalf("a valid workload must compile: %v", err)
		}
	})
}

// FuzzDraftFromStats checks that any exported query text produces a draft
// that is either empty or loads as a valid workload: query text comes from
// production and may contain anything.
func FuzzDraftFromStats(f *testing.F) {
	f.Add("SELECT id FROM orders WHERE id = $1", int64(10))
	f.Add("UPDATE orders SET note = 'a\"b\\\\c' WHERE id = $2 AND x = $1", int64(3))
	f.Add("WITH x AS (SELECT 1) SELECT * FROM x", int64(1))
	f.Add("INSERT INTO t VALUES ($1, $2, $3) RETURNING id", int64(7))
	f.Fuzz(func(t *testing.T, query string, calls int64) {
		d := DraftFromStats([]StatRecord{{Query: query, Calls: calls}}, DraftOptions{Name: "fuzz"})
		if len(d.Scenarios) == 0 {
			return
		}
		var buf bytes.Buffer
		if err := d.Write(&buf); err != nil {
			t.Fatal(err)
		}
		if _, err := Parse(buf.Bytes()); err != nil {
			t.Fatalf("draft for %q does not load: %v\n%s", query, err, buf.String())
		}
	})
}
