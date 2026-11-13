package sqlinspect

import (
	"testing"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/pglock"
)

func TestPS107ValidatedConstraints(t *testing.T) {
	expectCount(t, findings(t, "PS107", Options{}, "ALTER TABLE orders ADD CONSTRAINT fk FOREIGN KEY (customer_id) REFERENCES customers (id);"), 1)
	expectCount(t, findings(t, "PS107", Options{}, "ALTER TABLE orders ADD CONSTRAINT fk FOREIGN KEY (customer_id) REFERENCES customers (id) NOT VALID;"), 0)
	expectCount(t, findings(t, "PS107", Options{}, "ALTER TABLE orders ADD CONSTRAINT positive CHECK (amount > 0);"), 1)
	expectCount(t, findings(t, "PS107", Options{}, "ALTER TABLE orders ADD CONSTRAINT positive CHECK (amount > 0) NOT VALID;"), 0)
}

func TestPS108ConstraintIndexBuild(t *testing.T) {
	expectCount(t, findings(t, "PS108", Options{}, "ALTER TABLE orders ADD CONSTRAINT u UNIQUE (ref);"), 1)
	expectCount(t, findings(t, "PS108", Options{}, "ALTER TABLE orders ADD PRIMARY KEY (id);"), 1)
	expectCount(t, findings(t, "PS108", Options{}, "ALTER TABLE orders ADD CONSTRAINT u UNIQUE USING INDEX orders_ref_idx;"), 0)
}

func TestPS119CreateTableForeignKey(t *testing.T) {
	expectCount(t, findings(t, "PS119", Options{}, "CREATE TABLE refunds (id int, order_id int REFERENCES orders (id));"), 1)
	expectCount(t, findings(t, "PS119", Options{}, "CREATE TABLE a (id int PRIMARY KEY); CREATE TABLE b (a_id int REFERENCES a (id));"), 0)
}

func TestInlineForeignKeyOnAddColumn(t *testing.T) {
	st := one(t, "ALTER TABLE orders ADD COLUMN coupon_id int REFERENCES coupons (id)")
	if st.LockOn("coupons") != pglock.ShareRowExclusive || st.LockOn("orders") != pglock.AccessExclusive {
		t.Fatalf("locks = %v", st.Locks)
	}
	expectCount(t, findings(t, "PS107", Options{}, "ALTER TABLE orders ADD COLUMN coupon_id int REFERENCES coupons (id);"), 1)
	expectCount(t, findings(t, "PS107", Options{}, "ALTER TABLE orders ADD COLUMN coupon_id int;"), 0)
}
