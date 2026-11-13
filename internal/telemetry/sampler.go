// Package telemetry samples lock waits and session states from the
// disposable server while a workload runs, and snapshots cumulative
// database counters around each phase.
//
// Polling cannot see waits shorter than the sampling interval; the report
// always states the interval. Client-side latency, recorded for every
// workload operation, is the complementary measurement that catches short
// waits.
package telemetry

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// Session is one backend observed in a sample.
type Session struct {
	PID         int32   `json:"pid"`
	Application string  `json:"application"`
	State       string  `json:"state"`
	WaitType    string  `json:"wait_type,omitempty"`
	WaitEvent   string  `json:"wait_event,omitempty"`
	BlockedBy   []int32 `json:"blocked_by,omitempty"`
	WaitMS      float64 `json:"wait_ms,omitempty"`
	XactMS      float64 `json:"xact_ms,omitempty"`
	LockMode    string  `json:"lock_mode,omitempty"`
	LockType    string  `json:"lock_type,omitempty"`
	Relation    string  `json:"relation,omitempty"`
}

// WaitingOnLock reports whether the session waits for a heavyweight lock.
func (s Session) WaitingOnLock() bool { return s.WaitType == "Lock" }

// Sample is one poll of the server.
type Sample struct {
	At       time.Time `json:"at"`
	Sessions []Session `json:"sessions"`
}

// sampleQuery reads client backends of the experiment database. The
// blocking PIDs are only computed for sessions waiting on a lock, because
// pg_blocking_pids() briefly locks the lock manager. pg_locks.waitstart
// (PostgreSQL 14+) gives the exact start of the current lock wait.
const sampleQuery = `
SELECT a.pid,
       COALESCE(a.application_name, ''),
       COALESCE(a.state, ''),
       COALESCE(a.wait_event_type, ''),
       COALESCE(a.wait_event, ''),
       CASE WHEN a.wait_event_type = 'Lock' THEN pg_blocking_pids(a.pid) ELSE '{}'::int[] END,
       CASE WHEN a.wait_event_type = 'Lock'
            THEN EXTRACT(EPOCH FROM clock_timestamp() - COALESCE(l.waitstart, a.state_change)) * 1000
            ELSE 0 END,
       COALESCE(EXTRACT(EPOCH FROM clock_timestamp() - a.xact_start) * 1000, 0),
       COALESCE(l.mode, ''),
       COALESCE(l.locktype, ''),
       COALESCE(l.relation::regclass::text, '')
FROM pg_stat_activity a
LEFT JOIN LATERAL (
    SELECT mode, locktype, relation, waitstart
    FROM pg_locks
    WHERE pid = a.pid AND NOT granted
    LIMIT 1
) l ON true
WHERE a.datname = current_database()
  AND a.pid <> pg_backend_pid()
  AND a.backend_type = 'client backend'`

// Sampler polls the server at a fixed interval.
type Sampler struct {
	conn     *pgx.Conn
	interval time.Duration
}

// NewSampler uses conn, which must be connected to the experiment database
// with a role that can see all sessions (the administrative role).
func NewSampler(conn *pgx.Conn, interval time.Duration) *Sampler {
	return &Sampler{conn: conn, interval: interval}
}

// Run samples until ctx is done and returns all samples. Individual sample
// failures are counted rather than aborting the run.
func (s *Sampler) Run(ctx context.Context) ([]Sample, int) {
	var samples []Sample
	failures := 0
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		qctx, cancel := context.WithTimeout(ctx, s.interval*5+time.Second)
		sample, err := s.once(qctx)
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				return samples, failures
			}
			failures++
		} else {
			samples = append(samples, sample)
		}
		select {
		case <-ctx.Done():
			return samples, failures
		case <-ticker.C:
		}
	}
}

func (s *Sampler) once(ctx context.Context) (Sample, error) {
	rows, err := s.conn.Query(ctx, sampleQuery)
	if err != nil {
		return Sample{}, err
	}
	at := time.Now()
	sessions, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Session, error) {
		var x Session
		err := r.Scan(&x.PID, &x.Application, &x.State, &x.WaitType, &x.WaitEvent, &x.BlockedBy,
			&x.WaitMS, &x.XactMS, &x.LockMode, &x.LockType, &x.Relation)
		return x, err
	})
	if err != nil {
		return Sample{}, err
	}
	return Sample{At: at, Sessions: sessions}, nil
}
