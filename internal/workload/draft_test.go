package workload

import (
	"bytes"
	"strings"
	"testing"
)

func TestDraftFromStats(t *testing.T) {
	records := []StatRecord{
		{Query: "UPDATE orders SET status = $1 WHERE id = $2", Calls: 800},
		{Query: "SELECT id, status FROM orders WHERE id = $1", Calls: 5400},
		{Query: "SELECT id FROM orders WHERE customer_id = $1 FOR UPDATE", Calls: 50},
		{Query: "SELECT name FROM customers WHERE id = $1", Calls: 3000},
		{Query: "SELECT id FROM orders WHERE created_at > $1", Calls: 10},
		{Query: "BEGIN", Calls: 9000},
		{Query: "CREATE INDEX x ON orders (status)", Calls: 1},
		{Query: "SELECT 1", Calls: 0},
	}
	d := DraftFromStats(records, DraftOptions{Table: "orders", Top: 3, Name: "orders-api"})
	if len(d.Scenarios) != 3 {
		t.Fatalf("scenarios = %+v", d.Scenarios)
	}
	want := []DraftScenario{
		{ID: "select_orders", Mode: "read", Weight: 5400, SQL: "SELECT id, status FROM orders WHERE id = $1", Params: 1},
		{ID: "update_orders", Mode: "write", Weight: 800, SQL: "UPDATE orders SET status = $1 WHERE id = $2", Params: 2},
		{ID: "select_orders_2", Mode: "write", Weight: 50, SQL: "SELECT id FROM orders WHERE customer_id = $1 FOR UPDATE", Params: 1},
	}
	for i, w := range want {
		if d.Scenarios[i] != w {
			t.Errorf("scenario %d = %+v, want %+v", i, d.Scenarios[i], w)
		}
	}
	if all := DraftFromStats(records, DraftOptions{Table: "orders"}); len(all.Scenarios) != 4 || len(all.Skipped) != 1 || !strings.Contains(all.Skipped[0], "CREATE INDEX") {
		t.Errorf("without --top: %d scenarios, skipped %v", len(all.Scenarios), all.Skipped)
	}

	// The rendered draft must be a valid workload once loaded.
	var buf bytes.Buffer
	if err := d.Write(&buf); err != nil {
		t.Fatal(err)
	}
	spec, err := Parse(buf.Bytes())
	if err != nil {
		t.Fatalf("draft does not parse: %v\n%s", err, buf.String())
	}
	if spec.Name != "orders-api" || len(spec.Scenarios) != 3 || len(spec.Scenarios[1].Parameters) != 2 {
		t.Fatalf("parsed draft: %+v", spec)
	}
	if !strings.Contains(buf.String(), "# TODO: $2") {
		t.Errorf("draft lacks hints:\n%s", buf.String())
	}
}
