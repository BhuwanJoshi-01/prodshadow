package sqlinspect

import (
	"fmt"
	"slices"
	"strings"

	"github.com/BhuwanJoshi-01/prodshadow/internal/pglock"

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
	delete(c.viewOutputs, catalogKey(view))
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

// outputColumns returns the column names a view defines: its explicit
// column list, or the names PostgreSQL derives from the select list (the
// alias, the column, the function name, or ?column?). SELECT * expands to
// the table's known columns. It returns nil when it cannot tell.
func (c *Catalog) outputColumns(st *pg_query.ViewStmt) []string {
	sel := st.Query.GetSelectStmt()
	if sel == nil || len(sel.TargetList) == 0 {
		return nil
	}
	var cols []string
	for _, t := range sel.TargetList {
		rt := t.GetResTarget()
		switch {
		case rt.Name != "":
			cols = append(cols, rt.Name)
		case rt.Val.GetColumnRef() != nil:
			fields := rt.Val.GetColumnRef().Fields
			last := fields[len(fields)-1]
			if last.GetAStar() != nil {
				if len(sel.FromClause) != 1 || sel.FromClause[0].GetRangeVar() == nil {
					return nil
				}
				known := c.columns[catalogKey(relName(sel.FromClause[0].GetRangeVar()))]
				if len(known) == 0 {
					return nil
				}
				cols = append(cols, known...)
				continue
			}
			cols = append(cols, last.GetString_().GetSval())
		case rt.Val.GetFuncCall() != nil:
			names := stringItems(rt.Val.GetFuncCall().Funcname)
			cols = append(cols, names[len(names)-1])
		case rt.Val.GetTypeCast() != nil && rt.Val.GetTypeCast().Arg.GetColumnRef() != nil:
			fields := rt.Val.GetTypeCast().Arg.GetColumnRef().Fields
			cols = append(cols, fields[len(fields)-1].GetString_().GetSval())
		default:
			cols = append(cols, "?column?")
		}
	}
	if aliases := stringItems(st.Aliases); len(aliases) > 0 {
		for i := range min(len(aliases), len(cols)) {
			cols[i] = aliases[i]
		}
	}
	return cols
}

// viewReplaceProblem explains why CREATE OR REPLACE VIEW will be rejected:
// the new definition must keep the existing columns, with the same names,
// in the same order, and may only add columns at the end.
func (c *Catalog) viewReplaceProblem(st *pg_query.ViewStmt) string {
	if !st.Replace {
		return ""
	}
	old := c.viewOutputs[catalogKey(relName(st.View))]
	cols := c.outputColumns(st)
	if old == nil || cols == nil {
		return ""
	}
	for i, name := range old {
		switch {
		case i >= len(cols):
			return fmt.Sprintf("it drops column %s (PostgreSQL cannot drop columns from a view)", name)
		case cols[i] != name:
			return fmt.Sprintf("column %d is %s instead of %s (columns cannot be renamed or reordered)", i+1, cols[i], name)
		}
	}
	return ""
}

// viewDependency is a view that makes a statement fail.
type viewDependency struct {
	view   string
	column string
	change string // drop, type or table
}

// isView reports whether name is a (non-materialized) view.
func (c *Catalog) isView(name string) bool {
	key := catalogKey(name)
	_, known := c.viewReads[key]
	_, mat := c.matviews[key]
	return known && !mat
}

// viewBaseLocks adds the locks a statement takes on the relations behind
// the views it uses: PostgreSQL rewrites a query on a view into a query on
// its base tables, and LOCK TABLE on a view locks them too. A simple view
// over one table passes the statement's lock through (an UPDATE through
// it takes ROW EXCLUSIVE on the table); the tables of a view that joins
// several are read (ACCESS SHARE), except for LOCK TABLE, which locks
// them all in the requested mode. Views on views are followed.
func (c *Catalog) viewBaseLocks(s *Statement) {
	switch s.Kind {
	case KindSelect, KindInsert, KindUpdate, KindDelete, KindMerge, KindLockTable:
	default:
		return // DDL on a view (DROP, RENAME, ...) does not touch its tables
	}
	lock := s.Kind == KindLockTable
	var visit func(view string, mode pglock.Mode, depth int)
	visit = func(view string, mode pglock.Mode, depth int) {
		if depth > 8 {
			return
		}
		reads := c.viewReads[catalogKey(view)]
		for _, t := range reads {
			m := mode
			if len(reads) > 1 && !lock {
				m = pglock.AccessShare
			}
			s.addImpliedLock(t, m, "view "+view)
			if c.isView(t) {
				visit(t, m, depth+1)
			}
		}
	}
	for _, l := range slices.Clone(s.Locks) {
		if l.Via == "" && l.CreatedBy == "" && c.isView(l.Table) {
			visit(l.Table, l.Mode, 0)
		}
	}
}
