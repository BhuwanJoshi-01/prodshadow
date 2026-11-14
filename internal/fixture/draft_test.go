package fixture

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/BhuwanJoshi-01/prodshadow/internal/sqlinspect"
)

const draftSchema = `
CREATE TYPE order_status AS ENUM ('new', 'paid', 'shipped');
CREATE TABLE customers (
  id bigint PRIMARY KEY,
  email varchar(20) NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE orders (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  customer_id bigint NOT NULL REFERENCES customers (id),
  status order_status NOT NULL,
  total numeric(12, 2),
  paid boolean,
  reference uuid,
  payload jsonb NOT NULL,
  note jsonb
);
CREATE TABLE audit (id serial PRIMARY KEY, at timestamptz DEFAULT now());
`

func TestDraftGenerator(t *testing.T) {
	stmts, err := sqlinspect.ParseSources([]sqlinspect.Source{{Path: "schema.sql", Content: draftSchema}})
	if err != nil {
		t.Fatal(err)
	}
	d := DraftGenerator(stmts, DraftOptions{Rows: map[string]int64{"customers": 500}, DefaultRows: 2000})
	if len(d.File.Tables) != 2 {
		t.Fatalf("tables = %+v", d.File.Tables)
	}
	customers, orders := d.File.Tables[0], d.File.Tables[1]
	if customers.Name != "customers" || customers.Rows != 500 || len(customers.Columns) != 2 {
		t.Fatalf("customers = %+v", customers)
	}
	if c := customers.Columns[0]; c.Name != "id" || c.Generator != "sequence" {
		t.Errorf("customers.id = %+v", c)
	}
	if c := customers.Columns[1]; c.Generator != "text" || c.MaxLength != 20 {
		t.Errorf("customers.email = %+v", c)
	}
	want := map[string]string{"customer_id": "int_range", "status": "choice", "total": "float_range", "paid": "bool", "reference": "uuid"}
	if orders.Rows != 2000 || len(orders.Columns) != len(want) {
		t.Fatalf("orders = %+v", orders)
	}
	for _, c := range orders.Columns {
		if want[c.Name] != c.Generator {
			t.Errorf("orders.%s = %s, want %s", c.Name, c.Generator, want[c.Name])
		}
		if c.Name == "customer_id" && *c.Max != 500 {
			t.Errorf("foreign key range should follow customers' row count: %+v", c)
		}
		if c.Name == "status" && len(c.Values) != 3 {
			t.Errorf("enum values: %+v", c)
		}
	}
	if len(d.Notes) != 3 {
		t.Fatalf("notes = %v", d.Notes)
	}
	if err := d.File.Validate(1_000_000); err != nil {
		t.Fatalf("draft does not validate: %v", err)
	}
}

func TestDraftGeneratorWriteRoundTrips(t *testing.T) {
	stmts, err := sqlinspect.ParseSources([]sqlinspect.Source{{Path: "schema.sql", Content: draftSchema}})
	if err != nil {
		t.Fatal(err)
	}
	d := DraftGenerator(stmts, DraftOptions{})
	path := filepath.Join(t.TempDir(), "generator.yaml")
	var buf bytes.Buffer
	if err := d.Write(&buf); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	gf, err := LoadGeneratorFile(path, 1_000_000)
	if err != nil {
		t.Fatalf("draft does not load: %v\n%s", err, buf.String())
	}
	if !reflect.DeepEqual(gf.Tables, d.File.Tables) {
		t.Fatalf("round trip changed the draft:\n%+v\n%+v", gf.Tables, d.File.Tables)
	}
	if !strings.Contains(buf.String(), "# TODO: orders.payload (jsonb, NOT NULL)") {
		t.Errorf("notes missing:\n%s", buf.String())
	}
}
