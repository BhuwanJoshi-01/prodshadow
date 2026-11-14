package sqlinspect

import "testing"

func TestPS125IntegrityChecksDisabled(t *testing.T) {
	cases := []struct {
		sql  string
		want int
	}{
		{"SET session_replication_role = replica;", 1},
		{"SET LOCAL session_replication_role TO 'replica';", 1},
		{"SET session_replication_role = origin;", 0},
		{"ALTER TABLE orders DISABLE TRIGGER ALL;", 1},
		{"ALTER TABLE orders DISABLE TRIGGER USER;", 0},
		{"ALTER TABLE orders DISABLE TRIGGER audit_orders;", 0},
		{"ALTER TABLE orders ENABLE TRIGGER ALL;", 0},
		{"CREATE TABLE staging (id int);\nALTER TABLE staging DISABLE TRIGGER ALL;", 0},
	}
	for _, tc := range cases {
		if got := findings(t, "PS125", Options{}, tc.sql); len(got) != tc.want {
			t.Errorf("%q: expected %d findings, got %d: %+v", tc.sql, tc.want, len(got), got)
		}
	}
}
