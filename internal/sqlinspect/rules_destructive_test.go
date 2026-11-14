package sqlinspect

import "testing"

func TestPS109Destructive(t *testing.T) {
	expectCount(t, findings(t, "PS109", Options{}, "DROP TABLE legacy_orders;"), 1)
	expectCount(t, findings(t, "PS109", Options{}, "TRUNCATE a, b;"), 2)
	expectCount(t, findings(t, "PS109", Options{}, "ALTER TABLE orders DROP COLUMN note, DROP COLUMN memo;"), 2)
	expectCount(t, findings(t, "PS109", Options{}, "CREATE TABLE tmp (id int); DROP TABLE tmp;"), 0)
	for _, sql := range []string{
		"DROP VIEW order_totals;",
		"DROP MATERIALIZED VIEW daily_sales;",
		"DROP SEQUENCE invoice_no;",
		"DROP FUNCTION order_total(bigint);",
		"DROP TYPE mood;",
		"DROP SCHEMA reporting;",
	} {
		expectCount(t, findings(t, "PS109", Options{}, sql), 1)
	}
	expectCount(t, findings(t, "PS109", Options{}, "DROP VIEW a, b;"), 2)
	expectCount(t, findings(t, "PS109", Options{}, "DROP INDEX orders_note_idx;"), 0)
	expectCount(t, findings(t, "PS109", Options{}, "CREATE SEQUENCE s; DROP SEQUENCE s;"), 0)
}

func TestPS110Rename(t *testing.T) {
	expectCount(t, findings(t, "PS110", Options{}, "ALTER TABLE orders RENAME COLUMN note TO memo;"), 1)
	expectCount(t, findings(t, "PS110", Options{}, "ALTER TABLE orders RENAME TO purchases;"), 1)
	expectCount(t, findings(t, "PS110", Options{}, "ALTER INDEX idx RENAME TO idx2;"), 0)
}
