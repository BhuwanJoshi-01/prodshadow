package sqlinspect

import (
	"fmt"
	"time"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/pglock"
)

// LongLockTimeout is the lock_timeout above which PS120 warns: every query
// on the table can queue for that long behind a waiting migration.
const LongLockTimeout = 10 * time.Second

func init() {
	register(Rule{
		ID:       "PS120",
		Name:     "ineffective-lock-timeout",
		Summary:  "The migration disables lock_timeout or sets it so high that queries can queue for a long time",
		Severity: evidence.Warning,
		DocsURL:  docLockTimeout,
		check:    checkLockTimeoutValue,
	})
	register(Rule{
		ID:       "PS122",
		Name:     "drop-index-blocks-traffic",
		Summary:  "DROP INDEX without CONCURRENTLY takes ACCESS EXCLUSIVE on the table",
		Severity: evidence.Warning,
		DocsURL:  "https://www.postgresql.org/docs/current/sql-dropindex.html",
		check:    checkDropIndex,
	})
}

func checkLockTimeoutValue(c *Context, s *Statement) []Finding {
	name, value, ok := settingValue(s)
	if !ok || name != "lock_timeout" {
		return nil
	}
	d, ok := parseTimeout(value)
	if !ok {
		return nil
	}
	var msg string
	switch {
	case d == 0:
		msg = "SET lock_timeout = 0 disables the timeout: a blocked migration waits indefinitely, and every query on the table queues behind it."
	case d > LongLockTimeout:
		msg = fmt.Sprintf("lock_timeout is %s: while the migration waits for its lock, every later query on the table can queue for up to %s.", d, d)
	default:
		return nil
	}
	f := at(s, msg)
	f.Remediation = "Use a short lock_timeout (a few seconds at most) and retry the migration with backoff instead."
	return []Finding{f}
}

func checkDropIndex(c *Context, s *Statement) []Finding {
	if s.Kind != KindDropIndex || s.Txn == TxnNotInBlock {
		return nil
	}
	idx := firstTable(s)
	f := at(s, fmt.Sprintf("DROP INDEX %s takes ACCESS EXCLUSIVE on the table that owns it; the drop itself is quick, but it queues behind long transactions and blocks reads and writes while it waits.", idx))
	f.Table, f.Lock = idx, pglock.AccessExclusive
	f.Remediation = "Use DROP INDEX CONCURRENTLY, outside a transaction block."
	return []Finding{f}
}
