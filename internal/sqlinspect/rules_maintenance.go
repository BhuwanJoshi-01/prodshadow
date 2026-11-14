package sqlinspect

import (
	"fmt"
	"strings"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"
)

func init() {
	register(Rule{
		ID:       "PS126",
		Name:     "database-wide-maintenance",
		Summary:  "VACUUM, ANALYZE or REINDEX without a table name processes every table in the database or schema",
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
	if s.Kind == KindReindex {
		f.DocsURL = docReindex
	}
	return []Finding{f}
}
