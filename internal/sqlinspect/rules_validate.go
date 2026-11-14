package sqlinspect

import (
	"fmt"

	"github.com/BhuwanJoshi-01/prodshadow/internal/evidence"

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

func init() {
	register(Rule{
		ID:       "PS137",
		Name:     "constraint-left-not-valid",
		Summary:  "A constraint added NOT VALID is not validated later in the migration",
		Severity: evidence.Info,
		DocsURL:  docAlterTable,
		check:    checkLeftNotValid,
	})
}

// checkLeftNotValid reminds that a NOT VALID constraint still has to be
// validated. Validating it in a later migration is common and correct, so
// this is information, not a warning.
func checkLeftNotValid(c *Context, s *Statement) []Finding {
	table, cons := addedConstraints(c, s)
	var out []Finding
	for _, con := range cons {
		if !con.SkipValidation || con.Conname == "" || c.validatedLater(s, table, con.Conname) {
			continue
		}
		f := at(s, fmt.Sprintf("Constraint %s on %s is added NOT VALID and not validated later in this migration. New and updated rows are checked, but existing rows are not until ALTER TABLE %s VALIDATE CONSTRAINT %s runs, and until then a NOT VALID CHECK cannot stand in for the table scan of SET NOT NULL.", con.Conname, table, table, con.Conname))
		f.Table = table
		f.Evidence, f.Confidence = evidence.Static, evidence.High
		f.Remediation = "Validate it in the next migration, in its own transaction, so the constraint is fully enforced."
		out = append(out, f)
	}
	return out
}

// validatedLater reports whether a statement after s validates constraint
// name on table.
func (c *Context) validatedLater(s *Statement, table, name string) bool {
	for _, o := range c.Statements[c.position[s]+1:] {
		if o.Kind != KindAlterTable || o.node == nil || o.node.GetAlterTableStmt() == nil || relName(o.node.GetAlterTableStmt().Relation) != table {
			continue
		}
		for _, n := range o.node.GetAlterTableStmt().Cmds {
			if cmd := n.GetAlterTableCmd(); cmd != nil && cmd.Subtype == pg_query.AlterTableType_AT_ValidateConstraint && cmd.Name == name {
				return true
			}
		}
	}
	return false
}
