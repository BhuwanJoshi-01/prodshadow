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
