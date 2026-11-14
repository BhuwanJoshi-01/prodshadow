package sqlinspect

import (
	"slices"
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v6"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// learnViewColumns records which columns of which tables a view's query
// reads. Qualified references (o.status) are resolved through the FROM
// aliases; unqualified ones are attributed to every table of the query
// that has such a column, or to every table when the columns are unknown.
// SELECT * depends on all columns the table had when the view was created.
func (c *Catalog) learnViewColumns(view string, query *pg_query.Node) {
	c.forgetView(view)             // CREATE OR REPLACE VIEW replaces the dependencies
	aliases := map[string]string{} // alias or table name -> table
	var tables []string
	walk(query, func(m protoreflect.Message) bool {
		if rv, ok := m.Interface().(*pg_query.RangeVar); ok {
			t := relName(rv)
			tables = append(tables, t)
			aliases[strings.ToLower(rv.Relname)] = t
			if rv.Alias != nil {
				aliases[strings.ToLower(rv.Alias.Aliasname)] = t
			}
		}
		return true
	})
	use := func(table, column string) {
		key := catalogKey(table)
		if c.viewColumns[key] == nil {
			c.viewColumns[key] = map[string][]string{}
		}
		if !slices.Contains(c.viewColumns[key][column], view) {
			c.viewColumns[key][column] = append(c.viewColumns[key][column], view)
		}
	}
	walk(query, func(m protoreflect.Message) bool {
		ref, ok := m.Interface().(*pg_query.ColumnRef)
		if !ok {
			return true
		}
		var names []string
		star := false
		for _, f := range ref.Fields {
			switch {
			case f.GetString_() != nil:
				names = append(names, f.GetString_().Sval)
			case f.GetAStar() != nil:
				star = true
			}
		}
		targets := tables
		if len(names) > 0 && (star || len(names) > 1) {
			// The first name qualifies: alias.column or alias.*.
			if t, ok := aliases[strings.ToLower(names[0])]; ok {
				targets = []string{t}
			}
		}
		if star {
			for _, t := range targets {
				for _, col := range c.columns[catalogKey(t)] {
					use(t, col)
				}
			}
			return true
		}
		if len(names) == 0 {
			return true
		}
		col := names[len(names)-1]
		known := false
		for _, t := range targets {
			if slices.Contains(c.columns[catalogKey(t)], col) {
				use(t, col)
				known = true
			}
		}
		if !known {
			for _, t := range targets {
				if len(c.columns[catalogKey(t)]) == 0 {
					use(t, col)
				}
			}
		}
		return true
	})
}

// forgetView removes a view's column dependencies, when it is dropped or
// replaced.
func (c *Catalog) forgetView(view string) {
	for _, cols := range c.viewColumns {
		for col, views := range cols {
			cols[col] = slices.DeleteFunc(views, func(v string) bool { return catalogKey(v) == catalogKey(view) })
		}
	}
}

// viewsUsing returns the views that read table.column, or, with column
// empty, any column of table.
func (c *Catalog) viewsUsing(table, column string) []string {
	var out []string
	for col, views := range c.viewColumns[catalogKey(table)] {
		if column == "" || col == column {
			for _, v := range views {
				if !slices.Contains(out, v) {
					out = append(out, v)
				}
			}
		}
	}
	slices.Sort(out)
	return out
}

// viewDependencies records on DROP COLUMN, ALTER COLUMN TYPE and DROP
// TABLE statements without CASCADE the views PostgreSQL will refuse to
// leave behind (PS159).
func (c *Catalog) viewDependencies(s *Statement) {
	switch x := s.node.Node.(type) {
	case *pg_query.Node_AlterTableStmt:
		table := relName(x.AlterTableStmt.Relation)
		for _, n := range x.AlterTableStmt.Cmds {
			at := n.GetAlterTableCmd()
			switch {
			case at.GetSubtype() == pg_query.AlterTableType_AT_DropColumn && at.Behavior != pg_query.DropBehavior_DROP_CASCADE:
				for _, v := range c.viewsUsing(table, at.Name) {
					s.dependentViews = append(s.dependentViews, viewDependency{view: v, column: at.Name, change: "drop"})
				}
			case at.GetSubtype() == pg_query.AlterTableType_AT_AlterColumnType:
				for _, v := range c.viewsUsing(table, at.Name) {
					s.dependentViews = append(s.dependentViews, viewDependency{view: v, column: at.Name, change: "type"})
				}
			}
		}
	case *pg_query.Node_DropStmt:
		if x.DropStmt.RemoveType != pg_query.ObjectType_OBJECT_TABLE || x.DropStmt.Behavior == pg_query.DropBehavior_DROP_CASCADE {
			return
		}
		for _, t := range dropObjectNames(x.DropStmt.Objects) {
			for _, v := range c.viewsUsing(t, "") {
				s.dependentViews = append(s.dependentViews, viewDependency{view: v, change: "table"})
			}
		}
	}
}

// viewDependency is a view that makes a statement fail.
type viewDependency struct {
	view   string
	column string
	change string // drop, type or table
}
