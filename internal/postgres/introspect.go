package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

// ServerInfo describes the server an experiment ran on.
type ServerInfo struct {
	Version       string            `json:"version"`
	VersionNum    int               `json:"version_num"`
	Major         int               `json:"major"`
	Settings      map[string]string `json:"settings"`
	SettingsHash  string            `json:"settings_hash"`
	CPUs          string            `json:"cpus,omitempty"`
	DatabaseBytes int64             `json:"database_bytes,omitempty"`
}

// reportedSettings influence lock and performance behaviour and are
// recorded so runs on different configurations can be told apart.
var reportedSettings = []string{
	"shared_buffers", "work_mem", "maintenance_work_mem", "max_connections",
	"effective_cache_size", "max_wal_size", "checkpoint_timeout", "wal_level",
	"synchronous_commit", "fsync", "deadlock_timeout", "lock_timeout",
	"statement_timeout", "idle_in_transaction_session_timeout",
	"max_parallel_workers", "max_parallel_maintenance_workers", "jit",
	"default_transaction_isolation", "log_lock_waits",
}

// Introspect reads version and relevant settings.
func Introspect(ctx context.Context, conn *pgx.Conn) (ServerInfo, error) {
	var info ServerInfo
	if err := conn.QueryRow(ctx, "SELECT version(), current_setting('server_version_num')::int, pg_database_size(current_database())").
		Scan(&info.Version, &info.VersionNum, &info.DatabaseBytes); err != nil {
		return info, fmt.Errorf("read server version: %w", err)
	}
	info.Major = info.VersionNum / 10000
	rows, err := conn.Query(ctx, "SELECT name, setting || COALESCE(unit, '') FROM pg_settings WHERE name = ANY($1)", reportedSettings)
	if err != nil {
		return info, fmt.Errorf("read settings: %w", err)
	}
	info.Settings = map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			rows.Close()
			return info, err
		}
		info.Settings[k] = v
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return info, err
	}
	info.SettingsHash = hashSettings(info.Settings)
	return info, nil
}

func hashSettings(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k + "=" + m[k] + "\n")
	}
	sum := sha256.Sum256([]byte(b.String()))
	return "sha256:" + hex.EncodeToString(sum[:8])
}

// TableSize describes one user table after seeding.
type TableSize struct {
	Name         string `json:"name"`
	RowsEstimate int64  `json:"rows_estimate"`
	TotalBytes   int64  `json:"total_bytes"`
	Indexes      int    `json:"indexes"`
	// Partitioned marks a partitioned table. It stores no rows itself; its
	// estimate is the sum of its partitions, which are listed separately.
	Partitioned bool `json:"partitioned,omitempty"`
}

// TotalRows sums the row estimates of tables that store rows, so rows of
// partitions are not counted a second time through their parent.
func TotalRows(tables []TableSize) int64 {
	var n int64
	for _, t := range tables {
		if !t.Partitioned {
			n += t.RowsEstimate
		}
	}
	return n
}

// TableSizes lists user tables with estimated row counts and sizes. Call
// ANALYZE first for accurate estimates.
func TableSizes(ctx context.Context, conn *pgx.Conn) ([]TableSize, error) {
	rows, err := conn.Query(ctx, `
SELECT format('%s.%s', n.nspname, c.relname),
       GREATEST(c.reltuples, 0)::bigint,
       pg_total_relation_size(c.oid),
       (SELECT count(*) FROM pg_index i WHERE i.indrelid = c.oid)::int,
       c.relkind = 'p'
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind IN ('r', 'p')
  AND n.nspname NOT IN ('pg_catalog', 'information_schema')
  AND n.nspname NOT LIKE 'pg_toast%'
ORDER BY pg_total_relation_size(c.oid) DESC, 1`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (TableSize, error) {
		var t TableSize
		err := r.Scan(&t.Name, &t.RowsEstimate, &t.TotalBytes, &t.Indexes, &t.Partitioned)
		return t, err
	})
}
