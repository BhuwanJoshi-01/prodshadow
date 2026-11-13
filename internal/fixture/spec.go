// Package fixture builds the disposable database contents: it applies the
// schema and seed SQL files and loads deterministic synthetic rows.
package fixture

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"regexp"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/gen"

	"go.yaml.in/yaml/v3"
)

// GeneratorFile is the format of fixture.generator.path.
//
//	version: 1
//	tables:
//	  - name: orders
//	    rows: 100000
//	    columns:
//	      - name: id
//	        generator: sequence
//	      - name: customer_id
//	        generator: zipf
//	        min: 1
//	        max: 10000
//	        s: 1.1
type GeneratorFile struct {
	Version int         `yaml:"version" json:"version"`
	Tables  []TableSpec `yaml:"tables" json:"tables"`
}

// TableSpec generates rows for one existing table.
type TableSpec struct {
	Name    string       `yaml:"name" json:"name"`
	Rows    int64        `yaml:"rows" json:"rows"`
	Columns []ColumnSpec `yaml:"columns" json:"columns"`
}

// ColumnSpec binds a generator to a column.
type ColumnSpec struct {
	Name     string `yaml:"name" json:"name"`
	gen.Spec `yaml:",inline"`
}

var (
	tableRe  = regexp.MustCompile(`^([a-z_][a-z0-9_]{0,62}\.)?[a-z_][a-z0-9_]{0,62}$`)
	columnRe = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)
)

// LoadGeneratorFile reads and validates a generator file. maxRows caps the
// total number of rows across all tables.
func LoadGeneratorFile(path string, maxRows int64) (*GeneratorFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var gf GeneratorFile
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&gf); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := gf.Validate(maxRows); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &gf, nil
}

// Validate checks names, row counts and generator specs.
func (gf *GeneratorFile) Validate(maxRows int64) error {
	if gf.Version != 1 {
		return fmt.Errorf("unsupported generator file version %d", gf.Version)
	}
	if len(gf.Tables) == 0 {
		return errors.New("no tables defined")
	}
	var total int64
	seen := map[string]bool{}
	for i, t := range gf.Tables {
		where := fmt.Sprintf("tables[%d]", i)
		if !tableRe.MatchString(t.Name) {
			return fmt.Errorf("%s: invalid table name %q", where, t.Name)
		}
		if t.Rows <= 0 {
			return fmt.Errorf("%s (%s): rows must be positive", where, t.Name)
		}
		total += t.Rows
		if maxRows > 0 && total > maxRows {
			return fmt.Errorf("total generated rows exceed fixture.max_rows (%d)", maxRows)
		}
		if len(t.Columns) == 0 {
			return fmt.Errorf("%s (%s): no columns defined", where, t.Name)
		}
		cols := map[string]bool{}
		for j, c := range t.Columns {
			cw := fmt.Sprintf("%s.columns[%d]", where, j)
			if !columnRe.MatchString(c.Name) {
				return fmt.Errorf("%s: invalid column name %q", cw, c.Name)
			}
			if cols[c.Name] {
				return fmt.Errorf("%s: duplicate column %q", cw, c.Name)
			}
			cols[c.Name] = true
			if err := c.Validate(); err != nil {
				return fmt.Errorf("%s (%s.%s): %w", cw, t.Name, c.Name, err)
			}
		}
		if seen[t.Name] {
			return fmt.Errorf("%s: table %q listed twice", where, t.Name)
		}
		seen[t.Name] = true
	}
	return nil
}

// TotalRows returns the number of rows the file generates.
func (gf *GeneratorFile) TotalRows() int64 {
	var n int64
	for _, t := range gf.Tables {
		n += t.Rows
	}
	return n
}
