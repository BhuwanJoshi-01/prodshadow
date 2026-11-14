package sqlinspect

import (
	"slices"
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/pglock"
)

// catalogFK is a foreign key constraint known to the catalog.
type catalogFK struct {
	name       string
	table      string   // referencing table
	columns    []string // referencing columns
	refTable   string   // referenced table
	refColumns []string // referenced columns; empty means the primary key
	onDelete   string   // pg_query action code: a, r, c, n, d
	onUpdate   string
}

// cascades reports whether a referential action modifies the referencing
// rows (CASCADE, SET NULL, SET DEFAULT) instead of only checking them.
func cascades(action string) bool { return action == "c" || action == "n" || action == "d" }

// learnForeignKey records a FOREIGN KEY constraint of table. For a column
// constraint, column is the column it is declared on.
func (c *Catalog) learnForeignKey(rel *pg_query.RangeVar, column string, con *pg_query.Constraint) {
	if con.Contype != pg_query.ConstrType_CONSTR_FOREIGN {
		return
	}
	cols := stringItems(con.FkAttrs)
	if len(cols) == 0 && column != "" {
		cols = []string{column}
	}
	name := con.Conname
	if name == "" {
		name = rel.GetRelname() + "_" + strings.Join(cols, "_") + "_fkey"
	}
	c.foreignKeys = append(c.foreignKeys, catalogFK{
		name: name, table: relName(rel), columns: cols,
		refTable: relName(con.Pktable), refColumns: stringItems(con.PkAttrs),
		onDelete: con.FkDelAction, onUpdate: con.FkUpdAction,
	})
}

// learnKey records the columns of a PRIMARY KEY constraint, which foreign
// keys without a column list refer to.
func (c *Catalog) learnKey(rel *pg_query.RangeVar, column string, con *pg_query.Constraint) {
	if con.Contype != pg_query.ConstrType_CONSTR_PRIMARY {
		return
	}
	cols := stringItems(con.Keys)
	if len(cols) == 0 && column != "" {
		cols = []string{column}
	}
	c.primaryKeys[catalogKey(relName(rel))] = cols
}

// learnTableConstraints records keys and foreign keys of a CREATE TABLE.
func (c *Catalog) learnTableConstraints(st *pg_query.CreateStmt) {
	learn := func(column string, n *pg_query.Node) {
		if con := n.GetConstraint(); con != nil {
			c.learnForeignKey(st.Relation, column, con)
			c.learnKey(st.Relation, column, con)
		}
	}
	for _, n := range st.Constraints {
		learn("", n)
	}
	for _, e := range st.TableElts {
		if col := e.GetColumnDef(); col != nil {
			for _, n := range col.Constraints {
				learn(col.Colname, n)
			}
			continue
		}
		learn("", e)
	}
}

// referencing returns the foreign keys that point at table.
func (c *Catalog) referencing(table string) []catalogFK {
	var out []catalogFK
	for _, fk := range c.foreignKeys {
		if catalogKey(fk.refTable) == catalogKey(table) {
			out = append(out, fk)
		}
	}
	return out
}

// referencedBy returns the foreign keys declared on table.
func (c *Catalog) referencedBy(table string) []catalogFK {
	var out []catalogFK
	for _, fk := range c.foreignKeys {
		if catalogKey(fk.table) == catalogKey(table) {
			out = append(out, fk)
		}
	}
	return out
}

// keyColumns returns the referenced columns of fk.
func (c *Catalog) keyColumns(fk catalogFK) []string {
	if len(fk.refColumns) > 0 {
		return fk.refColumns
	}
	return c.primaryKeys[catalogKey(fk.refTable)]
}

// dmlForeignKeyLocks adds the locks of referential integrity triggers.
// Inserting or changing a referencing row checks the referenced row (ROW
// SHARE through SELECT ... FOR KEY SHARE); deleting or re-keying a
// referenced row checks the referencing rows (ROW SHARE) or, with ON
// DELETE/UPDATE CASCADE, SET NULL or SET DEFAULT, changes them (ROW
// EXCLUSIVE), which can cascade further. The triggers run only for rows
// the statement actually affects, so these are the locks it may take.
func (c *Catalog) dmlForeignKeyLocks(s *Statement) {
	switch x := s.node.Node.(type) {
	case *pg_query.Node_InsertStmt:
		for _, fk := range c.referencedBy(relName(x.InsertStmt.Relation)) {
			s.addImpliedLock(fk.refTable, pglock.RowShare, "foreign key "+fk.name)
		}
	case *pg_query.Node_UpdateStmt:
		table := relName(x.UpdateStmt.Relation)
		set := make([]string, 0, len(x.UpdateStmt.TargetList))
		for _, t := range x.UpdateStmt.TargetList {
			set = append(set, t.GetResTarget().GetName())
		}
		changes := func(cols []string) bool {
			return slices.ContainsFunc(cols, func(col string) bool { return slices.Contains(set, col) })
		}
		for _, fk := range c.referencedBy(table) {
			if changes(fk.columns) {
				s.addImpliedLock(fk.refTable, pglock.RowShare, "foreign key "+fk.name)
			}
		}
		for _, fk := range c.referencing(table) {
			if changes(c.keyColumns(fk)) {
				c.referentialAction(s, fk, fk.onUpdate, nil)
			}
		}
	case *pg_query.Node_DeleteStmt:
		c.deleteCascade(s, relName(x.DeleteStmt.Relation), map[string]bool{})
	}
}

// deleteCascade adds the locks of deleting rows from table.
func (c *Catalog) deleteCascade(s *Statement, table string, seen map[string]bool) {
	if seen[catalogKey(table)] {
		return
	}
	seen[catalogKey(table)] = true
	for _, fk := range c.referencing(table) {
		c.referentialAction(s, fk, fk.onDelete, seen)
	}
}

// referentialAction adds the lock of one referential action. seen is
// non-nil for deletes, whose CASCADE deletes rows of the referencing table
// and so reaches the tables that reference it in turn.
func (c *Catalog) referentialAction(s *Statement, fk catalogFK, action string, seen map[string]bool) {
	if !cascades(action) {
		s.addImpliedLock(fk.table, pglock.RowShare, "foreign key "+fk.name)
		return
	}
	s.addImpliedLock(fk.table, pglock.RowExclusive, "foreign key "+fk.name)
	if action == "c" && seen != nil {
		c.deleteCascade(s, fk.table, seen)
	}
}
