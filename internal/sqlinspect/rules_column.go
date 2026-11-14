package sqlinspect

import (
	"fmt"

	"github.com/BhuwanJoshi-01/prodshadow/internal/evidence"
	"github.com/BhuwanJoshi-01/prodshadow/internal/pglock"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

func init() {
	register(Rule{
		ID:       "PS103",
		Name:     "add-column-rewrites-table",
		Summary:  "ADD COLUMN with a volatile default, serial, identity or stored generated column rewrites the table",
		Severity: evidence.Warning,
		DocsURL:  docAlterTable,
		check:    checkAddColumnRewrite,
	})
	register(Rule{
		ID:       "PS104",
		Name:     "add-column-not-null-without-default",
		Summary:  "ADD COLUMN ... NOT NULL without a default fails on a non-empty table",
		Severity: evidence.Warning,
		DocsURL:  docAlterTable,
		check:    checkAddNotNullColumn,
	})
	register(Rule{
		ID:       "PS105",
		Name:     "alter-column-type",
		Summary:  "ALTER COLUMN TYPE usually rewrites the table and its indexes under ACCESS EXCLUSIVE",
		Severity: evidence.Warning,
		DocsURL:  docAlterTable,
		check:    checkAlterColumnType,
	})
	register(Rule{
		ID:       "PS106",
		Name:     "set-not-null-scans-table",
		Summary:  "SET NOT NULL scans the whole table under ACCESS EXCLUSIVE",
		Severity: evidence.Warning,
		DocsURL:  docAlterTable,
		check:    checkSetNotNull,
	})
}

// alterCmds returns the ALTER TABLE subcommands of s and its target table
// when the table pre-exists the migration.
func alterCmds(c *Context, s *Statement) (string, []*pg_query.AlterTableCmd) {
	if s.Kind != KindAlterTable || s.node == nil {
		return "", nil
	}
	at := s.node.GetAlterTableStmt()
	if at == nil {
		return "", nil
	}
	table := relName(at.Relation)
	if c.CreatedEarlier(s, table) {
		return "", nil
	}
	var cmds []*pg_query.AlterTableCmd
	for _, n := range at.Cmds {
		if cmd := n.GetAlterTableCmd(); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return table, cmds
}

func checkAddColumnRewrite(c *Context, s *Statement) []Finding {
	table, cmds := alterCmds(c, s)
	var out []Finding
	for _, cmd := range cmds {
		if cmd.Subtype != pg_query.AlterTableType_AT_AddColumn {
			continue
		}
		col := cmd.Def.GetColumnDef()
		reason, certain := columnRewrite(col)
		if reason == "" {
			continue
		}
		f := at(s, fmt.Sprintf("Adding column %s to %s rewrites the table while holding ACCESS EXCLUSIVE: %s. Reads and writes on %s wait for the rewrite.", col.Colname, table, reason, table))
		f.Table, f.Lock = table, pglock.AccessExclusive
		if !certain {
			f.Confidence, f.Evidence = evidence.Low, evidence.Heuristic
		}
		f.Remediation = "Add the column without the default (or as a plain nullable column), backfill in batches, then set the default for new rows."
		out = append(out, f)
	}
	return out
}

func checkAddNotNullColumn(c *Context, s *Statement) []Finding {
	table, cmds := alterCmds(c, s)
	var out []Finding
	for _, cmd := range cmds {
		if cmd.Subtype != pg_query.AlterTableType_AT_AddColumn {
			continue
		}
		col := cmd.Def.GetColumnDef()
		if col == nil || (!col.IsNotNull && !hasConstraint(col, pg_query.ConstrType_CONSTR_NOTNULL)) {
			continue
		}
		if col.RawDefault != nil || col.Identity != "" || col.Generated != "" ||
			hasConstraint(col, pg_query.ConstrType_CONSTR_DEFAULT) ||
			hasConstraint(col, pg_query.ConstrType_CONSTR_IDENTITY) ||
			hasConstraint(col, pg_query.ConstrType_CONSTR_GENERATED) {
			continue
		}
		f := at(s, fmt.Sprintf("Column %s is added to %s as NOT NULL without a default; the statement fails with a not-null violation if %s has any rows.", col.Colname, table, table))
		f.Table, f.Lock = table, pglock.AccessExclusive
		f.Remediation = "Add the column as nullable, backfill it, then add a NOT VALID check constraint, validate it and set NOT NULL."
		out = append(out, f)
	}
	return out
}

// binaryCompatibleTargets are target types for which a conversion from a
// related type can skip the rewrite (for example varchar(n) to text).
var binaryCompatibleTargets = map[string]bool{"text": true, "varchar": true, "numeric": true, "cidr": true}

// limitTargets are types whose length or precision limit can be raised
// without a rewrite, e.g. varchar(50) to varchar(100).
var limitTargets = map[string]bool{"varchar": true, "numeric": true, "varbit": true}

func checkAlterColumnType(c *Context, s *Statement) []Finding {
	table, cmds := alterCmds(c, s)
	var out []Finding
	for _, cmd := range cmds {
		if cmd.Subtype != pg_query.AlterTableType_AT_AlterColumnType {
			continue
		}
		col := cmd.Def.GetColumnDef()
		target := typeName(col.GetTypeName())
		f := at(s, fmt.Sprintf("Changing the type of %s.%s to %s takes ACCESS EXCLUSIVE and usually rewrites the table and rebuilds its indexes; reads and writes wait for the whole rewrite.", table, cmd.Name, target))
		f.Table, f.Lock = table, pglock.AccessExclusive
		f.Confidence, f.Evidence = evidence.Medium, evidence.Heuristic
		using := col.GetRawDefault()
		typmods := len(col.GetTypeName().GetTypmods()) > 0
		plain := using == nil || using.GetColumnRef() != nil // no USING, or USING the column itself
		switch {
		case !plain:
			// The USING expression is computed for every row.
			f.Message += " The USING expression converts every row, so the rewrite is certain."
			f.Confidence = evidence.High
		case binaryCompatibleTargets[target] && !typmods && plain:
			f.Message += " Some conversions to " + target + " (such as varchar(n) to text) avoid the rewrite; the experiment shows which applies."
			f.Confidence = evidence.Low
		case limitTargets[target] && typmods && plain:
			f.Message += " Raising the length or precision limit of a " + target + " column avoids the rewrite, but lowering it (or changing a numeric scale) does not; the old limit is not visible here and the experiment shows which applies."
			f.Confidence = evidence.Low
		}
		f.Remediation = "Add a new column of the target type, backfill it in batches, switch the application, then drop the old column."
		out = append(out, f)
	}
	return out
}

func checkSetNotNull(c *Context, s *Statement) []Finding {
	table, cmds := alterCmds(c, s)
	var out []Finding
	for _, cmd := range cmds {
		if cmd.Subtype != pg_query.AlterTableType_AT_SetNotNull {
			continue
		}
		if c.notNullCheckValidated(s, table, cmd.Name) {
			// PostgreSQL uses the validated CHECK and skips the scan.
			continue
		}
		f := at(s, fmt.Sprintf("SET NOT NULL on %s.%s scans every row while holding ACCESS EXCLUSIVE, unless a validated CHECK (%s IS NOT NULL) constraint already exists.", table, cmd.Name, cmd.Name))
		f.Table, f.Lock = table, pglock.AccessExclusive
		f.Confidence = evidence.Medium
		f.Remediation = fmt.Sprintf("First ADD CONSTRAINT ... CHECK (%s IS NOT NULL) NOT VALID, then VALIDATE CONSTRAINT in a separate transaction; SET NOT NULL then skips the scan.", cmd.Name)
		out = append(out, f)
	}
	return out
}

// notNullCheckValidated reports whether an earlier transaction of the
// migration left table with a validated CHECK (column IS NOT NULL): either
// added without NOT VALID, or added NOT VALID and validated, in a group
// that committed before s runs.
func (c *Context) notNullCheckValidated(s *Statement, table, column string) bool {
	group := c.groupOf[s]
	checks := map[string]bool{} // constraint name -> validated
	for _, o := range c.Statements[:c.position[s]] {
		if c.groupOf[o] == group || o.Kind != KindAlterTable || o.node == nil || o.node.GetAlterTableStmt() == nil {
			continue
		}
		st := o.node.GetAlterTableStmt()
		if relName(st.Relation) != table {
			continue
		}
		for _, n := range st.Cmds {
			cmd := n.GetAlterTableCmd()
			if cmd == nil {
				continue
			}
			switch cmd.Subtype {
			case pg_query.AlterTableType_AT_AddConstraint:
				if con := cmd.Def.GetConstraint(); con != nil && isNotNullCheck(con, column) {
					checks[con.Conname] = !con.SkipValidation
				}
			case pg_query.AlterTableType_AT_ValidateConstraint:
				if _, ok := checks[cmd.Name]; ok {
					checks[cmd.Name] = true
				}
			}
		}
	}
	for _, validated := range checks {
		if validated {
			return true
		}
	}
	return false
}

// isNotNullCheck reports whether con is CHECK (column IS NOT NULL).
func isNotNullCheck(con *pg_query.Constraint, column string) bool {
	if con.Contype != pg_query.ConstrType_CONSTR_CHECK {
		return false
	}
	nt := con.RawExpr.GetNullTest()
	if nt == nil || nt.Nulltesttype != pg_query.NullTestType_IS_NOT_NULL {
		return false
	}
	ref := nt.Arg.GetColumnRef()
	return ref != nil && lastName(qualifiedName(ref.Fields)) == column
}
