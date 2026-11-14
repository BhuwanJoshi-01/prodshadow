package sqlinspect

import (
	"testing"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/pglock"
)

func TestCreatedBy(t *testing.T) {
	stmts := parse(t, `
ALTER TABLE events_2027 ADD COLUMN early int;
CREATE TABLE public.events_2027 (LIKE events);
ALTER TABLE events_2027 ADD CONSTRAINT b CHECK (at >= '2027-01-01');
ALTER TABLE events ATTACH PARTITION events_2027 FOR VALUES FROM ('2027-01-01') TO ('2028-01-01');
`)
	c := NewCreated(stmts)
	if by := c.By(stmts[0], "events_2027"); by != nil {
		t.Fatalf("a statement before the CREATE must not see the table as new, got %s", by.ID)
	}
	if by := c.By(stmts[1], "public.events_2027"); by != nil {
		t.Fatal("the creating statement itself is not 'created earlier'")
	}
	if by := c.By(stmts[2], "events_2027"); by == nil || by.ID != "1.2" {
		t.Fatalf("unqualified name should match the public-qualified CREATE, got %v", by)
	}
	if by := c.By(stmts[3], "events"); by != nil {
		t.Fatal("events existed before the migration")
	}
	if got := c.ExistingLock(stmts[3]); got.Table != "events" || got.Mode != pglock.ShareUpdateExclusive {
		t.Fatalf("ExistingLock = %+v, want SHARE UPDATE EXCLUSIVE on events", got)
	}
	if got := c.ExistingLock(stmts[2]); got.Mode != pglock.None {
		t.Fatalf("locks only on new relations should not count, got %+v", got)
	}
}
