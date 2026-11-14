package sqlinspect

import (
	"fmt"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"
)

func init() {
	register(Rule{
		ID:       "PS132",
		Name:     "unnamed-concurrent-index",
		Summary:  "CREATE INDEX CONCURRENTLY without a name piles up invalid indexes when it is retried",
		Severity: evidence.Warning,
		DocsURL:  docCreateIndex,
		check:    checkUnnamedConcurrentIndex,
	})
}

// checkUnnamedConcurrentIndex flags CREATE INDEX CONCURRENTLY without an
// index name. A failed concurrent build leaves an INVALID index behind; with
// a generated name, the retry creates a second index next to it instead of
// failing on the name.
func checkUnnamedConcurrentIndex(c *Context, s *Statement) []Finding {
	if s.Kind != KindCreateIndex || s.node == nil || s.node.GetIndexStmt() == nil {
		return nil
	}
	st := s.node.GetIndexStmt()
	if !st.Concurrent || st.Idxname != "" {
		return nil
	}
	table := relName(st.Relation)
	if c.CreatedEarlier(s, table) {
		return nil
	}
	f := at(s, fmt.Sprintf("This CREATE INDEX CONCURRENTLY on %s has no index name. If the build fails (a lock timeout, a deadlock, a duplicate value), PostgreSQL leaves an INVALID index with a generated name; running the migration again creates another one beside it, and every invalid index still slows down writes.", table))
	f.Table = table
	f.Evidence, f.Confidence = evidence.Static, evidence.High
	f.Remediation = "Name the index and write the migration so a retry can clean up: DROP INDEX CONCURRENTLY IF EXISTS <name>; CREATE INDEX CONCURRENTLY <name> ON ..."
	return []Finding{f}
}
