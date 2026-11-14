package sqlinspect

import (
	"strings"
	"testing"
)

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
		"DROP EXTENSION pg_trgm;",
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
	for sql, want := range map[string]string{
		"ALTER VIEW order_totals RENAME TO order_sums;":               "view order_totals to order_sums",
		"ALTER MATERIALIZED VIEW daily_sales RENAME TO sales_by_day;": "materialized view daily_sales",
		"ALTER FUNCTION order_total(bigint) RENAME TO order_sum;":     "function order_total to order_sum",
		"ALTER TYPE mood RENAME TO feeling;":                          "type mood to feeling",
		"ALTER SCHEMA reporting RENAME TO analytics;":                 "schema reporting to analytics",
		"ALTER TYPE order_status RENAME VALUE 'paid' TO 'settled';":   "value 'paid' of enum order_status",
	} {
		got := findings(t, "PS110", Options{}, sql)
		expectCount(t, got, 1)
		if !strings.Contains(got[0].Message, want) {
			t.Errorf("%s: message %q lacks %q", sql, got[0].Message, want)
		}
	}
	expectCount(t, findings(t, "PS110", Options{}, "ALTER TYPE order_status ADD VALUE 'refunded';"), 0)
	expectCount(t, findings(t, "PS110", Options{}, "ALTER SEQUENCE invoice_no RENAME TO invoice_number;"), 0)
}
