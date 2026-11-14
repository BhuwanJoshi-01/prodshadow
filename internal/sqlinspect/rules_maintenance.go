package sqlinspect

import (
	"fmt"
	"strings"

	"github.com/BhuwanJoshi-01/prodshadow/internal/evidence"
)

func init() {
	register(Rule{
		ID:          "PS158",
		Name:        "concurrent-refresh-unsupported",
		Summary:     "REFRESH MATERIALIZED VIEW CONCURRENTLY fails on a view without a plain unique index, or one that is not populated",
		Severity:    evidence.Critical,
		DocsURL:     "https://www.postgresql.org/docs/current/sql-refreshmaterializedview.html",
		NeedsSchema: true,
		check:       checkConcurrentRefresh,
	})
	register(Rule{
		ID:       "PS126",
		Name:     "database-wide-maintenance",
		Summary:  "VACUUM, ANALYZE, CLUSTER or REINDEX without a table name processes every table in the database or schema",
		Severity: evidence.Warning,
		DocsURL:  docVacuum,
		check:    checkWideMaintenance,
	})
}

// checkWideMaintenance flags maintenance commands that are not limited to
// named tables. Their run time grows with the whole database, not with the
// change the migration makes.
func checkWideMaintenance(_ *Context, s *Statement) []Finding {
	var scope string
	switch {
	case s.Kind == KindVacuum && s.node != nil && s.node.GetVacuumStmt() != nil && len(s.node.GetVacuumStmt().Rels) == 0:
		scope = "every table in the database"
	case s.Kind == KindCluster && len(s.Locks) == 0:
		scope = "every previously clustered table in the database"
	case s.Kind == KindReindex && strings.HasPrefix(s.Tag, "REINDEX DATABASE"):
		scope = "every index in the database"
	case s.Kind == KindReindex && strings.HasPrefix(s.Tag, "REINDEX SYSTEM"):
		scope = "every system catalog index"
	case s.Kind == KindReindex && strings.HasPrefix(s.Tag, "REINDEX SCHEMA"):
		scope = "every index in the schema"
	default:
		return nil
	}
	f := at(s, fmt.Sprintf("%s without a table name processes %s, so the migration's run time depends on the size of the whole database rather than on the change it makes.", s.Tag, scope))
	f.Evidence, f.Confidence = evidence.Static, evidence.High
	f.Remediation = "Name the tables the migration changed, or leave database-wide maintenance to autovacuum and scheduled jobs outside the deploy."
	switch s.Kind {
	case KindReindex:
		f.DocsURL = docReindex
	case KindCluster:
		f.DocsURL = docCluster
	}
	return []Finding{f}
}

// checkConcurrentRefresh reports REFRESH MATERIALIZED VIEW CONCURRENTLY
// that PostgreSQL will reject, as determined from the fixture schema and
// the migration so far (Catalog.Resolve).
func checkConcurrentRefresh(c *Context, s *Statement) []Finding {
	if s.refreshProblem == "" {
		return nil
	}
	view := firstTable(s)
	f := at(s, fmt.Sprintf("REFRESH MATERIALIZED VIEW CONCURRENTLY %s fails: %s.", view, s.refreshProblem))
	f.Table = view
	f.Remediation = "Create a unique index on one or more plain columns of the view, without a WHERE clause (CREATE UNIQUE INDEX CONCURRENTLY works on a view too), and populate the view once with a plain REFRESH before refreshing it concurrently."
	return []Finding{f}
}
