package workload

import (
	"testing"

	"github.com/BhuwanJoshi-01/prodshadow/internal/sqlinspect"
)

func TestPredictConflicts(t *testing.T) {
	spec := &Spec{
		Scenarios: []Scenario{
			{ID: "read", SQL: "SELECT * FROM orders WHERE id = $1"},
			{ID: "invoice", SQL: "INSERT INTO invoices (id, customer_id) VALUES ($1, $2)"},
		},
		Background: []Background{{ID: "report", Transaction: TxnSpec{Statements: []string{"SELECT count(*) FROM orders"}}}},
	}
	migration, err := sqlinspect.ParseSources([]sqlinspect.Source{{Path: "m.sql", Content: "ALTER TABLE customers ADD COLUMN tier text;\n"}})
	if err != nil {
		t.Fatal(err)
	}
	if got, unblocked := PredictConflicts(spec, migration, nil); len(got) != 0 || len(unblocked) != 3 {
		t.Fatalf("without the schema nothing touches customers: %+v %v", got, unblocked)
	}
	schema, _ := sqlinspect.ParseSources([]sqlinspect.Source{{Path: "s.sql", Content: "CREATE TABLE customers (id int PRIMARY KEY);\nCREATE TABLE invoices (id int, customer_id int REFERENCES customers);\n"}})
	got, unblocked := PredictConflicts(spec, migration, sqlinspect.NewCatalog(schema))
	if len(got) != 1 || got[0].Session != "invoice" || got[0].Table != "customers" || got[0].WorkloadLock != "ROW SHARE" || got[0].MigrationLock != "ACCESS EXCLUSIVE" {
		t.Fatalf("the insert checks customers through its foreign key: %+v", got)
	}
	if len(unblocked) != 2 || unblocked[1] != "report" {
		t.Fatalf("unblocked = %v", unblocked)
	}
}

func TestPredictConflictsThroughTheSchema(t *testing.T) {
	schema, err := sqlinspect.ParseSources([]sqlinspect.Source{{Path: "s.sql", Content: `
CREATE TABLE orders (id int, status text);
CREATE VIEW open_orders AS SELECT id, status FROM orders WHERE status <> 'closed';
CREATE TABLE audit (id int);
CREATE TABLE events (id int, at date) PARTITION BY RANGE (at);
CREATE TABLE events_2026 PARTITION OF events FOR VALUES FROM ('2026-01-01') TO ('2027-01-01');
CREATE FUNCTION audit_events() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN INSERT INTO audit VALUES (NEW.id); RETURN NEW; END $$;
CREATE TRIGGER events_audit AFTER INSERT ON events FOR EACH ROW EXECUTE FUNCTION audit_events();
`}})
	if err != nil {
		t.Fatal(err)
	}
	spec := &Spec{Scenarios: []Scenario{
		{ID: "dashboard", SQL: "SELECT * FROM open_orders"},
		{ID: "track", SQL: "INSERT INTO events VALUES ($1, '2026-05-01')"},
	}}
	migration, err := sqlinspect.ParseSources([]sqlinspect.Source{{Path: "m.sql", Content: "ALTER TABLE orders ADD COLUMN note text;\nALTER TABLE events_2026 ADD CONSTRAINT positive CHECK (id > 0);\nTRUNCATE audit;\n"}})
	if err != nil {
		t.Fatal(err)
	}
	catalog := sqlinspect.NewCatalog(schema)
	catalog.Resolve(migration)
	got, _ := PredictConflicts(spec, migration, sqlinspect.NewCatalog(schema))
	want := map[string]bool{"dashboard/orders/1.1": true, "track/events_2026/1.2": true, "track/audit/1.3": true}
	for _, c := range got {
		key := c.Session + "/" + c.Table + "/" + c.StatementID
		if !want[key] {
			t.Errorf("unexpected conflict %s", key)
		}
		delete(want, key)
	}
	for k := range want {
		t.Errorf("missing conflict %s (through a view, a partition or a trigger)", k)
	}
}
