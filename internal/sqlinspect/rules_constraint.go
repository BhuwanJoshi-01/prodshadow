package sqlinspect

import (
	"fmt"
	"slices"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/pglock"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

func init() {
	register(Rule{
		ID:       "PS107",
		Name:     "constraint-validated-in-place",
		Summary:  "Adding a foreign key or CHECK constraint without NOT VALID validates every row while holding a lock",
		Severity: evidence.Warning,
		DocsURL:  docAlterTable,
		check:    checkValidatedConstraint,
	})
	register(Rule{
		ID:       "PS108",
		Name:     "unique-constraint-builds-index",
		Summary:  "Adding a UNIQUE, PRIMARY KEY or EXCLUDE constraint builds an index under ACCESS EXCLUSIVE",
		Severity: evidence.Warning,
		DocsURL:  docAlterTable,
		check:    checkConstraintIndexBuild,
	})
	register(Rule{
		ID:       "PS119",
		Name:     "foreign-key-locks-referenced-table",
		Summary:  "A new table's foreign key takes SHARE ROW EXCLUSIVE on the referenced table",
		Severity: evidence.Info,
		DocsURL:  docCreateTable,
		check:    checkCreateTableForeignKey,
	})
}

func addedConstraints(c *Context, s *Statement) (string, []*pg_query.Constraint) {
	table, cmds := alterCmds(c, s)
	var out []*pg_query.Constraint
	for _, cmd := range cmds {
		if cmd.Subtype == pg_query.AlterTableType_AT_AddConstraint {
			if con := cmd.Def.GetConstraint(); con != nil {
				out = append(out, con)
			}
		}
	}
	return table, out
}

func checkValidatedConstraint(c *Context, s *Statement) []Finding {
	table, cons := addedConstraints(c, s)
	var out []Finding
	// ADD COLUMN ... REFERENCES validates immediately and cannot be NOT VALID.
	_, cmds := alterCmds(c, s)
	for _, cmd := range cmds {
		if cmd.Subtype != pg_query.AlterTableType_AT_AddColumn {
			continue
		}
		col := cmd.Def.GetColumnDef()
		for _, n := range col.GetConstraints() {
			con := n.GetConstraint()
			if con == nil || con.Contype != pg_query.ConstrType_CONSTR_FOREIGN {
				continue
			}
			ref := relName(con.Pktable)
			f := at(s, fmt.Sprintf("Adding column %s with REFERENCES %s validates every existing row of %s while holding SHARE ROW EXCLUSIVE on %s; an inline foreign key cannot be added NOT VALID.", col.Colname, ref, table, ref))
			f.Table, f.Lock = table, pglock.AccessExclusive
			f.Remediation = "Add the column without REFERENCES, then ADD CONSTRAINT ... FOREIGN KEY ... NOT VALID and VALIDATE CONSTRAINT in a separate transaction."
			out = append(out, f)
		}
	}
	inline := map[*pg_query.Constraint]bool{}
	for _, con := range inlineColumnConstraints(c, s, pg_query.ConstrType_CONSTR_CHECK) {
		inline[con] = true
		cons = append(cons, con)
	}
	for _, con := range cons {
		if con.SkipValidation {
			continue
		}
		switch con.Contype {
		case pg_query.ConstrType_CONSTR_FOREIGN:
			ref := relName(con.Pktable)
			f := at(s, fmt.Sprintf("Adding foreign key %s on %s validates every existing row while holding SHARE ROW EXCLUSIVE on %s and %s; writes to both tables wait for the scan.", nameOr(con.Conname, "(unnamed)"), table, table, ref))
			f.Table, f.Lock = table, pglock.ShareRowExclusive
			f.Remediation = "Add the constraint with NOT VALID, then run ALTER TABLE ... VALIDATE CONSTRAINT in a separate transaction (it takes only SHARE UPDATE EXCLUSIVE)."
			out = append(out, f)
		case pg_query.ConstrType_CONSTR_CHECK:
			f := at(s, fmt.Sprintf("Adding CHECK constraint %s on %s scans every row while holding ACCESS EXCLUSIVE; reads and writes wait for the scan.", nameOr(con.Conname, "(unnamed)"), table))
			if inline[con] {
				f.Message = fmt.Sprintf("Adding a column with an inline CHECK constraint to %s checks every existing row while holding ACCESS EXCLUSIVE; an inline CHECK cannot be NOT VALID.", table)
			}
			f.Table, f.Lock = table, pglock.AccessExclusive
			f.Remediation = "Add the constraint with NOT VALID, then VALIDATE CONSTRAINT in a separate transaction."
			out = append(out, f)
		}
	}
	return out
}

func checkConstraintIndexBuild(c *Context, s *Statement) []Finding {
	table, cons := addedConstraints(c, s)
	// Inline UNIQUE and PRIMARY KEY on ADD COLUMN build an index too.
	cons = append(cons, inlineColumnConstraints(c, s, pg_query.ConstrType_CONSTR_UNIQUE, pg_query.ConstrType_CONSTR_PRIMARY)...)
	var out []Finding
	for _, con := range cons {
		var kind string
		switch con.Contype {
		case pg_query.ConstrType_CONSTR_UNIQUE:
			kind = "UNIQUE"
		case pg_query.ConstrType_CONSTR_PRIMARY:
			kind = "PRIMARY KEY"
		case pg_query.ConstrType_CONSTR_EXCLUSION:
			kind = "EXCLUDE"
		default:
			continue
		}
		if con.Indexname != "" {
			continue
		}
		f := at(s, fmt.Sprintf("Adding a %s constraint to %s builds its index while holding ACCESS EXCLUSIVE; reads and writes wait for the build.", kind, table))
		f.Table, f.Lock = table, pglock.AccessExclusive
		if con.Contype != pg_query.ConstrType_CONSTR_EXCLUSION {
			f.Remediation = fmt.Sprintf("Build the index first with CREATE UNIQUE INDEX CONCURRENTLY, then ADD CONSTRAINT ... %s USING INDEX.", kind)
		}
		out = append(out, f)
	}
	return out
}

func checkCreateTableForeignKey(c *Context, s *Statement) []Finding {
	if s.Kind != KindCreateTable || s.node == nil {
		return nil
	}
	ct := s.node.GetCreateStmt()
	if ct == nil {
		return nil
	}
	var out []Finding
	for _, con := range createTableConstraints(ct) {
		if con.Contype != pg_query.ConstrType_CONSTR_FOREIGN {
			continue
		}
		ref := relName(con.Pktable)
		if ref == "" || c.CreatedEarlier(s, ref) {
			continue
		}
		f := at(s, fmt.Sprintf("Creating %s with a foreign key to %s briefly takes SHARE ROW EXCLUSIVE on %s, which waits for and then blocks writes to it.", relName(ct.Relation), ref, ref))
		f.Table, f.Lock = ref, pglock.ShareRowExclusive
		out = append(out, f)
	}
	return out
}

func nameOr(name, fallback string) string {
	if name == "" {
		return fallback
	}
	return name
}

// inlineColumnConstraints returns constraints of the given types declared
// inline on columns that ALTER TABLE ... ADD COLUMN adds to an existing
// table.
func inlineColumnConstraints(c *Context, s *Statement, types ...pg_query.ConstrType) []*pg_query.Constraint {
	_, cmds := alterCmds(c, s)
	var out []*pg_query.Constraint
	for _, cmd := range cmds {
		if cmd.Subtype != pg_query.AlterTableType_AT_AddColumn {
			continue
		}
		for _, n := range cmd.Def.GetColumnDef().GetConstraints() {
			if con := n.GetConstraint(); con != nil && slices.Contains(types, con.Contype) {
				out = append(out, con)
			}
		}
	}
	return out
}
