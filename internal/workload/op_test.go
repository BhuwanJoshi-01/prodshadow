package workload

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestClassify(t *testing.T) {
	cases := map[ErrClass]error{
		OK:                  nil,
		ErrLockTimeout:      &pgconn.PgError{Code: "55P03"},
		ErrStatementTimeout: &pgconn.PgError{Code: "57014"},
		ErrDeadlock:         fmt.Errorf("wrapped: %w", &pgconn.PgError{Code: "40P01"}),
		ErrSerialization:    &pgconn.PgError{Code: "40001"},
		ErrUndefinedObject:  &pgconn.PgError{Code: "42703"},
		ErrConnection:       &pgconn.PgError{Code: "08006"},
		ErrOther:            &pgconn.PgError{Code: "23505"},
	}
	for want, err := range cases {
		if got := Classify(err); got != want {
			t.Errorf("Classify(%v) = %s, want %s", err, got, want)
		}
	}
	if Classify(context.DeadlineExceeded) != ErrStatementTimeout {
		t.Error("deadline exceeded should count as a timeout")
	}
	if Classify(errors.New("boom")) != ErrOther {
		t.Error("plain errors are other")
	}
}

func TestErrClassNames(t *testing.T) {
	if ErrLockTimeout.String() != "lock_timeout" || ErrClass(200).String() != "unknown" {
		t.Fatal("unexpected names")
	}
}
