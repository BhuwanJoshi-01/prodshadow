package sqlinspect

import "testing"

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
