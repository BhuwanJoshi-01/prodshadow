package sqlinspect

import (
	"fmt"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

func init() {
	register(Rule{
		ID:       "PS136",
		Name:     "validated-in-same-transaction",
		Summary:  "VALIDATE CONSTRAINT runs in the transaction that added the constraint NOT VALID, so the scan happens under the stronger lock",
		Severity: evidence.Warning,
		DocsURL:  docAlterTable,
		check:    checkValidateSameTransaction,
	})
}

// checkValidateSameTransaction flags VALIDATE CONSTRAINT when the same
// transaction added that constraint NOT VALID. The point of NOT VALID is to
// split the work: a short ADD under a strong lock, then a scan under SHARE
// UPDATE EXCLUSIVE. In one transaction the strong lock is still held while
// the scan runs.
func checkValidateSameTransaction(c *Context, s *Statement) []Finding {
	table, cmds := alterCmds(c, s)
	g := c.Group(s)
	if !g.InTransaction() {
		return nil
	}
	var out []Finding
	for _, cmd := range cmds {
		if cmd.Subtype != pg_query.AlterTableType_AT_ValidateConstraint {
			continue
		}
		add := addedNotValidEarlier(g, s, table, cmd.Name)
		if add == nil {
			continue
		}
		mode := add.LockOn(table)
		f := at(s, fmt.Sprintf("Constraint %s is validated in the same transaction that added it NOT VALID (statement %s). The %s lock taken by the ADD is held until COMMIT, so the validation scan runs under it, which %s, exactly what NOT VALID was meant to avoid.", cmd.Name, add.ID, mode, impactPhrase(mode)))
		f.Table, f.Lock = table, mode
		f.Evidence, f.Confidence = evidence.Static, evidence.High
		f.Remediation = "Commit after adding the constraint NOT VALID and run VALIDATE CONSTRAINT in a separate transaction, for example in the next migration file with migration.transaction: per-file."
		out = append(out, f)
	}
	return out
}

// addedNotValidEarlier returns the statement of group g, before s, that adds
// constraint name to table with NOT VALID.
func addedNotValidEarlier(g Group, s *Statement, table, name string) *Statement {
	for _, o := range g.Statements {
		if o == s {
			return nil
		}
		if o.Kind != KindAlterTable || o.node == nil || o.node.GetAlterTableStmt() == nil {
			continue
		}
		st := o.node.GetAlterTableStmt()
		if relName(st.Relation) != table {
			continue
		}
		for _, n := range st.Cmds {
			cmd := n.GetAlterTableCmd()
			if cmd == nil || cmd.Subtype != pg_query.AlterTableType_AT_AddConstraint {
				continue
			}
			if con := cmd.Def.GetConstraint(); con != nil && con.SkipValidation && con.Conname == name {
				return o
			}
		}
	}
	return nil
}
