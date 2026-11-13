package workload

import "testing"

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
