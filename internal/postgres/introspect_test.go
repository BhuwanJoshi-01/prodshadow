package postgres

import "testing"

func TestTotalRowsSkipsPartitionedParents(t *testing.T) {
	tables := []TableSize{
		{Name: "public.events", RowsEstimate: 200000, Partitioned: true},
		{Name: "public.events_2025", RowsEstimate: 120000},
		{Name: "public.events_2026", RowsEstimate: 80000},
		{Name: "public.accounts", RowsEstimate: 2000},
	}
	if got := TotalRows(tables); got != 202000 {
		t.Fatalf("TotalRows = %d, want 202000", got)
	}
	if got := TotalRows(nil); got != 0 {
		t.Fatalf("TotalRows(nil) = %d, want 0", got)
	}
}
