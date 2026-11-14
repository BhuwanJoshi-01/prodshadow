package sqlinspect

import (
	"testing"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/pglock"
)

func TestPredictConflicts(t *testing.T) {
	migration := parse(t, `
CREATE INDEX orders_status_idx ON public.orders (status);
ALTER TABLE orders ADD COLUMN note text;
CREATE TABLE audit (id int);
ALTER TABLE audit ADD COLUMN at timestamptz;
`)
	read, err := ParseOne("SELECT * FROM orders WHERE id = $1")
	if err != nil {
		t.Fatal(err)
	}
	got := PredictConflicts(migration, read)
	want := []Conflict{{StatementID: "1.2", Table: "orders", MigrationLock: pglock.AccessExclusive, WorkloadLock: pglock.AccessShare}}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("a read conflicts only with ACCESS EXCLUSIVE: %+v", got)
	}
	write, _ := ParseOne("UPDATE orders SET status = $1 WHERE id = $2")
	if got := PredictConflicts(migration, write); len(got) != 2 || got[0].StatementID != "1.1" || got[0].MigrationLock != pglock.Share {
		t.Fatalf("a write conflicts with the index build and the ALTER: %+v", got)
	}
	other, _ := ParseOne("SELECT * FROM audit")
	if got := PredictConflicts(migration, other); len(got) != 0 {
		t.Fatalf("locks on a table the migration created conflict with nothing: %+v", got)
	}
}
