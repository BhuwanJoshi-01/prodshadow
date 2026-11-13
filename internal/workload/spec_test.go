package workload

import (
	"strings"
	"testing"
)

const blueprintWorkload = `version: 1
name: order-api
scenarios:
  - id: recent_orders
    weight: 60
    sql: "SELECT id, status FROM orders WHERE customer_id = $1 ORDER BY created_at DESC LIMIT 20"
    parameters:
      - generator: int_range
        min: 1
        max: 100000
    mode: read
  - id: update_order_status
    weight: 30
    sql: "UPDATE orders SET status = $1 WHERE id = $2"
    parameters:
      - generator: choice
        values: [processing, shipped]
      - generator: int_range
        min: 1
        max: 1000000
    mode: write
  - id: held_transaction
    weight: 10
    mode: write
    transaction:
      statements:
        - "SELECT id FROM orders WHERE id = $1 FOR UPDATE"
      hold_ms: 1000
    parameters:
      - generator: int_range
        min: 1
        max: 1000000
background:
  - id: nightly_report
    sessions: 1
    start_after_seconds: 2
    repeat: true
    transaction:
      statements: ["SELECT count(*) FROM orders"]
      hold_ms: 8000
`

func TestParseBlueprintWorkload(t *testing.T) {
	s, err := Parse([]byte(blueprintWorkload))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Scenarios) != 3 || s.Scenarios[2].Transaction.HoldMS != 1000 || len(s.Background) != 1 {
		t.Fatalf("unexpected spec %+v", s)
	}
}

func TestWorkloadValidationErrors(t *testing.T) {
	cases := map[string]string{
		"unknown key":     strings.Replace(blueprintWorkload, "    mode: read\n", "    mode: read\n    moed: x\n", 1),
		"missing param":   strings.Replace(blueprintWorkload, "WHERE id = $2", "WHERE id = $3", 1),
		"ddl":             strings.Replace(blueprintWorkload, `"SELECT count(*) FROM orders"`, `"DROP TABLE orders"`, 1),
		"duplicate id":    strings.Replace(blueprintWorkload, "id: held_transaction", "id: recent_orders", 1),
		"bad weight":      strings.Replace(blueprintWorkload, "weight: 60", "weight: 0", 1),
		"bad mode":        strings.Replace(blueprintWorkload, "mode: read", "mode: both", 1),
		"bad generator":   strings.Replace(blueprintWorkload, "generator: choice", "generator: pick", 1),
		"sessions":        strings.Replace(blueprintWorkload, "sessions: 1", "sessions: 0", 1),
		"syntax":          strings.Replace(blueprintWorkload, "SELECT id, status", "SELEC id, status", 1),
		"multi statement": strings.Replace(blueprintWorkload, `"UPDATE orders SET status = $1 WHERE id = $2"`, `"UPDATE orders SET status = $1 WHERE id = $2; DELETE FROM orders"`, 1),
		"hold":            strings.Replace(blueprintWorkload, "hold_ms: 1000", "hold_ms: -1", 1),
		"version":         strings.Replace(blueprintWorkload, "version: 1", "version: 3", 1),
	}
	for name, doc := range cases {
		if _, err := Parse([]byte(doc)); err == nil {
			t.Errorf("%s: expected a validation error", name)
		}
	}
}

func TestValidationReportsAllErrors(t *testing.T) {
	doc := "version: 1\nscenarios:\n  - id: Bad\n    weight: 0\n    mode: x\n"
	_, err := Parse([]byte(doc))
	if err == nil {
		t.Fatal("expected errors")
	}
	for _, want := range []string{"id \"Bad\"", "weight", "mode", "sql or transaction"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q: %v", want, err)
		}
	}
}
