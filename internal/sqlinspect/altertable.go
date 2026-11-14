package sqlinspect

import (
	"strings"
	"unicode"

	"github.com/BhuwanJoshi-01/prodshadow/internal/pglock"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

// classifyAlterTable records one subcommand label per ALTER TABLE action and
// the strongest lock among them. Most actions take ACCESS EXCLUSIVE; the
// exceptions documented in the ALTER TABLE reference are listed here.
func classifyAlterTable(s *Statement, st *pg_query.AlterTableStmt) {
	s.Kind, s.Tag = KindAlterTable, "ALTER "+objectWord(st.Objtype)
	table := relName(st.Relation)
	for _, n := range st.Cmds {
		cmd := n.GetAlterTableCmd()
		if cmd == nil {
			continue
		}
		label, mode := alterTableCmd(s, cmd)
		s.Subcommands = append(s.Subcommands, label)
		s.addLock(table, mode)
	}
}

func objectWord(t pg_query.ObjectType) string {
	switch t {
	case pg_query.ObjectType_OBJECT_INDEX:
		return "INDEX"
	case pg_query.ObjectType_OBJECT_MATVIEW:
		return "MATERIALIZED VIEW"
	case pg_query.ObjectType_OBJECT_VIEW:
		return "VIEW"
	case pg_query.ObjectType_OBJECT_SEQUENCE:
		return "SEQUENCE"
	case pg_query.ObjectType_OBJECT_FOREIGN_TABLE:
		return "FOREIGN TABLE"
	default:
		return "TABLE"
	}
}

// alterTableCmd returns a stable label and the lock mode for one action.
// It may add locks on other relations (foreign key targets, partitions).
func alterTableCmd(s *Statement, cmd *pg_query.AlterTableCmd) (string, pglock.Mode) {
	switch cmd.Subtype {
	case pg_query.AlterTableType_AT_AddColumn:
		// An inline REFERENCES clause also locks the referenced table.
		if col := cmd.Def.GetColumnDef(); col != nil {
			for _, c := range col.Constraints {
				if con := c.GetConstraint(); con != nil && con.Contype == pg_query.ConstrType_CONSTR_FOREIGN {
					s.addLock(relName(con.Pktable), pglock.ShareRowExclusive)
				}
			}
		}
		return "add_column", pglock.AccessExclusive
	case pg_query.AlterTableType_AT_DropColumn:
		return "drop_column", pglock.AccessExclusive
	case pg_query.AlterTableType_AT_AlterColumnType:
		return "alter_column_type", pglock.AccessExclusive
	case pg_query.AlterTableType_AT_SetNotNull:
		return "set_not_null", pglock.AccessExclusive
	case pg_query.AlterTableType_AT_DropNotNull:
		return "drop_not_null", pglock.AccessExclusive
	case pg_query.AlterTableType_AT_ColumnDefault:
		if cmd.Def == nil {
			return "drop_default", pglock.AccessExclusive
		}
		return "set_default", pglock.AccessExclusive
	case pg_query.AlterTableType_AT_AddConstraint:
		return addConstraint(s, cmd.Def.GetConstraint())
	case pg_query.AlterTableType_AT_AddIndexConstraint:
		return "add_constraint_using_index", pglock.AccessExclusive
	case pg_query.AlterTableType_AT_ValidateConstraint:
		return "validate_constraint", pglock.ShareUpdateExclusive
	case pg_query.AlterTableType_AT_DropConstraint:
		return "drop_constraint", pglock.AccessExclusive
	case pg_query.AlterTableType_AT_SetStatistics,
		pg_query.AlterTableType_AT_SetOptions,
		pg_query.AlterTableType_AT_ResetOptions,
		pg_query.AlterTableType_AT_ClusterOn,
		pg_query.AlterTableType_AT_DropCluster,
		pg_query.AlterTableType_AT_SetRelOptions,
		pg_query.AlterTableType_AT_ResetRelOptions,
		pg_query.AlterTableType_AT_ReplaceRelOptions:
		return snakeLabel(cmd.Subtype.String()), pglock.ShareUpdateExclusive
	case pg_query.AlterTableType_AT_EnableTrig,
		pg_query.AlterTableType_AT_EnableAlwaysTrig,
		pg_query.AlterTableType_AT_EnableReplicaTrig,
		pg_query.AlterTableType_AT_EnableTrigAll,
		pg_query.AlterTableType_AT_EnableTrigUser,
		pg_query.AlterTableType_AT_DisableTrig,
		pg_query.AlterTableType_AT_DisableTrigAll,
		pg_query.AlterTableType_AT_DisableTrigUser:
		return snakeLabel(cmd.Subtype.String()), pglock.ShareRowExclusive
	case pg_query.AlterTableType_AT_AttachPartition:
		if pc := cmd.Def.GetPartitionCmd(); pc != nil {
			s.addLock(relName(pc.Name), pglock.AccessExclusive)
		}
		return "attach_partition", pglock.ShareUpdateExclusive
	case pg_query.AlterTableType_AT_DetachPartition:
		pc := cmd.Def.GetPartitionCmd()
		if pc != nil {
			s.addLock(relName(pc.Name), pglock.AccessExclusive)
			if pc.Concurrent {
				s.Txn = TxnNotInBlock
				return "detach_partition_concurrently", pglock.ShareUpdateExclusive
			}
		}
		return "detach_partition", pglock.AccessExclusive
	case pg_query.AlterTableType_AT_SetTableSpace:
		return "set_tablespace", pglock.AccessExclusive
	case pg_query.AlterTableType_AT_SetLogged:
		return "set_logged", pglock.AccessExclusive
	case pg_query.AlterTableType_AT_SetUnLogged:
		return "set_unlogged", pglock.AccessExclusive
	case pg_query.AlterTableType_AT_SetAccessMethod:
		return "set_access_method", pglock.AccessExclusive
	default:
		return snakeLabel(cmd.Subtype.String()), pglock.AccessExclusive
	}
}

// addConstraint classifies ADD CONSTRAINT. Foreign keys take SHARE ROW
// EXCLUSIVE on both tables; other constraint types take ACCESS EXCLUSIVE.
func addConstraint(s *Statement, c *pg_query.Constraint) (string, pglock.Mode) {
	if c == nil {
		return "add_constraint", pglock.AccessExclusive
	}
	suffix := ""
	if c.SkipValidation {
		suffix = "_not_valid"
	}
	switch c.Contype {
	case pg_query.ConstrType_CONSTR_FOREIGN:
		s.addLock(relName(c.Pktable), pglock.ShareRowExclusive)
		return "add_foreign_key" + suffix, pglock.ShareRowExclusive
	case pg_query.ConstrType_CONSTR_CHECK:
		return "add_check" + suffix, pglock.AccessExclusive
	case pg_query.ConstrType_CONSTR_UNIQUE:
		if c.Indexname != "" {
			return "add_unique_using_index", pglock.AccessExclusive
		}
		return "add_unique", pglock.AccessExclusive
	case pg_query.ConstrType_CONSTR_PRIMARY:
		if c.Indexname != "" {
			return "add_primary_key_using_index", pglock.AccessExclusive
		}
		return "add_primary_key", pglock.AccessExclusive
	case pg_query.ConstrType_CONSTR_EXCLUSION:
		return "add_exclusion", pglock.AccessExclusive
	case pg_query.ConstrType_CONSTR_NOTNULL:
		return "add_not_null" + suffix, pglock.AccessExclusive
	default:
		return "add_constraint", pglock.AccessExclusive
	}
}

// snakeLabel converts "AT_SetStatistics" into "set_statistics".
func snakeLabel(enum string) string {
	name := strings.TrimPrefix(enum, "AT_")
	var b strings.Builder
	for i, r := range name {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('_')
			}
			r = unicode.ToLower(r)
		}
		b.WriteRune(r)
	}
	return b.String()
}
