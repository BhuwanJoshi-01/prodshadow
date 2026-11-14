package sqlinspect

import (
	"fmt"
	"strings"

	"github.com/BhuwanJoshi-01/prodshadow/internal/evidence"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

func init() {
	register(Rule{
		ID:       "PS133",
		Name:     "foreign-key-without-index",
		Summary:  "A foreign key's columns have no index, so deletes and key updates on the referenced table scan this table",
		Severity: evidence.Info,
		DocsURL:  "https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-FK",
		check:    checkForeignKeyIndex,
	})
}

// foreignKey is a foreign key added by a statement.
type foreignKey struct {
	table, refTable string
	columns         []string
}

// checkForeignKeyIndex flags foreign keys whose referencing columns are not
// the leading columns of any index or key the migration creates. PostgreSQL
// does not index referencing columns automatically.
func checkForeignKeyIndex(c *Context, s *Statement) []Finding {
	var out []Finding
	for _, fk := range addedForeignKeys(s) {
		if c.indexedAnywhere(fk.table, fk.columns) {
			continue
		}
		f := at(s, fmt.Sprintf("The foreign key on %s (%s) has no index on those columns in this migration. Unless one already exists, every DELETE or key UPDATE on %s scans %s to find referencing rows, while holding row locks.", fk.table, strings.Join(fk.columns, ", "), fk.refTable, fk.table))
		f.Table = fk.table
		f.Evidence, f.Confidence = evidence.Heuristic, evidence.Medium
		if !c.CreatedEarlier(s, fk.table) && s.Kind != KindCreateTable {
			// An existing table may already have a suitable index.
			f.Confidence = evidence.Low
		}
		f.Remediation = fmt.Sprintf("Create an index whose leading columns are (%s), concurrently if %s already has data.", strings.Join(fk.columns, ", "), fk.table)
		out = append(out, f)
	}
	return out
}

func addedForeignKeys(s *Statement) []foreignKey {
	if s.node == nil {
		return nil
	}
	var out []foreignKey
	add := func(table string, con *pg_query.Constraint, column string) {
		if con == nil || con.Contype != pg_query.ConstrType_CONSTR_FOREIGN {
			return
		}
		cols := stringItems(con.FkAttrs)
		if len(cols) == 0 && column != "" {
			cols = []string{column}
		}
		if len(cols) > 0 {
			out = append(out, foreignKey{table: table, refTable: relName(con.Pktable), columns: cols})
		}
	}
	switch {
	case s.node.GetCreateStmt() != nil:
		st := s.node.GetCreateStmt()
		table := relName(st.Relation)
		for _, e := range st.TableElts {
			add(table, e.GetConstraint(), "")
			if col := e.GetColumnDef(); col != nil {
				for _, n := range col.Constraints {
					add(table, n.GetConstraint(), col.Colname)
				}
			}
		}
		for _, n := range st.Constraints {
			add(table, n.GetConstraint(), "")
		}
	case s.Kind == KindAlterTable:
		st := s.node.GetAlterTableStmt()
		if st == nil {
			return nil
		}
		table := relName(st.Relation)
		for _, n := range st.Cmds {
			cmd := n.GetAlterTableCmd()
			if cmd == nil {
				continue
			}
			switch cmd.Subtype {
			case pg_query.AlterTableType_AT_AddConstraint:
				add(table, cmd.Def.GetConstraint(), "")
			case pg_query.AlterTableType_AT_AddColumn:
				if col := cmd.Def.GetColumnDef(); col != nil {
					for _, cn := range col.Constraints {
						add(table, cn.GetConstraint(), col.Colname)
					}
				}
			}
		}
	}
	return out
}

// indexedAnywhere reports whether any statement of the migration creates an
// index, primary key or unique constraint on table whose leading columns
// are exactly columns, in any order.
func (c *Context) indexedAnywhere(table string, columns []string) bool {
	for _, s := range c.Statements {
		for _, keys := range keysCreated(s, table) {
			if leadingMatch(keys, columns) {
				return true
			}
		}
	}
	return false
}

// keysCreated lists the column lists of indexes and keys s creates on table.
func keysCreated(s *Statement, table string) [][]string {
	if s.node == nil {
		return nil
	}
	var out [][]string
	key := func(con *pg_query.Constraint, column string) {
		if con == nil || (con.Contype != pg_query.ConstrType_CONSTR_PRIMARY && con.Contype != pg_query.ConstrType_CONSTR_UNIQUE) {
			return
		}
		cols := stringItems(con.Keys)
		if len(cols) == 0 && column != "" {
			cols = []string{column}
		}
		out = append(out, cols)
	}
	switch {
	case s.node.GetIndexStmt() != nil:
		st := s.node.GetIndexStmt()
		if relName(st.Relation) != table {
			return nil
		}
		var cols []string
		for _, p := range st.IndexParams {
			el := p.GetIndexElem()
			if el == nil || el.Name == "" {
				break // an expression ends the usable prefix
			}
			cols = append(cols, el.Name)
		}
		out = append(out, cols)
	case s.node.GetCreateStmt() != nil:
		st := s.node.GetCreateStmt()
		if relName(st.Relation) != table {
			return nil
		}
		for _, e := range st.TableElts {
			key(e.GetConstraint(), "")
			if col := e.GetColumnDef(); col != nil {
				for _, n := range col.Constraints {
					key(n.GetConstraint(), col.Colname)
				}
			}
		}
		for _, n := range st.Constraints {
			key(n.GetConstraint(), "")
		}
	case s.node.GetAlterTableStmt() != nil:
		st := s.node.GetAlterTableStmt()
		if relName(st.Relation) != table {
			return nil
		}
		for _, n := range st.Cmds {
			if cmd := n.GetAlterTableCmd(); cmd != nil && cmd.Subtype == pg_query.AlterTableType_AT_AddConstraint {
				key(cmd.Def.GetConstraint(), "")
			}
		}
	}
	return out
}

// leadingMatch reports whether the first len(columns) keys are columns, in
// any order.
func leadingMatch(keys, columns []string) bool {
	if len(keys) < len(columns) {
		return false
	}
	want := map[string]bool{}
	for _, c := range columns {
		want[c] = true
	}
	for _, k := range keys[:len(columns)] {
		if !want[k] {
			return false
		}
		delete(want, k)
	}
	return len(want) == 0
}
