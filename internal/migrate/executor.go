// Package migrate executes the migration under test in a dedicated session
// of the disposable database, statement by statement, with session-level
// lock_timeout and statement_timeout. It follows the same transaction plan
// as the static analysis (sqlinspect.Plan), never wraps statements that
// cannot run in a transaction block, and stops at the first failure like a
// conventional migration runner.
package migrate

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/sqlinspect"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Status of a statement or of the whole migration.
type Status string

const (
	Succeeded  Status = "succeeded"
	Failed     Status = "failed"
	Skipped    Status = "skipped"
	RolledBack Status = "rolled_back"
)

// Options configure execution.
type Options struct {
	LockTimeout      time.Duration
	StatementTimeout time.Duration
	Transaction      sqlinspect.TransactionMode
	// Reference is the time offsets are measured from, normally the start
	// of the measurement window.
	Reference time.Time
}

// StatementOutcome records one statement's execution.
type StatementOutcome struct {
	StatementID string  `json:"statement_id"`
	File        string  `json:"file"`
	Line        int     `json:"line"`
	Tag         string  `json:"tag"`
	Status      Status  `json:"status"`
	OffsetMS    float64 `json:"offset_ms"`
	DurationMS  float64 `json:"duration_ms"`
	SQLState    string  `json:"sqlstate,omitempty"`
	Error       string  `json:"error,omitempty"`
}

// Outcome records the whole migration.
type Outcome struct {
	Status          Status             `json:"status"`
	Started         time.Time          `json:"started"`
	Finished        time.Time          `json:"finished"`
	OffsetMS        float64            `json:"offset_ms"`
	DurationMS      float64            `json:"duration_ms"`
	FailedStatement string             `json:"failed_statement,omitempty"`
	SQLState        string             `json:"sqlstate,omitempty"`
	Error           string             `json:"error,omitempty"`
	Statements      []StatementOutcome `json:"statements"`
}

// LockTimedOut reports whether the migration failed to acquire a lock
// within lock_timeout (SQLSTATE 55P03).
func (o *Outcome) LockTimedOut() bool { return o.SQLState == "55P03" }

// StatementTimedOut reports whether a statement exceeded statement_timeout.
func (o *Outcome) StatementTimedOut() bool { return o.SQLState == "57014" }

// Execute runs stmts on conn. Errors executing statements are recorded in
// the outcome; the returned error is reserved for session setup failures.
func Execute(ctx context.Context, conn *pgx.Conn, stmts []*sqlinspect.Statement, opts Options) (*Outcome, error) {
	if opts.Transaction == "" {
		opts.Transaction = sqlinspect.TxnModeAuto
	}
	if opts.Reference.IsZero() {
		opts.Reference = time.Now()
	}
	setup := fmt.Sprintf("SET lock_timeout = %d; SET statement_timeout = %d",
		opts.LockTimeout.Milliseconds(), opts.StatementTimeout.Milliseconds())
	if _, err := conn.PgConn().Exec(ctx, setup).ReadAll(); err != nil {
		return nil, fmt.Errorf("configure migration session: %w", err)
	}

	out := &Outcome{Status: Succeeded, Started: time.Now()}
	out.OffsetMS = ms(out.Started.Sub(opts.Reference))
	failed := false
	for _, g := range sqlinspect.Plan(stmts, opts.Transaction) {
		if failed {
			for _, s := range g.Statements {
				out.Statements = append(out.Statements, skipped(s))
			}
			continue
		}
		if !runGroup(ctx, conn, g, opts, out) {
			failed = true
		}
	}
	out.Finished = time.Now()
	out.DurationMS = ms(out.Finished.Sub(out.Started))
	if failed {
		out.Status = Failed
	}
	return out, nil
}

// runGroup executes one group and returns false if it failed.
func runGroup(ctx context.Context, conn *pgx.Conn, g sqlinspect.Group, opts Options, out *Outcome) bool {
	if g.Wrapped {
		if err := simple(ctx, conn, "BEGIN"); err != nil {
			recordFailure(out, g.Statements[0], err)
			return false
		}
	}
	first := len(out.Statements)
	for i, s := range g.Statements {
		so := StatementOutcome{StatementID: s.ID, File: s.File, Line: s.StartLine, Tag: s.Tag}
		start := time.Now()
		err := simple(ctx, conn, s.SQL)
		so.OffsetMS, so.DurationMS = ms(start.Sub(opts.Reference)), ms(time.Since(start))
		if err == nil {
			so.Status = Succeeded
			out.Statements = append(out.Statements, so)
			continue
		}
		so.Status, so.SQLState, so.Error = Failed, sqlState(err), err.Error()
		out.Statements = append(out.Statements, so)
		out.FailedStatement, out.SQLState, out.Error = s.ID, so.SQLState, so.Error
		for _, rest := range g.Statements[i+1:] {
			out.Statements = append(out.Statements, skipped(rest))
		}
		if g.InTransaction() {
			// Leave the session usable and mark earlier statements of the
			// transaction as rolled back.
			_ = simple(context.Background(), conn, "ROLLBACK")
			for j := first; j < first+i; j++ {
				if out.Statements[j].Status == Succeeded {
					out.Statements[j].Status = RolledBack
				}
			}
		}
		return false
	}
	if g.Wrapped {
		if err := simple(ctx, conn, "COMMIT"); err != nil {
			recordFailure(out, g.Statements[len(g.Statements)-1], err)
			return false
		}
	}
	return true
}

func recordFailure(out *Outcome, s *sqlinspect.Statement, err error) {
	out.FailedStatement, out.SQLState, out.Error = s.ID, sqlState(err), err.Error()
}

func skipped(s *sqlinspect.Statement) StatementOutcome {
	return StatementOutcome{StatementID: s.ID, File: s.File, Line: s.StartLine, Tag: s.Tag, Status: Skipped}
}

// simple runs sql with the simple query protocol, which accepts every
// statement type and no parameters.
func simple(ctx context.Context, conn *pgx.Conn, sql string) error {
	_, err := conn.PgConn().Exec(ctx, sql).ReadAll()
	return err
}

func sqlState(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

func ms(d time.Duration) float64 {
	return float64(d.Round(10*time.Microsecond)) / float64(time.Millisecond)
}

// AppName is the application_name of the migration session; telemetry uses
// it to attribute lock waits to the migration.
const AppName = "prodshadow/migration"
