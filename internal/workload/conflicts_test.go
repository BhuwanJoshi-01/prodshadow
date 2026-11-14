package workload

import (
	"testing"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/sqlinspect"
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
