package fixture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const genFile = `version: 1
tables:
  - name: customers
    rows: 100
    columns:
      - name: id
        generator: sequence
      - name: email
        generator: pattern
        format: "user{n}@example.test"
  - name: orders
    rows: 1000
    columns:
      - name: id
        generator: sequence
      - name: customer_id
        generator: zipf
        min: 1
        max: 100
        s: 1.1
      - name: status
        generator: choice
        values: [pending, shipped]
        weights: [3, 1]
`

func write(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "generate.yaml")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadGeneratorFile(t *testing.T) {
	gf, err := LoadGeneratorFile(write(t, genFile), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(gf.Tables) != 2 || gf.TotalRows() != 1100 || gf.Tables[1].Columns[1].Generator != "zipf" {
		t.Fatalf("unexpected file %+v", gf)
	}
}

func TestLoadGeneratorFileErrors(t *testing.T) {
	cases := map[string]string{
		"max_rows":    genFile,
		"unknown key": strings.Replace(genFile, "rows: 100\n", "rows: 100\n    rowz: 1\n", 1),
		"bad table":   strings.Replace(genFile, "name: customers", "name: Customers", 1),
		"bad gen":     strings.Replace(genFile, "generator: zipf", "generator: zipff", 1),
		"version":     strings.Replace(genFile, "version: 1", "version: 2", 1),
		"dup column":  strings.Replace(genFile, "name: email", "name: id", 1),
		"zero rows":   strings.Replace(genFile, "rows: 100\n", "rows: 0\n", 1),
	}
	for name, content := range cases {
		max := int64(0)
		if name == "max_rows" {
			max = 500
		}
		if _, err := LoadGeneratorFile(write(t, content), max); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
