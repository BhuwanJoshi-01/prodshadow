package sqlinspect

import (
	"fmt"
	"strings"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

func init() {
	register(Rule{
		ID:       "PS142",
		Name:     "autovacuum-disabled",
		Summary:  "Turning autovacuum off for a table lets dead rows and transaction ID age accumulate",
		Severity: evidence.Warning,
		DocsURL:  "https://www.postgresql.org/docs/current/routine-vacuuming.html",
		check:    checkAutovacuumDisabled,
	})
}

// checkAutovacuumDisabled flags ALTER TABLE ... SET (autovacuum_enabled =
// false) and the TOAST variant, including on new tables.
func checkAutovacuumDisabled(c *Context, s *Statement) []Finding {
	if s.node == nil {
		return nil
	}
	var table string
	var options []*pg_query.Node
	switch {
	case s.node.GetAlterTableStmt() != nil:
		st := s.node.GetAlterTableStmt()
		table = relName(st.Relation)
		for _, n := range st.Cmds {
			if cmd := n.GetAlterTableCmd(); cmd != nil && cmd.Subtype == pg_query.AlterTableType_AT_SetRelOptions {
				options = append(options, cmd.Def.GetList().GetItems()...)
			}
		}
	case s.node.GetCreateStmt() != nil:
		st := s.node.GetCreateStmt()
		table, options = relName(st.Relation), st.Options
	default:
		return nil
	}
	var out []Finding
	for _, o := range options {
		d := o.GetDefElem()
		if d == nil || !strings.EqualFold(d.Defname, "autovacuum_enabled") || defElemTrue(d) {
			continue
		}
		name := d.Defname
		if d.Defnamespace != "" {
			name = d.Defnamespace + "." + name
		}
		f := at(s, fmt.Sprintf("%s = false stops routine autovacuum on %s: dead rows from updates and deletes are not cleaned up, statistics go stale, and only the anti-wraparound vacuum still runs, eventually as an emergency on a large table.", name, table))
		f.Table = table
		f.Evidence, f.Confidence = evidence.Static, evidence.High
		f.Remediation = "Tune autovacuum for the table instead (for example autovacuum_vacuum_scale_factor), or re-enable it right after the bulk operation it was disabled for."
		out = append(out, f)
	}
	return out
}

func init() {
	register(Rule{
		ID:       "PS143",
		Name:     "unlogged-table",
		Summary:  "Unlogged tables are emptied after a crash and are not replicated to standbys",
		Severity: evidence.Warning,
		DocsURL:  "https://www.postgresql.org/docs/current/sql-createtable.html#SQL-CREATETABLE-UNLOGGED",
		check:    checkUnloggedTable,
	})
}

// checkUnloggedTable flags CREATE UNLOGGED TABLE and ALTER TABLE ... SET
// UNLOGGED. Temporary tables are not reported.
func checkUnloggedTable(c *Context, s *Statement) []Finding {
	if s.node == nil {
		return nil
	}
	var table, how string
	switch {
	case s.node.GetCreateStmt() != nil && s.node.GetCreateStmt().GetRelation().GetRelpersistence() == "u":
		table, how = relName(s.node.GetCreateStmt().Relation), "is created UNLOGGED"
	case s.node.GetCreateTableAsStmt() != nil && s.node.GetCreateTableAsStmt().GetInto().GetRel().GetRelpersistence() == "u":
		table, how = relName(s.node.GetCreateTableAsStmt().GetInto().GetRel()), "is created UNLOGGED"
	case s.Kind == KindAlterTable:
		for _, sub := range s.Subcommands {
			if sub == "set_unlogged" {
				table, how = firstTable(s), "is switched to UNLOGGED"
			}
		}
	}
	if table == "" {
		return nil
	}
	f := at(s, fmt.Sprintf("%s %s: its writes skip the write-ahead log, so after a crash or failover PostgreSQL empties it, and physical standbys and backups based on WAL never contain its rows.", table, how))
	f.Table = table
	f.Evidence, f.Confidence = evidence.Static, evidence.High
	f.Remediation = "Keep tables that hold data you need logged; use UNLOGGED only for scratch data that can be rebuilt, and drop it after use."
	return []Finding{f}
}
