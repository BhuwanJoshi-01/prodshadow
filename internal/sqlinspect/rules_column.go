package sqlinspect

import (
	"fmt"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/pglock"

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
		if col == nil || !(col.IsNotNull || hasConstraint(col, pg_query.ConstrType_CONSTR_NOTNULL)) {
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
		if binaryCompatibleTargets[target] && len(col.GetTypeName().GetTypmods()) == 0 && col.GetRawDefault() == nil {
			f.Message += " Some conversions to " + target + " (such as varchar(n) to text) avoid the rewrite; the experiment shows which applies."
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
		f := at(s, fmt.Sprintf("SET NOT NULL on %s.%s scans every row while holding ACCESS EXCLUSIVE, unless a validated CHECK (%s IS NOT NULL) constraint already exists.", table, cmd.Name, cmd.Name))
		f.Table, f.Lock = table, pglock.AccessExclusive
		f.Confidence = evidence.Medium
		f.Remediation = fmt.Sprintf("First ADD CONSTRAINT ... CHECK (%s IS NOT NULL) NOT VALID, then VALIDATE CONSTRAINT in a separate transaction; SET NOT NULL then skips the scan.", cmd.Name)
		out = append(out, f)
	}
	return out
}
