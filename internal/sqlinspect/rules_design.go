package sqlinspect

import (
	"fmt"
	"slices"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

func init() {
	register(Rule{
		ID:       "PS152",
		Name:     "int-primary-key",
		Summary:  "A new table's primary key is a 32-bit integer, which can run out of values",
		Severity: evidence.Info,
		DocsURL:  "https://www.postgresql.org/docs/current/datatype-numeric.html#DATATYPE-INT",
		check:    checkIntPrimaryKey,
	})
}

// smallIntegerTypes hold at most 2^31-1 (or 2^15-1) positive values.
var smallIntegerTypes = []string{"int", "int4", "integer", "int2", "smallint", "serial", "serial4", "smallserial", "serial2"}

// checkIntPrimaryKey flags single-column integer primary keys on new
// tables. Changing the type later rewrites the table and every table whose
// foreign keys reference it, under ACCESS EXCLUSIVE.
func checkIntPrimaryKey(_ *Context, s *Statement) []Finding {
	if s.node == nil || s.node.GetCreateStmt() == nil {
		return nil
	}
	st := s.node.GetCreateStmt()
	if st.GetRelation().GetRelpersistence() == "t" {
		return nil
	}
	types := map[string]string{}
	for _, e := range st.TableElts {
		if col := e.GetColumnDef(); col != nil {
			types[col.Colname] = typeName(col.GetTypeName())
		}
	}
	var pk []string
	for _, con := range createTableConstraints(st) {
		if con.Contype == pg_query.ConstrType_CONSTR_PRIMARY {
			pk = stringItems(con.Keys)
		}
	}
	if len(pk) == 0 {
		for _, e := range st.TableElts {
			if col := e.GetColumnDef(); col != nil && hasConstraint(col, pg_query.ConstrType_CONSTR_PRIMARY) {
				pk = []string{col.Colname}
			}
		}
	}
	if len(pk) != 1 || !slices.Contains(smallIntegerTypes, types[pk[0]]) {
		return nil
	}
	table := relName(st.Relation)
	f := at(s, fmt.Sprintf("The primary key %s.%s is %s, which holds at most %s values. Tables that grow, or whose sequence skips values, can run out; changing the type then rewrites %s and every table referencing it.", table, pk[0], types[pk[0]], limitOf(types[pk[0]]), table))
	f.Table = table
	f.Evidence, f.Confidence = evidence.Heuristic, evidence.Medium
	f.Remediation = "Use bigint (or bigint GENERATED ALWAYS AS IDENTITY) for primary keys of tables that can grow; it costs four bytes per row."
	return []Finding{f}
}

func limitOf(t string) string {
	if t == "int2" || t == "smallint" || t == "smallserial" || t == "serial2" {
		return "32,767"
	}
	return "2,147,483,647"
}

func init() {
	register(Rule{
		ID:       "PS153",
		Name:     "table-without-primary-key",
		Summary:  "A new table has no primary key, which logical replication and many tools need",
		Severity: evidence.Info,
		DocsURL:  "https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-PRIMARY-KEYS",
		check:    checkTableWithoutPrimaryKey,
	})
}

// checkTableWithoutPrimaryKey flags permanent tables created without a
// primary key. Partitions (which inherit it), tables created with LIKE
// (which may copy it) and tables that get one later in the migration are
// not reported.
func checkTableWithoutPrimaryKey(c *Context, s *Statement) []Finding {
	if s.node == nil || s.node.GetCreateStmt() == nil {
		return nil
	}
	st := s.node.GetCreateStmt()
	if st.GetRelation().GetRelpersistence() == "t" || st.Partbound != nil || len(st.InhRelations) > 0 || st.OfTypename != nil {
		return nil
	}
	for _, e := range st.TableElts {
		if e.GetTableLikeClause() != nil {
			return nil
		}
		if col := e.GetColumnDef(); col != nil && hasConstraint(col, pg_query.ConstrType_CONSTR_PRIMARY) {
			return nil
		}
	}
	for _, con := range createTableConstraints(st) {
		if con.Contype == pg_query.ConstrType_CONSTR_PRIMARY {
			return nil
		}
	}
	table := relName(st.Relation)
	if c.primaryKeyAddedLater(s, table) {
		return nil
	}
	f := at(s, fmt.Sprintf("Table %s has no primary key. Rows cannot be identified reliably, logical replication cannot replicate its updates and deletes without REPLICA IDENTITY FULL, and many tools (ORMs, online schema change, data sync) refuse or misbehave.", table))
	f.Table = table
	f.Evidence, f.Confidence = evidence.Static, evidence.High
	f.Remediation = "Add a primary key, for example id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, or a natural key that is unique and not null."
	return []Finding{f}
}

// primaryKeyAddedLater reports whether a statement after s adds a primary
// key to table.
func (c *Context) primaryKeyAddedLater(s *Statement, table string) bool {
	for _, o := range c.Statements[c.position[s]+1:] {
		if o.node == nil || o.node.GetAlterTableStmt() == nil || relName(o.node.GetAlterTableStmt().Relation) != table {
			continue
		}
		for _, n := range o.node.GetAlterTableStmt().Cmds {
			cmd := n.GetAlterTableCmd()
			if cmd == nil {
				continue
			}
			if con := cmd.Def.GetConstraint(); con != nil && cmd.Subtype == pg_query.AlterTableType_AT_AddConstraint && con.Contype == pg_query.ConstrType_CONSTR_PRIMARY {
				return true
			}
			if col := cmd.Def.GetColumnDef(); col != nil && cmd.Subtype == pg_query.AlterTableType_AT_AddColumn && hasConstraint(col, pg_query.ConstrType_CONSTR_PRIMARY) {
				return true
			}
		}
	}
	return false
}
