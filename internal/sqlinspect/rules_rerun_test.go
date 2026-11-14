package sqlinspect

import "testing"

func TestPS155NotRerunnable(t *testing.T) {
	cases := []struct {
		sql  string
		opts Options
		want int
	}{
		{"CREATE INDEX CONCURRENTLY a ON t (a);\nCREATE INDEX CONCURRENTLY b ON t (b);\n", Options{}, 1},
		{"CREATE INDEX CONCURRENTLY IF NOT EXISTS a ON t (a);\nCREATE INDEX CONCURRENTLY IF NOT EXISTS b ON t (b);\n", Options{}, 0},
		{"CREATE INDEX CONCURRENTLY a ON t (a);\n", Options{}, 0},
		{"ALTER TABLE t ADD COLUMN c int;\nUPDATE t SET c = 1 WHERE id < 10;\n", Options{Transaction: TxnModeNone}, 1},
		{"ALTER TABLE t ADD COLUMN IF NOT EXISTS c int;\nUPDATE t SET c = 1 WHERE id < 10;\n", Options{Transaction: TxnModeNone}, 0},
		{"ALTER TABLE t ADD COLUMN c int;\nUPDATE t SET c = 1 WHERE id < 10;\n", Options{Transaction: TxnModeAuto}, 0},
	}
	for _, tc := range cases {
		if got := findings(t, "PS155", tc.opts, tc.sql); len(got) != tc.want {
			t.Errorf("%q (%s): expected %d findings, got %d", tc.sql, tc.opts.Transaction, tc.want, len(got))
		}
	}
}
