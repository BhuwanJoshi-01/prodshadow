package sqlinspect

import "testing"

func TestPS126DatabaseWideMaintenance(t *testing.T) {
	cases := []struct {
		sql  string
		want int
	}{
		{"VACUUM;", 1},
		{"VACUUM (ANALYZE);", 1},
		{"ANALYZE;", 1},
		{"VACUUM orders;", 0},
		{"ANALYZE orders;", 0},
		{"REINDEX DATABASE app;", 1},
		{"REINDEX SYSTEM;", 1},
		{"REINDEX SCHEMA CONCURRENTLY public;", 1},
		{"REINDEX TABLE orders;", 0},
		{"REINDEX INDEX CONCURRENTLY orders_pkey;", 0},
	}
	for _, tc := range cases {
		if got := findings(t, "PS126", Options{}, tc.sql); len(got) != tc.want {
			t.Errorf("%q: expected %d findings, got %d: %+v", tc.sql, tc.want, len(got), got)
		}
	}
}
