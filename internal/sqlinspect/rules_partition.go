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

func init() {
	register(Rule{
		ID:       "PS140",
		Name:     "detach-partition-blocks-parent",
		Summary:  "DETACH PARTITION without CONCURRENTLY takes ACCESS EXCLUSIVE on the partitioned table",
		Severity: evidence.Warning,
		DocsURL:  "https://www.postgresql.org/docs/current/sql-altertable.html#SQL-ALTERTABLE-DETACH-PARTITION",
		check:    checkDetachPartition,
	})
	register(Rule{
		ID:       "PS141",
		Name:     "create-partition-locks-parent",
		Summary:  "CREATE TABLE ... PARTITION OF takes ACCESS EXCLUSIVE on the partitioned table",
		Severity: evidence.Warning,
		DocsURL:  docCreateTable,
		check:    checkCreatePartition,
	})
}

func checkDetachPartition(c *Context, s *Statement) []Finding {
	if s.Kind != KindAlterTable || s.node == nil || s.node.GetAlterTableStmt() == nil {
		return nil
	}
	alter := s.node.GetAlterTableStmt()
	parent := relName(alter.Relation)
	if c.CreatedEarlier(s, parent) {
		return nil
	}
	var out []Finding
	for _, n := range alter.Cmds {
		cmd := n.GetAlterTableCmd()
		if cmd == nil || cmd.Subtype != pg_query.AlterTableType_AT_DetachPartition {
			continue
		}
		pc := cmd.Def.GetPartitionCmd()
		if pc == nil || pc.Concurrent {
			continue
		}
		f := at(s, fmt.Sprintf("Detaching %s takes ACCESS EXCLUSIVE on the partitioned table %s, so every query on any partition of %s waits for the detach, and queues behind it while it waits.", relName(pc.Name), parent, parent))
		f.Table, f.Lock = parent, pglock.AccessExclusive
		f.Evidence, f.Confidence = evidence.Static, evidence.High
		f.Remediation = "Use DETACH PARTITION ... CONCURRENTLY (PostgreSQL 14+), which takes SHARE UPDATE EXCLUSIVE on the parent; it cannot run in a transaction block."
		out = append(out, f)
	}
	return out
}

func checkCreatePartition(c *Context, s *Statement) []Finding {
	if s.Kind != KindCreateTable || s.node == nil || s.node.GetCreateStmt() == nil {
		return nil
	}
	st := s.node.GetCreateStmt()
	if st.Partbound == nil {
		return nil
	}
	var out []Finding
	for _, p := range st.InhRelations {
		parent := relName(p.GetRangeVar())
		if parent == "" || c.CreatedEarlier(s, parent) {
			continue
		}
		f := at(s, fmt.Sprintf("Creating partition %s with PARTITION OF takes ACCESS EXCLUSIVE on %s, which blocks every read and write of the partitioned table until the transaction commits.", relName(st.Relation), parent))
		f.Table, f.Lock = parent, pglock.AccessExclusive
		f.Evidence, f.Confidence = evidence.Static, evidence.High
		f.Remediation = "Create the table on its own (CREATE TABLE ... (LIKE parent INCLUDING ALL)), add a CHECK constraint matching the bounds, then ATTACH PARTITION, which only takes SHARE UPDATE EXCLUSIVE on the parent."
		out = append(out, f)
	}
	return out
}
