package sqlinspect

import "testing"

func TestPS109Destructive(t *testing.T) {
	expectCount(t, findings(t, "PS109", Options{}, "DROP TABLE legacy_orders;"), 1)
	expectCount(t, findings(t, "PS109", Options{}, "TRUNCATE a, b;"), 2)
	expectCount(t, findings(t, "PS109", Options{}, "ALTER TABLE orders DROP COLUMN note, DROP COLUMN memo;"), 2)
	expectCount(t, findings(t, "PS109", Options{}, "CREATE TABLE tmp (id int); DROP TABLE tmp;"), 0)
}

func TestPS110Rename(t *testing.T) {
	expectCount(t, findings(t, "PS110", Options{}, "ALTER TABLE orders RENAME COLUMN note TO memo;"), 1)
	expectCount(t, findings(t, "PS110", Options{}, "ALTER TABLE orders RENAME TO purchases;"), 1)
	expectCount(t, findings(t, "PS110", Options{}, "ALTER INDEX idx RENAME TO idx2;"), 0)
}
