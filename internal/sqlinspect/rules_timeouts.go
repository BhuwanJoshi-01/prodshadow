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
	// With the fixture schema the index's table is known (a lock "via
	// index ..."); otherwise the finding names the index.
	table, owner := idx, "the table that owns it"
	for _, l := range s.Locks {
		if l.Via == "index "+idx {
			table, owner = l.Table, l.Table
			break
		}
	}
	f := at(s, fmt.Sprintf("DROP INDEX %s takes ACCESS EXCLUSIVE on %s; the drop itself is quick, but it queues behind long transactions and blocks reads and writes while it waits.", idx, owner))
	f.Table, f.Lock = table, pglock.AccessExclusive
	f.Remediation = "Use DROP INDEX CONCURRENTLY, outside a transaction block."
	return []Finding{f}
}

func init() {
	register(Rule{
		ID:       "PS135",
		Name:     "statement-timeout-disabled",
		Summary:  "SET statement_timeout = 0 lets the following statements run, and hold their locks, without limit",
		Severity: evidence.Info,
		DocsURL:  "https://www.postgresql.org/docs/current/runtime-config-client.html#GUC-STATEMENT-TIMEOUT",
		check:    checkStatementTimeoutDisabled,
	})
}

// checkStatementTimeoutDisabled flags statement_timeout = 0. Disabling it
// is sometimes deliberate for a long backfill, so the rule only informs.
func checkStatementTimeoutDisabled(c *Context, s *Statement) []Finding {
	name, value, ok := settingValue(s)
	if !ok || name != "statement_timeout" {
		return nil
	}
	if d, ok := parseTimeout(value); !ok || d != 0 {
		return nil
	}
	f := at(s, "SET statement_timeout = 0 removes the time limit for every following statement of the session, including the deploy tool's own timeout settings. A statement that runs longer than expected keeps its locks until it finishes or someone cancels it.")
	f.Evidence, f.Confidence = evidence.Static, evidence.High
	f.Remediation = "Set a generous but finite limit for the statements that need it, e.g. SET LOCAL statement_timeout = '15min' inside their transaction."
	return []Finding{f}
}
