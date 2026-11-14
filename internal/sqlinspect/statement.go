// Package sqlinspect statically analyses PostgreSQL migration files.
//
// It splits each file into statements with exact line numbers, parses every
// statement with the PostgreSQL grammar (libpg_query through pg_query_go),
// derives the table-level locks a statement acquires, and evaluates a small
// set of hazard rules. The analysis is advisory: the disposable database is
// the source of truth, and statements the parser cannot handle are still
// executed there and reported as an analysis limitation.
package sqlinspect

import (
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/pglock"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

// Kind is a coarse statement classification.
type Kind string

const (
	KindCreateIndex     Kind = "create_index"
	KindAlterTable      Kind = "alter_table"
	KindRename          Kind = "rename"
	KindDropTable       Kind = "drop_table"
	KindDropIndex       Kind = "drop_index"
	KindDropOther       Kind = "drop_other"
	KindCreateTable     Kind = "create_table"
	KindInsert          Kind = "insert"
	KindUpdate          Kind = "update"
	KindDelete          Kind = "delete"
	KindMerge           Kind = "merge"
	KindSelect          Kind = "select"
	KindTruncate        Kind = "truncate"
	KindVacuum          Kind = "vacuum"
	KindCluster         Kind = "cluster"
	KindReindex         Kind = "reindex"
	KindRefreshMatView  Kind = "refresh_materialized_view"
	KindLockTable       Kind = "lock_table"
	KindCreateTrigger   Kind = "create_trigger"
	KindTransaction     Kind = "transaction_control"
	KindSet             Kind = "set"
	KindProcedural      Kind = "procedural"
	KindCreateExtension Kind = "create_extension"
	KindCopy            Kind = "copy"
	KindComment         Kind = "comment"
	KindOtherDDL        Kind = "other_ddl"
	KindUnknown         Kind = "unknown"
)

// TxnRequirement states how a statement interacts with transaction blocks.
type TxnRequirement string

const (
	// TxnAny statements may run inside or outside a transaction block.
	TxnAny TxnRequirement = "any"
	// TxnNotInBlock statements fail inside a transaction block, for
	// example CREATE INDEX CONCURRENTLY.
	TxnNotInBlock TxnRequirement = "no_transaction_block"
	// TxnControl statements begin or end transactions themselves.
	TxnControl TxnRequirement = "transaction_control"
)

// TableLock is a lock a statement acquires on one relation.
type TableLock struct {
	Table string      `json:"table"`
	Mode  pglock.Mode `json:"mode"`
	// CreatedBy is the ID of the earlier statement of the migration that
	// created Table. Such a lock cannot block the application: nothing
	// uses the relation yet.
	CreatedBy string `json:"created_by,omitempty"`
	// Via names the object that implies a lock on a relation the statement
	// does not name, such as "index orders_status_idx" or "partition of
	// events". It is set only when a Catalog resolved the lock.
	Via string `json:"via,omitempty"`
}

// Statement is one analysed migration statement.
type Statement struct {
	// ID is "<file number>.<statement number>", both 1-based.
	ID        string `json:"id"`
	File      string `json:"file"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	// SQL is the statement text. Reports omit it unless raw text
	// persistence is enabled; Fingerprint identifies it instead.
	SQL         string         `json:"-"`
	Fingerprint string         `json:"fingerprint,omitempty"`
	Kind        Kind           `json:"kind"`
	Tag         string         `json:"tag"`
	Locks       []TableLock    `json:"locks,omitempty"`
	Txn         TxnRequirement `json:"transaction_requirement"`
	Subcommands []string       `json:"subcommands,omitempty"`
	Parsed      bool           `json:"parsed"`
	ParseError  string         `json:"parse_error,omitempty"`
	// Annotations are inline "prodshadow:" directives from leading
	// comments.
	Annotations []Annotation `json:"annotations,omitempty"`

	node *pg_query.Node
	// referencedBy lists the foreign keys ("table.constraint") that make a
	// DROP TABLE or TRUNCATE without CASCADE fail; set by Catalog.Resolve.
	referencedBy []string
	// refreshProblem says why a REFRESH MATERIALIZED VIEW CONCURRENTLY
	// will be rejected; set by Catalog.Resolve.
	refreshProblem string
	// dependentViews are views that make a DROP COLUMN, ALTER COLUMN TYPE
	// or DROP TABLE fail; set by Catalog.Resolve.
	dependentViews []viewDependency
	// viewReplaceProblem says why a CREATE OR REPLACE VIEW will be
	// rejected; set by Catalog.Resolve.
	viewReplaceProblem string
	// detachProblem names the DEFAULT partition that makes a DETACH
	// PARTITION ... CONCURRENTLY fail; set by Catalog.Resolve.
	detachProblem string
	// bodyAnalysed marks a DO block whose PL/pgSQL body was parsed;
	// dynamicSQL marks one that also runs dynamic SQL (EXECUTE).
	bodyAnalysed bool
	dynamicSQL   bool
}

// ExistingLock returns the strongest lock the statement takes on a relation
// that existed before the migration: the lock that can make application
// sessions wait.
func (s *Statement) ExistingLock() TableLock {
	var best TableLock
	for _, l := range s.Locks {
		if l.CreatedBy == "" && l.Mode > best.Mode {
			best = l
		}
	}
	return best
}

// OnIndex reports whether l is the lock a DROP INDEX, REINDEX INDEX or
// ALTER INDEX statement records against the index it names. Such a lock is
// not a table lock; with a Catalog, the index's table gets its own entry.
func (s *Statement) OnIndex(l TableLock) bool {
	if l.Via != "" || s.node == nil {
		return false
	}
	switch {
	case s.node.GetDropStmt().GetRemoveType() == pg_query.ObjectType_OBJECT_INDEX:
		return true
	case s.node.GetReindexStmt().GetKind() == pg_query.ReindexObjectType_REINDEX_OBJECT_INDEX:
		return true
	case s.node.GetRenameStmt().GetRenameType() == pg_query.ObjectType_OBJECT_INDEX:
		return true
	case s.node.GetAlterTableStmt().GetObjtype() == pg_query.ObjectType_OBJECT_INDEX:
		return true
	}
	return false
}

// StrongestLock returns the strongest lock mode the statement acquires.
func (s *Statement) StrongestLock() pglock.Mode {
	best := pglock.None
	for _, l := range s.Locks {
		best = pglock.Strongest(best, l.Mode)
	}
	return best
}

// LockOn returns the mode acquired on table, or pglock.None.
func (s *Statement) LockOn(table string) pglock.Mode {
	best := pglock.None
	for _, l := range s.Locks {
		if l.Table == table {
			best = pglock.Strongest(best, l.Mode)
		}
	}
	return best
}

// Tables returns the distinct relations the statement locks, in order.
func (s *Statement) Tables() []string {
	var out []string
	seen := map[string]bool{}
	for _, l := range s.Locks {
		if !seen[l.Table] {
			seen[l.Table] = true
			out = append(out, l.Table)
		}
	}
	return out
}

// Node exposes the parse tree to rules. It is nil when parsing failed.
func (s *Statement) Node() *pg_query.Node { return s.node }

// addImpliedLock records a lock on a relation the statement does not name.
// An existing lock on the relation keeps its origin.
func (s *Statement) addImpliedLock(table string, mode pglock.Mode, via string) {
	if table == "" || mode == pglock.None {
		return
	}
	for i, l := range s.Locks {
		if catalogKey(l.Table) == catalogKey(table) {
			s.Locks[i].Mode = pglock.Strongest(l.Mode, mode)
			return
		}
	}
	s.Locks = append(s.Locks, TableLock{Table: table, Mode: mode, Via: via})
}

func (s *Statement) addLock(table string, mode pglock.Mode) {
	if table == "" || mode == pglock.None {
		return
	}
	for i, l := range s.Locks {
		if l.Table == table {
			s.Locks[i].Mode = pglock.Strongest(l.Mode, mode)
			return
		}
	}
	s.Locks = append(s.Locks, TableLock{Table: table, Mode: mode})
}
