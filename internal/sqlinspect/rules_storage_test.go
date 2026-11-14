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

func TestPS143UnloggedTable(t *testing.T) {
	cases := map[string]int{
		"CREATE UNLOGGED TABLE import_rows (id int, payload jsonb);": 1,
		"CREATE UNLOGGED TABLE totals AS SELECT 1 AS n;":             1,
		"ALTER TABLE events SET UNLOGGED;":                           1,
		"ALTER TABLE events SET LOGGED;":                             0,
		"CREATE TABLE events_copy (id int);":                         0,
		"CREATE TEMPORARY TABLE scratch (id int);":                   0,
	}
	for sql, want := range cases {
		if got := findings(t, "PS143", Options{}, sql); len(got) != want {
			t.Errorf("%s: expected %d findings, got %d", sql, want, len(got))
		}
	}
}
