package sqlinspect

import (
	"fmt"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

func init() {
	register(Rule{
		ID:       "PS155",
		Name:     "not-rerunnable",
		Summary:  "A file whose statements commit one by one cannot be re-run after a partial failure",
		Severity: evidence.Info,
		DocsURL:  docCreateIndex,
		checkAll: checkNotRerunnable,
	})
}

// checkNotRerunnable reports files that are not executed in one
// transaction (they contain statements such as CREATE INDEX CONCURRENTLY,
// or the transaction mode is none) and create objects without IF NOT
// EXISTS before a later statement. If that later statement fails, the
// earlier ones stay committed, and running the file again fails on them.
func checkNotRerunnable(c *Context) []Finding {
	var out []Finding
	for _, file := range byFile(c.Statements) {
		first, last := file[0], file[len(file)-1]
		if len(file) < 2 || (c.groupOf[first] == c.groupOf[last] && c.Group(first).InTransaction()) {
			continue // a single statement, or one transaction for the whole file
		}
		for _, s := range file[:len(file)-1] {
			if c.Group(s).InTransaction() || !createsWithoutIfNotExists(s) {
				continue
			}
			f := at(s, fmt.Sprintf("%s commits on its own, before the rest of %s. If a later statement fails, running the file again fails here because the object already exists.", s.Tag, s.File))
			f.Evidence, f.Confidence = evidence.Static, evidence.High
			f.Remediation = "Add IF NOT EXISTS (and, for concurrent indexes, DROP INDEX CONCURRENTLY IF EXISTS before them in case a failed build left an invalid index), or put each non-transactional statement in its own migration file."
			out = append(out, f)
			break
		}
	}
	return out
}

// createsWithoutIfNotExists reports statements that create an index, table,
// sequence or column and fail if it already exists.
func createsWithoutIfNotExists(s *Statement) bool {
	n := s.node
	if n == nil {
		return false
	}
	switch {
	case n.GetIndexStmt() != nil:
		return !n.GetIndexStmt().IfNotExists
	case n.GetCreateStmt() != nil:
		return !n.GetCreateStmt().IfNotExists
	case n.GetCreateSeqStmt() != nil:
		return !n.GetCreateSeqStmt().IfNotExists
	case n.GetAlterTableStmt() != nil:
		for _, c := range n.GetAlterTableStmt().Cmds {
			if cmd := c.GetAlterTableCmd(); cmd != nil && cmd.Subtype == pg_query.AlterTableType_AT_AddColumn && !cmd.MissingOk {
				return true
			}
		}
	}
	return false
}
