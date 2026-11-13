package sqlinspect

import (
	"fmt"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/pglock"
)

func init() {
	register(Rule{
		ID:       "PS101",
		Name:     "create-index-blocks-writes",
		Summary:  "CREATE INDEX without CONCURRENTLY blocks writes for the whole build",
		Severity: evidence.Warning,
		DocsURL:  docCreateIndex,
		check:    checkBlockingIndex,
	})
	register(Rule{
		ID:       "PS102",
		Name:     "concurrent-operation-in-transaction",
		Summary:  "Statements that cannot run in a transaction block are inside one",
		Severity: evidence.Critical,
		DocsURL:  docCreateIndex,
		check:    checkNoBlockInTransaction,
	})
	register(Rule{
		ID:       "PS121",
		Name:     "concurrent-index-caveats",
		Summary:  "CREATE INDEX CONCURRENTLY waits for old transactions and can leave an invalid index",
		Severity: evidence.Info,
		DocsURL:  docCreateIndex,
		check:    checkConcurrentIndexCaveats,
	})
}

func checkBlockingIndex(c *Context, s *Statement) []Finding {
	if s.Kind != KindCreateIndex || s.Txn == TxnNotInBlock {
		return nil
	}
	table := firstTable(s)
	if table == "" || c.CreatedEarlier(s, table) {
		return nil
	}
	f := at(s, fmt.Sprintf("%s on %s takes a SHARE lock: inserts, updates and deletes on %s wait until the index build finishes.", s.Tag, table, table))
	f.Table, f.Lock = table, pglock.Share
	f.Remediation = "Use CREATE INDEX CONCURRENTLY in its own migration step, outside a transaction block."
	return []Finding{f}
}

func checkNoBlockInTransaction(c *Context, s *Statement) []Finding {
	if s.Txn != TxnNotInBlock || !c.Group(s).InTransaction() {
		return nil
	}
	how := "the executor wraps this file in a transaction (migration.transaction: per-file)"
	if c.Group(s).Explicit {
		how = "it follows an explicit BEGIN"
	}
	f := at(s, fmt.Sprintf("%s cannot run inside a transaction block, but %s; PostgreSQL will reject it.", s.Tag, how))
	f.Table = firstTable(s)
	f.Remediation = "Move the statement to its own file or outside BEGIN/COMMIT, and use migration.transaction: auto."
	return []Finding{f}
}

func checkConcurrentIndexCaveats(c *Context, s *Statement) []Finding {
	if s.Kind != KindCreateIndex || s.Txn != TxnNotInBlock {
		return nil
	}
	table := firstTable(s)
	if table == "" || c.CreatedEarlier(s, table) {
		return nil
	}
	f := at(s, fmt.Sprintf("%s on %s does not block writes, but it scans the table twice, waits for every transaction that could use the index, and leaves an INVALID index behind if it fails.", s.Tag, table))
	f.Table, f.Lock = table, pglock.ShareUpdateExclusive
	f.Remediation = "Check for an INVALID index after a failure and drop it (DROP INDEX CONCURRENTLY) before retrying."
	return []Finding{f}
}

// firstTable returns the first locked relation of s, or "".
func firstTable(s *Statement) string {
	if len(s.Locks) == 0 {
		return ""
	}
	return s.Locks[0].Table
}
