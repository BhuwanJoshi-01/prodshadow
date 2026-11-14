package sqlinspect

import (
	"strings"
	"testing"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/pglock"
)

func TestPS111UnboundedDML(t *testing.T) {
	expectCount(t, findings(t, "PS111", Options{}, "UPDATE orders SET status = 'new';"), 1)
	expectCount(t, findings(t, "PS111", Options{}, "DELETE FROM orders;"), 1)
	expectCount(t, findings(t, "PS111", Options{}, "UPDATE orders SET status = 'new' WHERE id < 1000;"), 0)
}

func TestPS112ExplicitLock(t *testing.T) {
	expectCount(t, findings(t, "PS112", Options{}, "LOCK TABLE orders IN ACCESS EXCLUSIVE MODE;"), 1)
	expectCount(t, findings(t, "PS112", Options{}, "LOCK TABLE orders IN ACCESS SHARE MODE;"), 0)
}

func TestPS113RewriteMaintenance(t *testing.T) {
	for _, sql := range []string{"VACUUM FULL orders;", "CLUSTER orders USING idx;", "REFRESH MATERIALIZED VIEW mv;", "REINDEX TABLE orders;", "ALTER TABLE orders SET LOGGED;"} {
		expectCount(t, findings(t, "PS113", Options{}, sql), 1)
	}
	for _, sql := range []string{"VACUUM orders;", "REINDEX TABLE CONCURRENTLY orders;", "REFRESH MATERIALIZED VIEW CONCURRENTLY mv;", "REINDEX SCHEMA CONCURRENTLY public;"} {
		expectCount(t, findings(t, "PS113", Options{}, sql), 0)
	}
	wide := findings(t, "PS113", Options{}, "REINDEX SCHEMA public;")
	expectCount(t, wide, 1)
	if wide[0].Lock != pglock.Share || !strings.Contains(wide[0].Message, "one table at a time") {
		t.Errorf("schema-wide REINDEX finding: %+v", wide[0])
	}
}

func TestPS114MissingLockTimeout(t *testing.T) {
	got := findings(t, "PS114", Options{}, "ALTER TABLE orders ADD COLUMN c int; ALTER TABLE orders ADD COLUMN d int;")
	expectCount(t, got, 1)
	if got[0].StatementID != "1.1" {
		t.Fatalf("finding should point at the first blocking statement: %+v", got[0])
	}
	expectCount(t, findings(t, "PS114", Options{}, "SET lock_timeout = '2s'; ALTER TABLE orders ADD COLUMN c int;"), 0)
	expectCount(t, findings(t, "PS114", Options{}, "CREATE INDEX CONCURRENTLY i ON orders (a);"), 0)
}

func TestPS115LockHeldAcrossStatements(t *testing.T) {
	got := findings(t, "PS115", Options{}, "ALTER TABLE orders ADD COLUMN c int;\nUPDATE orders SET c = 1 WHERE id < 10;")
	expectCount(t, got, 1)
	if !strings.Contains(got[0].Message, "UPDATE at a.sql:2") {
		t.Fatalf("message should reference the later statement: %s", got[0].Message)
	}
	expectCount(t, findings(t, "PS115", Options{Transaction: TxnModeNone}, "ALTER TABLE orders ADD COLUMN c int; UPDATE orders SET c = 1 WHERE id < 10;"), 0)
	expectCount(t, findings(t, "PS115", Options{}, "UPDATE orders SET c = 1 WHERE id = 1; ALTER TABLE orders ADD COLUMN c int;"), 0)
}

func TestPS118TriggerChange(t *testing.T) {
	expectCount(t, findings(t, "PS118", Options{}, "CREATE TRIGGER t BEFORE UPDATE ON orders FOR EACH ROW EXECUTE FUNCTION f();"), 1)
	expectCount(t, findings(t, "PS118", Options{}, "ALTER TABLE orders DISABLE TRIGGER t;"), 1)
}
