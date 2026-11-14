package sqlinspect

import (
	"fmt"
	"strings"

	"github.com/BhuwanJoshi-01/prodshadow/internal/evidence"
	"github.com/BhuwanJoshi-01/prodshadow/internal/pglock"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

func init() {
	register(Rule{
		ID:       "PS109",
		Name:     "destructive-change",
		Summary:  "DROP TABLE, DROP COLUMN, TRUNCATE and dropping views, functions, types, sequences or schemas need explicit review",
		Severity: evidence.Warning,
		DocsURL:  docDropTable,
		check:    checkDestructive,
	})
	register(Rule{
		ID:       "PS110",
		Name:     "rename-breaks-running-code",
		Summary:  "Renaming a table or column breaks application instances that still use the old name",
		Severity: evidence.Warning,
		DocsURL:  docAlterTable,
		check:    checkRename,
	})
}

func checkDestructive(c *Context, s *Statement) []Finding {
	var out []Finding
	switch s.Kind {
	case KindDropTable, KindTruncate:
		verb := "Dropping"
		if s.Kind == KindTruncate {
			verb = "Truncating"
		}
		for _, l := range s.Locks {
			if c.CreatedEarlier(s, l.Table) {
				continue
			}
			what := l.Table
			if l.Via != "" {
				// Locks resolved from the schema are on tables the
				// statement does not name. Only TRUNCATE ... CASCADE
				// removes their data (the referencing tables).
				if s.Kind != KindTruncate || !strings.HasPrefix(l.Via, "foreign key ") {
					continue
				}
				what = l.Table + " (through CASCADE, " + l.Via + ")"
			}
			f := at(s, fmt.Sprintf("%s %s permanently removes its data and takes ACCESS EXCLUSIVE; application versions that still query it will fail.", verb, what))
			f.Table, f.Lock = l.Table, pglock.AccessExclusive
			f.Remediation = "Confirm no deployed version reads the table, keep a backup or rename-and-wait period, and drop it in a later release."
			out = append(out, f)
		}
	case KindDropOther:
		st := s.node.GetDropStmt()
		if st == nil {
			break
		}
		kind, ok := droppedObjectKinds[st.RemoveType]
		if !ok {
			break
		}
		for _, o := range st.Objects {
			name := dropTargetName(o)
			if c.CreatedEarlier(s, name) {
				continue
			}
			f := at(s, fmt.Sprintf("Dropping %s %s removes it for every application version; code that still uses it fails, and its definition is lost.", kind, name))
			f.Remediation = "Confirm no deployed version or scheduled job uses it, keep its definition in version control, and drop it in a release after the last user is gone."
			out = append(out, f)
		}
	case KindAlterTable:
		table, cmds := alterCmds(c, s)
		for _, cmd := range cmds {
			if cmd.Subtype != pg_query.AlterTableType_AT_DropColumn {
				continue
			}
			f := at(s, fmt.Sprintf("Dropping column %s.%s removes its data; application versions that still select or insert it will fail during a rolling deploy.", table, cmd.Name))
			f.Table, f.Lock = table, pglock.AccessExclusive
			f.Remediation = "Stop reading and writing the column in a release that is fully deployed first, then drop it."
			out = append(out, f)
		}
	}
	return out
}

// droppedObjectKinds are the non-table objects whose removal PS109 reports.
// Indexes are left to PS122: dropping one loses no data.
var droppedObjectKinds = map[pg_query.ObjectType]string{
	pg_query.ObjectType_OBJECT_VIEW:      "view",
	pg_query.ObjectType_OBJECT_MATVIEW:   "materialized view",
	pg_query.ObjectType_OBJECT_SEQUENCE:  "sequence",
	pg_query.ObjectType_OBJECT_FUNCTION:  "function",
	pg_query.ObjectType_OBJECT_PROCEDURE: "procedure",
	pg_query.ObjectType_OBJECT_TYPE:      "type",
	pg_query.ObjectType_OBJECT_DOMAIN:    "domain",
	pg_query.ObjectType_OBJECT_SCHEMA:    "schema",
	// Dropping an extension drops its functions, types and operators.
	pg_query.ObjectType_OBJECT_EXTENSION: "extension",
}

// renamedObjectKinds are the renames, besides tables and columns, that
// break code referring to the old name.
var renamedObjectKinds = map[string]string{
	"rename_view": "view", "rename_matview": "materialized view", "rename_function": "function",
	"rename_procedure": "procedure", "rename_type": "type", "rename_domain": "domain", "rename_schema": "schema",
}

func checkRename(c *Context, s *Statement) []Finding {
	if s.node != nil && s.node.GetAlterEnumStmt() != nil && s.node.GetAlterEnumStmt().OldVal != "" {
		st := s.node.GetAlterEnumStmt()
		f := at(s, fmt.Sprintf("Renaming value '%s' of enum %s to '%s' breaks every running application instance that still reads or writes '%s'.", st.OldVal, qualifiedName(st.TypeName), st.NewVal, st.OldVal))
		f.Remediation = "Add the new value, migrate code and data to it, and stop using the old value; enum values cannot be dropped, so keep the old one unused."
		return []Finding{f}
	}
	if s.Kind != KindRename || len(s.Subcommands) == 0 {
		return nil
	}
	sub := s.Subcommands[0]
	if kind, ok := renamedObjectKinds[sub]; ok {
		return renameObject(c, s, kind)
	}
	if sub != "rename_column" && sub != "rename_table" {
		return nil
	}
	table := firstTable(s)
	if c.CreatedEarlier(s, table) {
		return nil
	}
	what := strings.TrimPrefix(sub, "rename_")
	f := at(s, fmt.Sprintf("Renaming a %s on %s is instant but breaks every running application instance that still uses the old name until all instances are replaced.", what, table))
	f.Table, f.Lock = table, pglock.AccessExclusive
	f.Remediation = "Use expand and contract: add the new name, write to both, migrate readers, then remove the old name in a later release."
	return []Finding{f}
}

// renameObject reports the rename of a view, function, type or schema.
func renameObject(c *Context, s *Statement, kind string) []Finding {
	st := s.node.GetRenameStmt()
	if st == nil {
		return nil
	}
	name := relName(st.Relation)
	switch {
	case name != "":
	case st.Object != nil:
		name = dropTargetName(st.Object)
	default:
		name = st.Subname
	}
	if name != "" && c.CreatedEarlier(s, name) {
		return nil
	}
	f := at(s, fmt.Sprintf("Renaming %s %s to %s breaks every running application instance, job or report that still uses the old name until all of them are updated.", kind, name, st.Newname))
	f.Remediation = "Create the object under the new name next to the old one (for a view, a second view; for a function, a wrapper), move callers over, then drop the old name in a later release."
	return []Finding{f}
}
