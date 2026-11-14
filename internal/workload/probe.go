package workload

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/gen"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ProbeTimeout bounds each compatibility probe.
const ProbeTimeout = 5 * time.Second

// ProbeResult is the outcome of one compatibility probe.
type ProbeResult struct {
	Version  string `json:"version"`
	ID       string `json:"id"`
	OK       bool   `json:"ok"`
	SQLState string `json:"sqlstate,omitempty"`
	Error    string `json:"error,omitempty"`
}

// RunProbes executes every compatibility probe of s once on conn, in
// declaration order. Each probe runs in its own transaction with a
// statement timeout and is rolled back, so probes that write leave no
// trace and a failing probe does not affect the next one. Parameters are
// drawn from the probe's generators with a stream derived from seed and
// stream, so the same run draws the same values before and after the
// migration.
func RunProbes(ctx context.Context, conn *pgx.Conn, s *Spec, seed int64, stream string) ([]ProbeResult, error) {
	var out []ProbeResult
	for _, v := range s.Compatibility {
		for _, p := range v.Probes {
			args, err := probeArgs(p, v.Version, seed, stream)
			if err != nil {
				return out, fmt.Errorf("probe %s/%s: %w", v.Version, p.ID, err)
			}
			res := ProbeResult{Version: v.Version, ID: p.ID, OK: true}
			if err := runProbe(ctx, conn, p.SQL, args); err != nil {
				if ctx.Err() != nil {
					return out, ctx.Err()
				}
				res.OK, res.Error = false, err.Error()
				var pgErr *pgconn.PgError
				if errors.As(err, &pgErr) {
					res.SQLState, res.Error = pgErr.Code, pgErr.Message
				}
			}
			out = append(out, res)
		}
	}
	return out, nil
}

func probeArgs(p Probe, version string, seed int64, stream string) ([]any, error) {
	args := make([]any, len(p.Parameters))
	for i, spec := range p.Parameters {
		g, err := gen.New(spec)
		if err != nil {
			return nil, err
		}
		args[i] = g.Next(gen.Stream(seed, fmt.Sprintf("%s/compat/%s/%s/p%d", stream, version, p.ID, i)))
	}
	return args, nil
}

func runProbe(ctx context.Context, conn *pgx.Conn, sql string, args []any) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		rctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = tx.Rollback(rctx)
	}()
	if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL statement_timeout = %d", ProbeTimeout.Milliseconds())); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, sql, args...)
	return err
}
