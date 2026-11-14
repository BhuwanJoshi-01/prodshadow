package sqlinspect

import (
	"fmt"
	"slices"
	"strings"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/pglock"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

func init() {
	register(Rule{
		ID:       "PS111",
		Name:     "unbounded-dml",
		Summary:  "UPDATE or DELETE without WHERE touches every row in one transaction",
		Severity: evidence.Warning,
		DocsURL:  docUpdate,
		check:    checkUnboundedDML,
	})
	register(Rule{
		ID:       "PS112",
		Name:     "explicit-table-lock",
		Summary:  "LOCK TABLE in a mode that blocks application traffic",
		Severity: evidence.Warning,
		DocsURL:  docLock,
		check:    checkExplicitLock,
	})
	register(Rule{
		ID:       "PS113",
		Name:     "rewrite-under-exclusive-lock",
		Summary:  "VACUUM FULL, CLUSTER, REINDEX, REFRESH MATERIALIZED VIEW and storage changes rewrite data under a blocking lock",
		Severity: evidence.Warning,
		DocsURL:  docExplicitLocking,
		check:    checkRewriteMaintenance,
	})
	register(Rule{
		ID:       "PS114",
		Name:     "missing-lock-timeout",
		Summary:  "The migration takes a blocking lock without setting lock_timeout",
		Severity: evidence.Info,
		DocsURL:  docLockTimeout,
		checkAll: checkMissingLockTimeout,
	})
	register(Rule{
		ID:       "PS115",
		Name:     "lock-held-across-statements",
		Summary:  "A blocking lock is held until COMMIT while later statements in the same transaction run",
		Severity: evidence.Warning,
		DocsURL:  docTransactions,
		check:    checkLockHeldAcrossStatements,
	})
	register(Rule{
		ID:       "PS118",
		Name:     "trigger-change-blocks-writes",
		Summary:  "Creating, enabling or disabling a trigger takes SHARE ROW EXCLUSIVE",
		Severity: evidence.Info,
		DocsURL:  docCreateTrigger,
		check:    checkTriggerChange,
	})
}

func checkUnboundedDML(c *Context, s *Statement) []Finding {
	if s.node == nil {
		return nil
	}
	var verb string
	switch {
	case s.Kind == KindUpdate && s.node.GetUpdateStmt().GetWhereClause() == nil:
		verb = "UPDATE"
	case s.Kind == KindDelete && s.node.GetDeleteStmt().GetWhereClause() == nil:
		verb = "DELETE"
	case s.Kind == KindSelect && unboundedLockingSelect(s.node.GetSelectStmt()):
		table := firstTable(s)
		if c.CreatedEarlier(s, table) {
			return nil
		}
		f := at(s, fmt.Sprintf("%s without WHERE or LIMIT locks every row of %s until the transaction ends; concurrent updates and deletes of any row wait.", s.Tag, table))
		f.Table, f.Lock = table, pglock.RowShare
		f.Remediation = "Lock only the rows the migration changes, in bounded batches, or use LOCK TABLE deliberately with a lock_timeout if the whole table must be frozen."
		return []Finding{f}
	default:
		return nil
	}
	table := firstTable(s)
	if c.CreatedEarlier(s, table) {
		return nil
	}
	f := at(s, fmt.Sprintf("%s without WHERE locks every row of %s in a single transaction; concurrent writers to any row wait until it commits, and the change produces WAL and dead tuples for the whole table.", verb, table))
	f.Table, f.Lock = table, pglock.RowExclusive
	f.Remediation = "Backfill in bounded batches (for example by primary-key range), committing each batch, outside the schema-change transaction."
	return []Finding{f}
}

func checkExplicitLock(c *Context, s *Statement) []Finding {
	if s.Kind != KindLockTable {
		return nil
	}
	var out []Finding
	for _, l := range s.Locks {
		if l.Via != "" || (l.Mode.Impact() != pglock.ImpactWrites && l.Mode.Impact() != pglock.ImpactAllTraffic) {
			continue // partitions and view tables follow the named table
		}
		f := at(s, fmt.Sprintf("LOCK TABLE %s IN %s MODE %s until the transaction ends.", l.Table, l.Mode, impactPhrase(l.Mode)))
		f.Table, f.Lock = l.Table, l.Mode
		f.Remediation = "Keep the transaction holding the lock as short as possible and set lock_timeout before acquiring it."
		out = append(out, f)
	}
	return out
}

func checkRewriteMaintenance(c *Context, s *Statement) []Finding {
	var what string
	switch {
	case s.Kind == KindVacuum && s.Tag == "VACUUM FULL":
		what = "VACUUM FULL rewrites the table"
	case s.Kind == KindCluster:
		what = "CLUSTER rewrites the table in index order"
	case s.Kind == KindRefreshMatView && !strings.HasSuffix(s.Tag, "CONCURRENTLY"):
		what = "REFRESH MATERIALIZED VIEW recomputes the view"
	case s.Kind == KindReindex && !strings.HasSuffix(s.Tag, "CONCURRENTLY"):
		what = "REINDEX rebuilds the index"
	case s.Kind == KindAlterTable:
		for _, sub := range s.Subcommands {
			switch sub {
			case "set_tablespace", "set_logged", "set_unlogged", "set_access_method":
				what = "ALTER TABLE " + strings.ToUpper(strings.ReplaceAll(sub, "_", " ")) + " rewrites the table"
			}
		}
	}
	if what == "" {
		return nil
	}
	table := firstTable(s)
	if table != "" && c.CreatedEarlier(s, table) {
		return nil
	}
	mode := s.StrongestLock()
	msg := fmt.Sprintf("%s while holding %s, which %s for the full duration.", what, mode, impactPhrase(mode))
	switch {
	case s.Kind == KindReindex && mode == pglock.None && !strings.HasSuffix(s.Tag, "CONCURRENTLY"):
		// REINDEX SCHEMA, DATABASE and SYSTEM name no table.
		mode = pglock.Share
		msg = fmt.Sprintf("%s rebuilds every index it covers one table at a time, holding SHARE on each table (which %s) and ACCESS EXCLUSIVE on each index while it is rebuilt.", s.Tag, impactPhrase(mode))
	case s.Kind == KindCluster && mode == pglock.None:
		mode = pglock.AccessExclusive
		msg = fmt.Sprintf("CLUSTER without a table rewrites every previously clustered table of the database one at a time, holding ACCESS EXCLUSIVE on each, which %s while that table is rewritten.", impactPhrase(mode))
	}
	f := at(s, msg)
	f.Table, f.Lock = table, mode
	f.Remediation = "Prefer the CONCURRENTLY variant where one exists, or schedule the operation separately from application deploys."
	switch s.Kind {
	case KindReindex:
		f.DocsURL = docReindex
	case KindCluster:
		f.DocsURL = docCluster
	case KindVacuum:
		f.DocsURL = docVacuum
	case KindRefreshMatView:
		f.DocsURL = docRefreshMatView
	}
	return []Finding{f}
}

func checkMissingLockTimeout(c *Context) []Finding {
	// A session-level SET lasts until changed; SET LOCAL only until the
	// end of its transaction group.
	session, local, group := false, false, -1
	for _, s := range c.Statements {
		if g := c.groupOf[s]; g != group {
			group, local = g, false
		}
		if on, isLocal, ok := lockTimeoutChange(s); ok {
			if isLocal {
				local = on
			} else {
				session, local = on, on && local
			}
		}
		if session || local {
			continue
		}
		// Every blocking lock of the statement, including the ones it takes
		// on tables it does not name (resolved from the schema).
		var blocking []TableLock
		resolvedIndex := slices.ContainsFunc(s.Locks, func(l TableLock) bool { return strings.HasPrefix(l.Via, "index ") })
		for _, l := range s.Locks {
			impact := l.Mode.Impact()
			if (impact != pglock.ImpactAllTraffic && impact != pglock.ImpactWrites) || c.CreatedEarlier(s, l.Table) || l.CreatedBy != "" {
				continue
			}
			if resolvedIndex && s.OnIndex(l) {
				continue // the index's table is listed instead
			}
			blocking = append(blocking, l)
		}
		if len(blocking) == 0 {
			continue
		}
		parts, tables := make([]string, 0, len(blocking)), make([]string, 0, len(blocking))
		for _, l := range blocking {
			part := fmt.Sprintf("%s on %s", l.Mode, l.Table)
			if l.Via != "" {
				part += " (via " + l.Via + ")"
			}
			parts = append(parts, part)
			tables = append(tables, l.Table)
		}
		f := at(s, fmt.Sprintf("%s requests %s without a lock_timeout. If a long transaction holds a conflicting lock, the request queues indefinitely and every later query on %s queues behind it. ProdShadow applies migration.lock_timeout_ms during rehearsal; production runs need the same protection.", s.Tag, strings.Join(parts, " and "), strings.Join(tables, " and ")))
		f.Table, f.Lock = blocking[0].Table, blocking[0].Mode
		f.Remediation = "Start the migration with SET lock_timeout = '2s' (or configure it in the migration runner) and retry on lock_not_available."
		return []Finding{f}
	}
	return nil
}

func checkLockHeldAcrossStatements(c *Context, s *Statement) []Finding {
	g := c.Group(s)
	if !g.InTransaction() {
		return nil
	}
	var later []*Statement
	seen := false
	for _, o := range g.Statements {
		if o == s {
			seen = true
			continue
		}
		if seen && o.Kind != KindTransaction && o.Kind != KindSet {
			later = append(later, o)
		}
	}
	if len(later) == 0 {
		return nil
	}
	var out []Finding
	for _, l := range s.Locks {
		impact := l.Mode.Impact()
		if (impact != pglock.ImpactAllTraffic && impact != pglock.ImpactWrites) || c.CreatedEarlier(s, l.Table) {
			continue
		}
		f := at(s, fmt.Sprintf("%s on %s is held until COMMIT while %d later statement(s) in the same transaction run (next: %s at %s:%d); %s for that whole time.", l.Mode, l.Table, len(later), later[0].Tag, later[0].File, later[0].StartLine, impactPhrase(l.Mode)))
		f.Table, f.Lock = l.Table, l.Mode
		f.Confidence = evidence.Medium
		f.Remediation = "Split the migration so each blocking DDL statement commits on its own, and move backfills and index builds out of the transaction."
		out = append(out, f)
	}
	return out
}

func checkTriggerChange(c *Context, s *Statement) []Finding {
	isTrigger := s.Kind == KindCreateTrigger
	for _, sub := range s.Subcommands {
		if strings.Contains(sub, "_trig") {
			isTrigger = true
		}
	}
	if !isTrigger {
		return nil
	}
	table := firstTable(s)
	if table == "" || c.CreatedEarlier(s, table) {
		return nil
	}
	f := at(s, fmt.Sprintf("%s takes SHARE ROW EXCLUSIVE on %s: it waits for in-flight writes and blocks new writes until the transaction commits.", s.Tag, table))
	f.Table, f.Lock = table, pglock.ShareRowExclusive
	return []Finding{f}
}

func impactPhrase(m pglock.Mode) string {
	switch m.Impact() {
	case pglock.ImpactAllTraffic:
		return "blocks reads and writes"
	case pglock.ImpactWrites:
		return "blocks writes"
	case pglock.ImpactDDLOnly:
		return "blocks other schema changes and maintenance"
	default:
		return "does not block application queries"
	}
}

// unboundedLockingSelect reports whether st is SELECT ... FOR UPDATE/SHARE
// over a table with neither WHERE nor LIMIT.
func unboundedLockingSelect(st *pg_query.SelectStmt) bool {
	return st != nil && len(st.LockingClause) > 0 && st.WhereClause == nil && st.LimitCount == nil && len(st.FromClause) > 0
}

// lockTimeoutChange reports whether s changes lock_timeout, whether the
// change enables a timeout (a non-zero value) and whether it is SET LOCAL.
// RESET, SET ... TO DEFAULT and a value of 0 disable it.
func lockTimeoutChange(s *Statement) (on, local, ok bool) {
	if s.Kind != KindSet || s.node == nil {
		return false, false, false
	}
	vs := s.node.GetVariableSetStmt()
	if vs == nil || !strings.EqualFold(vs.Name, "lock_timeout") {
		return false, false, false
	}
	if _, value, ok := settingValue(s); ok {
		d, parsed := parseTimeout(value)
		return parsed && d > 0, vs.IsLocal, true
	}
	return false, vs.IsLocal, true
}
