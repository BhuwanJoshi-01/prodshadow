package workload

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// ErrClass groups operation failures by their operational meaning.
type ErrClass uint8

const (
	OK ErrClass = iota
	// ErrLockTimeout is SQLSTATE 55P03 (lock_not_available).
	ErrLockTimeout
	// ErrStatementTimeout is SQLSTATE 57014 (query_canceled).
	ErrStatementTimeout
	// ErrDeadlock is SQLSTATE 40P01.
	ErrDeadlock
	// ErrSerialization is SQLSTATE 40001.
	ErrSerialization
	// ErrConnection covers broken or refused sessions.
	ErrConnection
	// ErrUndefinedObject covers missing tables or columns (42P01, 42703),
	// for example when a migration drops something the workload uses.
	ErrUndefinedObject
	// ErrOther is any other failure.
	ErrOther
	// ErrDropped marks open-loop arrivals that could not be dispatched
	// because every session was busy.
	ErrDropped
)

var errClassNames = [...]string{
	OK: "ok", ErrLockTimeout: "lock_timeout", ErrStatementTimeout: "statement_timeout",
	ErrDeadlock: "deadlock", ErrSerialization: "serialization_failure", ErrConnection: "connection",
	ErrUndefinedObject: "undefined_object", ErrOther: "other", ErrDropped: "dropped",
}

func (c ErrClass) String() string {
	if int(c) < len(errClassNames) {
		return errClassNames[c]
	}
	return "unknown"
}

// Classify maps an error to an ErrClass.
func Classify(err error) ErrClass {
	if err == nil {
		return OK
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "55P03":
			return ErrLockTimeout
		case "57014":
			return ErrStatementTimeout
		case "40P01":
			return ErrDeadlock
		case "40001":
			return ErrSerialization
		case "42P01", "42703", "42883":
			return ErrUndefinedObject
		}
		if len(pgErr.Code) >= 2 && pgErr.Code[:2] == "08" {
			return ErrConnection
		}
		return ErrOther
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return ErrStatementTimeout
	}
	if pgconn.SafeToRetry(err) || pgconn.Timeout(err) {
		return ErrConnection
	}
	var connErr *pgconn.ConnectError
	if errors.As(err, &connErr) {
		return ErrConnection
	}
	return ErrOther
}

// Op is one executed (or dropped) operation. Times are nanoseconds
// relative to the start of the measurement window, so warm-up operations
// have a negative Start.
type Op struct {
	Scenario uint16
	Err      ErrClass
	Start    int64
	Latency  int64
}
