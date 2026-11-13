package telemetry

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Counters is a snapshot of cumulative database statistics. The difference
// between snapshots taken before and after a phase shows WAL volume,
// deadlocks and temporary file usage caused by that phase.
type Counters struct {
	XactCommit   int64  `json:"xact_commit"`
	XactRollback int64  `json:"xact_rollback"`
	Deadlocks    int64  `json:"deadlocks"`
	TempBytes    int64  `json:"temp_bytes"`
	BlocksRead   int64  `json:"blocks_read"`
	BlocksHit    int64  `json:"blocks_hit"`
	TupInserted  int64  `json:"tuples_inserted"`
	TupUpdated   int64  `json:"tuples_updated"`
	TupDeleted   int64  `json:"tuples_deleted"`
	WALLSN       string `json:"-"`
	WALBytes     int64  `json:"wal_bytes"`
	DatabaseSize int64  `json:"database_bytes"`
}

// Snapshot reads counters for the connected database. Statistics are
// flushed with a short delay, so callers should snapshot after a pause.
func Snapshot(ctx context.Context, conn *pgx.Conn) (Counters, error) {
	var c Counters
	err := conn.QueryRow(ctx, `
SELECT xact_commit, xact_rollback, deadlocks, temp_bytes, blks_read, blks_hit,
       tup_inserted, tup_updated, tup_deleted,
       pg_current_wal_lsn()::text,
       pg_database_size(current_database())
FROM pg_stat_database WHERE datname = current_database()`).Scan(
		&c.XactCommit, &c.XactRollback, &c.Deadlocks, &c.TempBytes, &c.BlocksRead, &c.BlocksHit,
		&c.TupInserted, &c.TupUpdated, &c.TupDeleted, &c.WALLSN, &c.DatabaseSize)
	if err != nil {
		return c, fmt.Errorf("read database counters: %w", err)
	}
	return c, nil
}

// Delta returns after minus before. WAL bytes are computed by the server
// from the two LSNs.
func Delta(ctx context.Context, conn *pgx.Conn, before, after Counters) (Counters, error) {
	d := Counters{
		XactCommit:   after.XactCommit - before.XactCommit,
		XactRollback: after.XactRollback - before.XactRollback,
		Deadlocks:    after.Deadlocks - before.Deadlocks,
		TempBytes:    after.TempBytes - before.TempBytes,
		BlocksRead:   after.BlocksRead - before.BlocksRead,
		BlocksHit:    after.BlocksHit - before.BlocksHit,
		TupInserted:  after.TupInserted - before.TupInserted,
		TupUpdated:   after.TupUpdated - before.TupUpdated,
		TupDeleted:   after.TupDeleted - before.TupDeleted,
		DatabaseSize: after.DatabaseSize - before.DatabaseSize,
	}
	if before.WALLSN != "" && after.WALLSN != "" {
		if err := conn.QueryRow(ctx, "SELECT pg_wal_lsn_diff($1::pg_lsn, $2::pg_lsn)::bigint", after.WALLSN, before.WALLSN).Scan(&d.WALBytes); err != nil {
			return d, fmt.Errorf("compute WAL delta: %w", err)
		}
	}
	return d, nil
}
