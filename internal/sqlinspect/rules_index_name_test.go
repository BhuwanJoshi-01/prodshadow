package sqlinspect

import "testing"

func TestPS132UnnamedConcurrentIndex(t *testing.T) {
	expectCount(t, findings(t, "PS132", Options{}, "CREATE INDEX CONCURRENTLY ON orders (customer_id);"), 1)
	expectCount(t, findings(t, "PS132", Options{}, "CREATE UNIQUE INDEX CONCURRENTLY ON orders (reference);"), 1)
	expectCount(t, findings(t, "PS132", Options{}, "CREATE INDEX CONCURRENTLY orders_customer_idx ON orders (customer_id);"), 0)
	expectCount(t, findings(t, "PS132", Options{}, "CREATE INDEX ON orders (customer_id);"), 0)
	expectCount(t, findings(t, "PS132", Options{}, "CREATE TABLE notes (id int);\nCREATE INDEX CONCURRENTLY ON notes (id);"), 0)
}
