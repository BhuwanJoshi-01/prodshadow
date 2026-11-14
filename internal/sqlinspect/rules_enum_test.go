package sqlinspect

import "testing"

func TestPS128EnumValueUsedBeforeCommit(t *testing.T) {
	add := "ALTER TYPE order_status ADD VALUE 'refunded';\n"
	use := "UPDATE orders SET status = 'refunded' WHERE refunded_at IS NOT NULL AND id < 1000;\n"
	got := findings(t, "PS128", Options{Transaction: TxnModeAuto}, add+use)
	expectCount(t, got, 1)
	if got[0].Line != 2 {
		t.Errorf("finding should point at the statement using the value, got line %d", got[0].Line)
	}
	// Separate transactions: one file each with per-file transactions.
	expectCount(t, findings(t, "PS128", Options{Transaction: TxnModePerFile}, add, use), 0)
	// No transaction at all.
	expectCount(t, findings(t, "PS128", Options{Transaction: TxnModeNone}, add+use), 0)
	// Value not used.
	expectCount(t, findings(t, "PS128", Options{Transaction: TxnModeAuto}, add+"UPDATE orders SET note = 'x' WHERE id = 1;\n"), 0)
	// The type was created in the same transaction.
	created := "CREATE TYPE order_status AS ENUM ('new', 'paid');\n"
	expectCount(t, findings(t, "PS128", Options{Transaction: TxnModeAuto}, created+add+use), 0)
	// RENAME VALUE is not an addition.
	expectCount(t, findings(t, "PS128", Options{Transaction: TxnModeAuto}, "ALTER TYPE order_status RENAME VALUE 'paid' TO 'refunded';\n"+use), 0)
}
