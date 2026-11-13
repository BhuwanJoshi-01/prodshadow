package sqlinspect

import (
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/pglock"
)

// Finding is a static hazard attached to a statement or to the migration.
type Finding struct {
	RuleID      string              `json:"rule_id"`
	Rule        string              `json:"rule"`
	Severity    evidence.Severity   `json:"severity"`
	Confidence  evidence.Confidence `json:"confidence"`
	Evidence    evidence.Kind       `json:"evidence_kind"`
	Message     string              `json:"message"`
	StatementID string              `json:"statement_id,omitempty"`
	File        string              `json:"file,omitempty"`
	Line        int                 `json:"line,omitempty"`
	Table       string              `json:"table,omitempty"`
	Lock        pglock.Mode         `json:"lock,omitempty"`
	Remediation string              `json:"remediation,omitempty"`
	DocsURL     string              `json:"docs_url,omitempty"`

	Suppressed        bool   `json:"suppressed,omitempty"`
	SuppressionReason string `json:"suppression_reason,omitempty"`
	SuppressionExpiry string `json:"suppression_expires,omitempty"`
}

// Documentation links used by rules.
const (
	docExplicitLocking = "https://www.postgresql.org/docs/current/explicit-locking.html"
	docCreateIndex     = "https://www.postgresql.org/docs/current/sql-createindex.html#SQL-CREATEINDEX-CONCURRENTLY"
	docAlterTable      = "https://www.postgresql.org/docs/current/sql-altertable.html"
	docLockTimeout     = "https://www.postgresql.org/docs/current/runtime-config-client.html#GUC-LOCK-TIMEOUT"
	docVacuum          = "https://www.postgresql.org/docs/current/sql-vacuum.html"
	docReindex         = "https://www.postgresql.org/docs/current/sql-reindex.html"
	docCluster         = "https://www.postgresql.org/docs/current/sql-cluster.html"
	docRefreshMatView  = "https://www.postgresql.org/docs/current/sql-refreshmaterializedview.html"
	docLock            = "https://www.postgresql.org/docs/current/sql-lock.html"
	docUpdate          = "https://www.postgresql.org/docs/current/sql-update.html"
	docDropTable       = "https://www.postgresql.org/docs/current/sql-droptable.html"
	docCreateTrigger   = "https://www.postgresql.org/docs/current/sql-createtrigger.html"
	docCreateTable     = "https://www.postgresql.org/docs/current/sql-createtable.html"
	docTransactions    = "https://www.postgresql.org/docs/current/tutorial-transactions.html"
)
