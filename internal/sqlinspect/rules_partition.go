package sqlinspect

import (
	"fmt"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/pglock"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

func init() {
	register(Rule{
		ID:       "PS123",
		Name:     "attach-partition-scans",
		Summary:  "ATTACH PARTITION scans the partition to validate its bounds unless a matching CHECK constraint exists",
		Severity: evidence.Warning,
		DocsURL:  "https://www.postgresql.org/docs/current/sql-altertable.html#SQL-ALTERTABLE-ATTACH-PARTITION",
		check:    checkAttachPartition,
	})
}

// checkAttachPartition flags ATTACH PARTITION unless an earlier statement of
// the migration added a CHECK constraint to the partition, which is the
// documented way to let PostgreSQL skip the validation scan.
func checkAttachPartition(c *Context, s *Statement) []Finding {
	if s.Kind != KindAlterTable || s.node == nil {
		return nil
	}
	alter := s.node.GetAlterTableStmt()
	if alter == nil {
		return nil
	}
	var out []Finding
	for _, n := range alter.Cmds {
		cmd := n.GetAlterTableCmd()
		if cmd == nil || cmd.Subtype != pg_query.AlterTableType_AT_AttachPartition {
			continue
		}
		part := relName(cmd.Def.GetPartitionCmd().GetName())
		if part == "" || c.CreatedEarlier(s, part) || c.checkAddedEarlier(s, part) {
			continue
		}
		f := at(s, fmt.Sprintf("Attaching %s to %s scans every row of %s to validate the partition bounds while holding ACCESS EXCLUSIVE on %s, unless a CHECK constraint already implies the bounds.", part, relName(alter.Relation), part, part))
		f.Table, f.Lock = part, pglock.AccessExclusive
		f.Evidence, f.Confidence = evidence.Heuristic, evidence.Medium
		f.Remediation = "Before attaching, add a CHECK constraint matching the bounds (NOT VALID, then VALIDATE CONSTRAINT), attach, then drop the CHECK constraint."
		out = append(out, f)
	}
	return out
}

// checkAddedEarlier reports whether a statement before s adds a CHECK
// constraint to table.
func (c *Context) checkAddedEarlier(s *Statement, table string) bool {
	for _, o := range c.Statements[:c.position[s]] {
		if o.Kind != KindAlterTable || o.node == nil {
			continue
		}
		at := o.node.GetAlterTableStmt()
		if at == nil || relName(at.Relation) != table {
			continue
		}
		for _, n := range at.Cmds {
			cmd := n.GetAlterTableCmd()
			if cmd != nil && cmd.Subtype == pg_query.AlterTableType_AT_AddConstraint &&
				cmd.Def.GetConstraint().GetContype() == pg_query.ConstrType_CONSTR_CHECK {
				return true
			}
		}
	}
	return false
}
