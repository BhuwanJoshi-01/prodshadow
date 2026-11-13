package migrate

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestSQLState(t *testing.T) {
	err := fmt.Errorf("wrapped: %w", &pgconn.PgError{Code: "55P03"})
	if sqlState(err) != "55P03" || sqlState(errors.New("x")) != "" {
		t.Fatal("sqlState extraction failed")
	}
}

func TestOutcomeHelpers(t *testing.T) {
	o := &Outcome{SQLState: "55P03"}
	if !o.LockTimedOut() || o.StatementTimedOut() {
		t.Fatal("lock timeout helper wrong")
	}
	o.SQLState = "57014"
	if o.LockTimedOut() || !o.StatementTimedOut() {
		t.Fatal("statement timeout helper wrong")
	}
}

func TestMillisecondRounding(t *testing.T) {
	if got := ms(1234567 * time.Nanosecond); got != 1.23 {
		t.Fatalf("ms = %v", got)
	}
}
