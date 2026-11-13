package fixture

import (
	"context"
	"fmt"
	"strings"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/sqlinspect"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/sqlsplit"

	"github.com/jackc/pgx/v5"
)

// ApplySQL executes a schema or seed file statement by statement so that a
// failure reports the exact file and line. The connection should belong to
// the unprivileged application role.
func ApplySQL(ctx context.Context, conn *pgx.Conn, src sqlinspect.Source) (int, error) {
	stmts, err := sqlsplit.Split(src.Content)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", src.Path, err)
	}
	for _, st := range stmts {
		if err := checkFixtureStatement(st.Text); err != nil {
			return 0, fmt.Errorf("%s:%d: %w", src.Path, st.StartLine, err)
		}
	}
	for i, st := range stmts {
		// The simple protocol accepts any statement type, including
		// those that cannot be prepared.
		if _, err := conn.PgConn().Exec(ctx, st.Text).ReadAll(); err != nil {
			return i, fmt.Errorf("%s:%d: %w", src.Path, st.StartLine, err)
		}
	}
	return len(stmts), nil
}

// forbidden statement prefixes in fixtures. They would fail for the
// unprivileged role anyway; rejecting them first gives a clear message and
// documents intent.
var forbidden = []struct{ prefix, why string }{
	{"ALTER SYSTEM", "changes server configuration"},
	{"CREATE ROLE", "manages roles"},
	{"CREATE USER", "manages roles"},
	{"ALTER ROLE", "manages roles"},
	{"CREATE DATABASE", "creates databases"},
	{"DROP DATABASE", "drops databases"},
	{"CREATE EXTENSION", "installs extensions; list them in postgres.extensions instead"},
	{"LOAD", "loads shared libraries"},
}

func checkFixtureStatement(sql string) error {
	upper := strings.ToUpper(strings.Join(strings.Fields(sql), " "))
	for _, f := range forbidden {
		if strings.HasPrefix(upper, f.prefix+" ") || upper == f.prefix {
			return fmt.Errorf("%s is not allowed in fixtures: it %s", f.prefix, f.why)
		}
	}
	if strings.HasPrefix(upper, "COPY ") && (strings.Contains(upper, " PROGRAM ") || !strings.Contains(upper, "STDIN") && !strings.Contains(upper, "STDOUT")) {
		return fmt.Errorf("COPY to or from server files or programs is not allowed in fixtures; use INSERT or the generator")
	}
	return nil
}
