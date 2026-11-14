package sqlinspect

import "testing"

func TestPS142AutovacuumDisabled(t *testing.T) {
	cases := map[string]int{
		"ALTER TABLE events SET (autovacuum_enabled = false);":                                   1,
		"ALTER TABLE events SET (toast.autovacuum_enabled = off);":                               1,
		"ALTER TABLE events SET (autovacuum_enabled = false, toast.autovacuum_enabled = false);": 2,
		"CREATE TABLE staging (id int) WITH (autovacuum_enabled = false);":                       1,
		"ALTER TABLE events SET (autovacuum_enabled = true);":                                    0,
		"ALTER TABLE events SET (autovacuum_vacuum_scale_factor = 0.01);":                        0,
		"ALTER TABLE events RESET (autovacuum_enabled);":                                         0,
	}
	for sql, want := range cases {
		if got := findings(t, "PS142", Options{}, sql); len(got) != want {
			t.Errorf("%s: expected %d findings, got %d", sql, want, len(got))
		}
	}
}
