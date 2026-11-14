package sqlinspect

import (
	"fmt"
	"slices"
	"strings"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

func init() {
	register(Rule{
		ID:       "PS160",
		Name:     "view-replace-incompatible",
		Summary:  "CREATE OR REPLACE VIEW fails when it drops, renames or reorders the view's columns",
		Severity: evidence.Critical,
		DocsURL:  "https://www.postgresql.org/docs/current/sql-createview.html",
		check:    checkViewReplace,
	})
	register(Rule{
		ID:       "PS159",
		Name:     "dependent-view",
		Summary:  "Dropping or retyping a column, or dropping a table, fails while a view depends on it",
		Severity: evidence.Critical,
		DocsURL:  "https://www.postgresql.org/docs/current/ddl-depend.html",
		check:    checkDependentView,
	})
	register(Rule{
		ID:       "PS157",
		Name:     "referenced-table-removed",
		Summary:  "DROP TABLE or TRUNCATE without CASCADE fails while other tables' foreign keys reference the table",
		Severity: evidence.Critical,
		DocsURL:  "https://www.postgresql.org/docs/current/ddl-depend.html",
		check:    checkReferencedTableRemoved,
	})
	register(Rule{
		ID:       "PS124",
		Name:     "cascade-drop",
		Summary:  "CASCADE silently drops or truncates dependent objects the migration does not name",
		Severity: evidence.Warning,
		DocsURL:  "https://www.postgresql.org/docs/current/ddl-depend.html",
		check:    checkCascade,
	})
}

// checkCascade flags DROP ... CASCADE, TRUNCATE ... CASCADE and ALTER TABLE
// DROP COLUMN/CONSTRAINT ... CASCADE. The objects removed by the cascade
// depend on the schema in production, which the statement does not show.
func checkCascade(c *Context, s *Statement) []Finding {
	if s.node == nil {
		return nil
	}
	var targets []string
	what := "also drops every object that depends on it (views, foreign keys, column defaults, functions)"
	switch {
	case s.node.GetDropStmt() != nil:
		st := s.node.GetDropStmt()
		if st.Behavior != pg_query.DropBehavior_DROP_CASCADE {
			return nil
		}
		kind := strings.ToLower(strings.ReplaceAll(strings.TrimPrefix(st.RemoveType.String(), "OBJECT_"), "_", " "))
		for _, o := range st.Objects {
			n := dropTargetName(o)
			if n != "" && c.CreatedEarlier(s, n) {
				continue
			}
			targets = append(targets, strings.TrimSpace(kind+" "+n))
		}
	case s.node.GetTruncateStmt() != nil:
		st := s.node.GetTruncateStmt()
		if st.Behavior != pg_query.DropBehavior_DROP_CASCADE {
			return nil
		}
		what = "also truncates every table with a foreign key referencing it, and the tables referencing those"
		for _, r := range st.Relations {
			if n := relName(r.GetRangeVar()); n != "" && !c.CreatedEarlier(s, n) {
				targets = append(targets, "table "+n)
			}
		}
	case s.Kind == KindAlterTable:
		table, cmds := alterCmds(c, s)
		for _, cmd := range cmds {
			if cmd.Behavior != pg_query.DropBehavior_DROP_CASCADE {
				continue
			}
			switch cmd.Subtype {
			case pg_query.AlterTableType_AT_DropColumn:
				targets = append(targets, fmt.Sprintf("column %s.%s", table, cmd.Name))
			case pg_query.AlterTableType_AT_DropConstraint:
				targets = append(targets, fmt.Sprintf("constraint %s on %s", cmd.Name, table))
			}
		}
	}
	var out []Finding
	for _, t := range targets {
		f := at(s, fmt.Sprintf("CASCADE on %s %s. Which objects those are depends on the production schema, not on this migration, so they are removed without being named or reviewed.", t, what))
		f.Evidence, f.Confidence = evidence.Static, evidence.High
		f.Remediation = "Drop or alter the dependent objects explicitly first, then use RESTRICT (the default) so an unexpected dependency fails the migration instead of being removed."
		out = append(out, f)
	}
	return out
}

// dropTargetName names one DropStmt object: a qualified name list, a bare
// name (schemas, extensions), a type, or a function with arguments.
func dropTargetName(o *pg_query.Node) string {
	switch {
	case o.GetList() != nil:
		return strings.Join(stringItems(o.GetList().Items), ".")
	case o.GetString_() != nil:
		return o.GetString_().Sval
	case o.GetTypeName() != nil:
		return typeName(o.GetTypeName())
	case o.GetObjectWithArgs() != nil:
		return qualifiedName(o.GetObjectWithArgs().Objname)
	}
	return ""
}

// checkReferencedTableRemoved reports DROP TABLE and TRUNCATE statements
// that PostgreSQL will reject because foreign keys of other tables
// reference the table. It needs the fixture schema (Catalog.Resolve).
func checkReferencedTableRemoved(c *Context, s *Statement) []Finding {
	if len(s.referencedBy) == 0 {
		return nil
	}
	verb := "DROP TABLE"
	if s.Kind == KindTruncate {
		verb = "TRUNCATE"
	}
	refers := "references"
	if len(s.referencedBy) > 1 {
		refers = "reference"
	}
	f := at(s, fmt.Sprintf("%s fails: foreign key %s still %s the table. PostgreSQL refuses to %s a table other tables' foreign keys point at.",
		verb, strings.Join(s.referencedBy, ", "), refers, strings.ToLower(verb)))
	f.Table = firstTable(s)
	f.Remediation = "Drop or change the referencing foreign keys first, in this or an earlier migration. CASCADE removes them silently (see PS124), which is rarely what the referencing code expects."
	return []Finding{f}
}

// checkDependentView reports DROP COLUMN, ALTER COLUMN TYPE and DROP TABLE
// statements that views known from the fixture schema make fail.
func checkDependentView(c *Context, s *Statement) []Finding {
	if len(s.dependentViews) == 0 {
		return nil
	}
	table := firstTable(s)
	var views []string
	for _, d := range s.dependentViews {
		if !slices.Contains(views, d.view) {
			views = append(views, d.view)
		}
	}
	list := strings.Join(views, ", ")
	d := s.dependentViews[0]
	var msg string
	switch d.change {
	case "drop":
		msg = fmt.Sprintf("Dropping column %s of %s fails: view %s reads it. PostgreSQL refuses to drop a column a view depends on (SQLSTATE 2BP01).", d.column, table, list)
	case "type":
		msg = fmt.Sprintf("Changing the type of column %s of %s fails: view %s reads it. PostgreSQL cannot alter the type of a column a view or rule uses (SQLSTATE 0A000).", d.column, table, list)
	default:
		msg = fmt.Sprintf("DROP TABLE %s fails: view %s depends on it (SQLSTATE 2BP01).", table, list)
	}
	f := at(s, msg)
	f.Table = table
	f.Remediation = "Drop the view first and recreate it after the change, in the same transaction; CASCADE drops it silently (see PS124), and the application then fails on the missing view."
	return []Finding{f}
}

// checkViewReplace reports CREATE OR REPLACE VIEW statements whose new
// column list does not extend the existing one.
func checkViewReplace(c *Context, s *Statement) []Finding {
	if s.viewReplaceProblem == "" {
		return nil
	}
	view := relName(s.node.GetViewStmt().View)
	f := at(s, fmt.Sprintf("CREATE OR REPLACE VIEW %s fails: %s. A replacement must keep the existing columns with the same names, types and order, and may only add columns at the end (SQLSTATE 42P16).", view, s.viewReplaceProblem))
	f.Table = view
	f.Confidence = evidence.Medium
	f.Remediation = "DROP VIEW and CREATE VIEW in the same transaction instead, after checking that nothing else depends on the view."
	return []Finding{f}
}
