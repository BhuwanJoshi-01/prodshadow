package sqlinspect

import "testing"

func TestPS130SequenceReset(t *testing.T) {
	cases := []struct {
		sql  string
		want int
	}{
		{"ALTER SEQUENCE orders_id_seq RESTART WITH 1;", 1},
		{"ALTER SEQUENCE orders_id_seq RESTART;", 1},
		{"ALTER SEQUENCE orders_id_seq INCREMENT BY 10;", 0},
		{"SELECT setval('orders_id_seq', 1000);", 1},
		{"SELECT setval('orders_id_seq', 1, false);", 1},
		{"SELECT setval('orders_id_seq', 1), setval('items_id_seq', 1);", 1},
		{"SELECT setval('orders_id_seq', (SELECT max(id) FROM orders));", 0},
		{"SELECT setval('orders_id_seq', coalesce(max(id), 1)) FROM orders;", 0},
		{"TRUNCATE orders RESTART IDENTITY;", 1},
		{"TRUNCATE orders;", 0},
		{"CREATE SEQUENCE invoice_no;\nALTER SEQUENCE invoice_no RESTART WITH 1000;", 0},
	}
	for _, tc := range cases {
		if got := findings(t, "PS130", Options{}, tc.sql); len(got) != tc.want {
			t.Errorf("%q: expected %d findings, got %d: %+v", tc.sql, tc.want, len(got), got)
		}
	}
}
