package fixture

import (
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/gen"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/sqlinspect"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

// DraftOptions control a drafted generator file.
type DraftOptions struct {
	// Rows maps table names to row counts; other tables get DefaultRows.
	Rows        map[string]int64
	DefaultRows int64
}

// Draft is a generator file derived from a schema, plus notes about
// columns the draft could not fill.
type Draft struct {
	File  GeneratorFile
	Notes []string
}

type draftColumn struct {
	name, typ string
	maxLen    int
	pk, skip  bool
	notNull   bool
	ref       string
}

// DraftGenerator proposes generators for the tables a schema creates:
// sequences for integer primary keys, ranges for foreign keys that point at
// another generated table, values of enum types, and type-based generators
// for the rest. Columns with defaults, identity and serial columns are left
// to their defaults.
func DraftGenerator(stmts []*sqlinspect.Statement, opts DraftOptions) Draft {
	if opts.DefaultRows <= 0 {
		opts.DefaultRows = 10_000
	}
	enums := map[string][]any{}
	type table struct {
		name string
		cols []*draftColumn
	}
	var tables []*table
	for _, s := range stmts {
		n := s.Node()
		if n == nil {
			continue
		}
		if e := n.GetCreateEnumStmt(); e != nil {
			var vals []any
			for _, v := range e.Vals {
				if sv := v.GetString_(); sv != nil {
					vals = append(vals, sv.Sval)
				}
			}
			enums[lastPart(e.TypeName)] = vals
			continue
		}
		ct := n.GetCreateStmt()
		if ct == nil || ct.Partbound != nil || ct.GetRelation().GetRelpersistence() == "t" {
			continue
		}
		t := &table{name: tableName(ct.Relation)}
		byName := map[string]*draftColumn{}
		for _, e := range ct.TableElts {
			cd := e.GetColumnDef()
			if cd == nil {
				continue
			}
			c := &draftColumn{name: cd.Colname, typ: lastPart(cd.GetTypeName().GetNames())}
			if mods := cd.GetTypeName().GetTypmods(); len(mods) == 1 {
				if ic := mods[0].GetAConst().GetIval(); ic != nil {
					c.maxLen = int(ic.Ival)
				}
			}
			for _, cn := range cd.Constraints {
				con := cn.GetConstraint()
				if con == nil {
					continue
				}
				switch con.Contype {
				case pg_query.ConstrType_CONSTR_DEFAULT, pg_query.ConstrType_CONSTR_IDENTITY, pg_query.ConstrType_CONSTR_GENERATED:
					c.skip = true
				case pg_query.ConstrType_CONSTR_PRIMARY:
					c.pk, c.notNull = true, true
				case pg_query.ConstrType_CONSTR_NOTNULL:
					c.notNull = true
				case pg_query.ConstrType_CONSTR_FOREIGN:
					c.ref = tableName(con.Pktable)
				}
			}
			if strings.Contains(c.typ, "serial") {
				c.skip = true
			}
			t.cols = append(t.cols, c)
			byName[c.name] = c
		}
		for _, e := range append(append([]*pg_query.Node(nil), ct.TableElts...), ct.Constraints...) {
			con := e.GetConstraint()
			if con == nil {
				continue
			}
			keys := names(con.Keys)
			switch {
			case con.Contype == pg_query.ConstrType_CONSTR_PRIMARY && len(keys) == 1 && byName[keys[0]] != nil:
				byName[keys[0]].pk, byName[keys[0]].notNull = true, true
			case con.Contype == pg_query.ConstrType_CONSTR_FOREIGN:
				if fk := names(con.FkAttrs); len(fk) == 1 && byName[fk[0]] != nil {
					byName[fk[0]].ref = tableName(con.Pktable)
				}
			}
		}
		tables = append(tables, t)
	}

	rowsOf := func(name string) int64 {
		if n, ok := opts.Rows[name]; ok && n > 0 {
			return n
		}
		return opts.DefaultRows
	}
	generated := map[string]bool{}
	for _, t := range tables {
		generated[t.name] = true
	}
	var d Draft
	d.File.Version = 1
	for _, t := range tables {
		ts := TableSpec{Name: t.name, Rows: rowsOf(t.name)}
		for _, c := range t.cols {
			if c.skip {
				continue
			}
			spec, ok := columnGenerator(c, enums, generated, rowsOf)
			if !ok {
				if c.notNull {
					d.Notes = append(d.Notes, fmt.Sprintf("%s.%s (%s, NOT NULL) has no generator and no default; add one or the load fails", t.name, c.name, c.typ))
				} else {
					d.Notes = append(d.Notes, fmt.Sprintf("%s.%s (%s) is left NULL", t.name, c.name, c.typ))
				}
				continue
			}
			ts.Columns = append(ts.Columns, ColumnSpec{Name: c.name, Spec: spec})
		}
		if len(ts.Columns) == 0 {
			d.Notes = append(d.Notes, fmt.Sprintf("%s has no column to generate and is skipped", t.name))
			continue
		}
		d.File.Tables = append(d.File.Tables, ts)
	}
	return d
}

func columnGenerator(c *draftColumn, enums map[string][]any, generated map[string]bool, rowsOf func(string) int64) (gen.Spec, bool) {
	f := func(v float64) *float64 { return &v }
	integer := slices.Contains([]string{"int2", "int4", "int8", "smallint", "integer", "int", "bigint"}, c.typ)
	switch {
	case c.pk && integer:
		return gen.Spec{Generator: "sequence"}, true
	case c.ref != "" && integer && generated[c.ref]:
		return gen.Spec{Generator: "int_range", Min: f(1), Max: f(float64(rowsOf(c.ref)))}, true
	case integer:
		return gen.Spec{Generator: "int_range", Min: f(0), Max: f(1_000_000)}, true
	case slices.Contains([]string{"numeric", "decimal", "float4", "float8", "real"}, c.typ):
		return gen.Spec{Generator: "float_range", Min: f(0), Max: f(1000)}, true
	case slices.Contains([]string{"text", "varchar", "bpchar", "citext"}, c.typ):
		maxLen := 32
		if c.maxLen > 0 && c.maxLen < maxLen {
			maxLen = c.maxLen
		}
		return gen.Spec{Generator: "text", MinLength: min(5, maxLen), MaxLength: maxLen}, true
	case c.typ == "bool":
		return gen.Spec{Generator: "bool"}, true
	case c.typ == "uuid":
		return gen.Spec{Generator: "uuid"}, true
	case slices.Contains([]string{"timestamptz", "timestamp", "date"}, c.typ):
		return gen.Spec{Generator: "timestamp_range", From: "2025-01-01T00:00:00Z", To: "2026-01-01T00:00:00Z"}, true
	case enums[c.typ] != nil:
		return gen.Spec{Generator: "choice", Values: enums[c.typ]}, true
	}
	return gen.Spec{}, false
}

func tableName(rv *pg_query.RangeVar) string {
	if rv == nil {
		return ""
	}
	if rv.Schemaname != "" && rv.Schemaname != "public" {
		return rv.Schemaname + "." + rv.Relname
	}
	return rv.Relname
}

func lastPart(nodes []*pg_query.Node) string {
	n := names(nodes)
	if len(n) == 0 {
		return ""
	}
	return n[len(n)-1]
}

func names(nodes []*pg_query.Node) []string {
	var out []string
	for _, n := range nodes {
		if s := n.GetString_(); s != nil {
			out = append(out, s.Sval)
		}
	}
	return out
}

// Write renders the draft as a commented generator file.
func (d Draft) Write(w io.Writer) error {
	var b strings.Builder
	b.WriteString("# Draft generated by prodshadow draft-fixture from the fixture schema.\n")
	b.WriteString("# Review the row counts and value ranges: skewed keys (zipf), realistic\n")
	b.WriteString("# text lengths and NULL ratios make lock and latency behaviour closer\n")
	b.WriteString("# to production. Columns not listed keep their defaults.\n")
	for _, n := range d.Notes {
		fmt.Fprintf(&b, "# TODO: %s\n", n)
	}
	b.WriteString("version: 1\ntables:\n")
	for _, t := range d.File.Tables {
		fmt.Fprintf(&b, "  - name: %s\n    rows: %d\n    columns:\n", t.Name, t.Rows)
		for _, c := range t.Columns {
			fmt.Fprintf(&b, "      - {%s}\n", strings.Join(specFields(c), ", "))
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// specFields lists the non-empty fields of a column spec in flow style.
func specFields(c ColumnSpec) []string {
	q := strconv.Quote
	num := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	out := []string{"name: " + c.Name, "generator: " + c.Generator}
	if c.Min != nil {
		out = append(out, "min: "+num(*c.Min))
	}
	if c.Max != nil {
		out = append(out, "max: "+num(*c.Max))
	}
	if len(c.Values) > 0 {
		vals := make([]string, len(c.Values))
		for i, v := range c.Values {
			vals[i] = q(fmt.Sprint(v))
		}
		out = append(out, "values: ["+strings.Join(vals, ", ")+"]")
	}
	if c.MinLength > 0 {
		out = append(out, "min_length: "+strconv.Itoa(c.MinLength))
	}
	if c.MaxLength > 0 {
		out = append(out, "max_length: "+strconv.Itoa(c.MaxLength))
	}
	if c.From != "" {
		out = append(out, "from: "+q(c.From), "to: "+q(c.To))
	}
	return out
}
