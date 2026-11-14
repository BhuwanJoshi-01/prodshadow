package sqlinspect

import "testing"

func TestPS123AttachPartition(t *testing.T) {
	attach := "ALTER TABLE events ATTACH PARTITION events_2026 FOR VALUES FROM ('2026-01-01') TO ('2027-01-01');"
	expectCount(t, findings(t, "PS123", Options{}, attach), 1)
	staged := "ALTER TABLE events_2026 ADD CONSTRAINT bounds CHECK (at >= '2026-01-01' AND at < '2027-01-01') NOT VALID;\n" + attach
	expectCount(t, findings(t, "PS123", Options{}, staged), 0)
	fresh := "CREATE TABLE events_2026 (LIKE events);\n" + attach
	expectCount(t, findings(t, "PS123", Options{}, fresh), 0)
}

func TestPS140DetachPartition(t *testing.T) {
	expectCount(t, findings(t, "PS140", Options{}, "ALTER TABLE events DETACH PARTITION events_2024;"), 1)
	expectCount(t, findings(t, "PS140", Options{}, "ALTER TABLE events DETACH PARTITION events_2024 CONCURRENTLY;"), 0)
	// FINALIZE completes an interrupted concurrent detach.
	expectCount(t, findings(t, "PS140", Options{}, "ALTER TABLE events DETACH PARTITION events_2024 FINALIZE;"), 0)
	expectCount(t, findings(t, "PS140", Options{}, "CREATE TABLE events (at date) PARTITION BY RANGE (at);\nALTER TABLE events DETACH PARTITION events_2024;"), 0)
}

func TestPS141CreatePartition(t *testing.T) {
	got := findings(t, "PS141", Options{}, "CREATE TABLE events_2027 PARTITION OF events FOR VALUES FROM ('2027-01-01') TO ('2028-01-01');")
	expectCount(t, got, 1)
	if got[0].Table != "events" {
		t.Errorf("the lock is on the parent: %+v", got[0])
	}
	expectCount(t, findings(t, "PS141", Options{}, "CREATE TABLE events_2027 (LIKE events INCLUDING ALL);"), 0)
	expectCount(t, findings(t, "PS141", Options{}, "CREATE TABLE events (at date) PARTITION BY RANGE (at);\nCREATE TABLE events_2027 PARTITION OF events FOR VALUES FROM ('2027-01-01') TO ('2028-01-01');"), 0)
}
