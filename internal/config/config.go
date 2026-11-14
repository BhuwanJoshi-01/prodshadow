// Package config defines the prodshadow.yaml configuration contract: its
// typed model, defaults, loading, environment overrides and validation.
//
// The configuration intentionally has no field that accepts a database
// connection string. Every database that ProdShadow executes SQL against is
// created by the isolation manager; see docs/threat-model.md.
package config

// CurrentVersion is the only configuration schema version understood by this
// build of ProdShadow.
const CurrentVersion = 1

// Config is the root of prodshadow.yaml.
type Config struct {
	Version   int             `yaml:"version" json:"version"`
	Project   string          `yaml:"project" json:"project"`
	Postgres  PostgresConfig  `yaml:"postgres" json:"postgres"`
	Fixture   FixtureConfig   `yaml:"fixture" json:"fixture"`
	Workload  WorkloadConfig  `yaml:"workload" json:"workload"`
	Migration MigrationConfig `yaml:"migration" json:"migration"`
	Policy    PolicyConfig    `yaml:"policy" json:"policy"`
	Report    ReportConfig    `yaml:"report" json:"report"`
	Privacy   PrivacyConfig   `yaml:"privacy" json:"privacy"`
	Telemetry TelemetryConfig `yaml:"telemetry" json:"telemetry"`

	// baseDir is the directory relative paths are resolved against. It is
	// the directory containing the configuration file.
	baseDir string
	// path is the file the configuration was loaded from, if any.
	path string
	// lines maps dotted key paths to source lines for diagnostics.
	lines map[string]int
}

// PostgresConfig selects the PostgreSQL server used for disposable databases.
type PostgresConfig struct {
	// Version is the PostgreSQL major version, e.g. "17".
	Version string `yaml:"version" json:"version"`
	// Image is the container image. Pin a digest in CI for reproducibility.
	Image string `yaml:"image" json:"image,omitempty"`
	// Database is the name of the database created inside the server.
	Database string `yaml:"database" json:"database"`
	// Extensions are created by the administrative session before the
	// fixture is applied.
	Extensions []string `yaml:"extensions" json:"extensions,omitempty"`
	// Settings are server configuration parameters passed at start-up.
	Settings map[string]string `yaml:"settings" json:"settings,omitempty"`
	// Isolation configures the disposable execution target.
	Isolation IsolationConfig `yaml:"isolation" json:"isolation"`
}

// IsolationConfig describes how the disposable server is provisioned.
type IsolationConfig struct {
	// Provider is one of "docker", "podman" or "local".
	Provider string `yaml:"provider" json:"provider"`
	// Network is "none" (unix socket only) or "loopback" (127.0.0.1 port).
	Network string `yaml:"network" json:"network"`
	// MemoryMB caps container memory.
	MemoryMB int `yaml:"memory_mb" json:"memory_mb"`
	// CPUs caps container CPU, e.g. 2 or 1.5.
	CPUs float64 `yaml:"cpus" json:"cpus"`
	// EphemeralDiskMB caps the size of the tmpfs data directory.
	EphemeralDiskMB int `yaml:"ephemeral_disk_mb" json:"ephemeral_disk_mb"`
	// PidsLimit caps the number of processes inside the container.
	PidsLimit int `yaml:"pids_limit" json:"pids_limit"`
	// StartupTimeoutSeconds bounds how long server start-up may take.
	StartupTimeoutSeconds int `yaml:"startup_timeout_seconds" json:"startup_timeout_seconds"`
	// Binary optionally overrides the container CLI path ("docker",
	// "podman") or, for the local provider, the PostgreSQL bin directory.
	Binary string `yaml:"binary" json:"binary,omitempty"`
}

// FixtureConfig describes how the disposable database is populated.
type FixtureConfig struct {
	// Schema is a SQL file that creates tables, indexes and constraints.
	Schema string `yaml:"schema" json:"schema,omitempty"`
	// SeedSQL is a SQL file that inserts data.
	SeedSQL string `yaml:"seed_sql" json:"seed_sql,omitempty"`
	// Generator optionally produces deterministic synthetic rows.
	Generator *GeneratorRef `yaml:"generator" json:"generator,omitempty"`
	// Representation states how the fixture relates to production data.
	// Only "manual-fixture" and "synthetic" are recognised in v1.
	Representation string `yaml:"representation" json:"representation"`
	// MaxRows caps the total number of generated rows.
	MaxRows int64 `yaml:"max_rows" json:"max_rows"`
}

// GeneratorRef points at a deterministic data generator specification.
type GeneratorRef struct {
	Path string `yaml:"path" json:"path"`
	Seed int64  `yaml:"seed" json:"seed"`
}

// WorkloadConfig controls the concurrent workload run against each database.
type WorkloadConfig struct {
	// File is the workload definition (see docs/workload.md).
	File string `yaml:"file" json:"file,omitempty"`
	// WarmupSeconds is discarded time before measurement starts.
	WarmupSeconds float64 `yaml:"warmup_seconds" json:"warmup_seconds"`
	// MeasurementSeconds is the length of each measured window.
	MeasurementSeconds float64 `yaml:"measurement_seconds" json:"measurement_seconds"`
	// Concurrency is the number of concurrent client sessions.
	Concurrency int `yaml:"concurrency" json:"concurrency"`
	// ArrivalRate, when positive, switches from closed-loop to open-loop
	// scheduling at the given operations per second.
	ArrivalRate float64 `yaml:"arrival_rate" json:"arrival_rate,omitempty"`
	// Repeats is the number of baseline/candidate pairs.
	Repeats int `yaml:"repeats" json:"repeats"`
	// Seed makes parameter generation and scenario selection reproducible.
	Seed int64 `yaml:"seed" json:"seed"`
	// MinSamplesPerScenario is the smallest sample count considered
	// sufficient for latency comparison.
	MinSamplesPerScenario int `yaml:"min_samples_per_scenario" json:"min_samples_per_scenario"`
}

// MigrationConfig describes the migration under test.
type MigrationConfig struct {
	// Files are executed in order.
	Files []string `yaml:"files" json:"files"`
	// LockTimeoutMS is applied as a session-local lock_timeout.
	LockTimeoutMS int `yaml:"lock_timeout_ms" json:"lock_timeout_ms"`
	// StatementTimeoutMS is applied as a session-local statement_timeout.
	StatementTimeoutMS int `yaml:"statement_timeout_ms" json:"statement_timeout_ms"`
	// StartAfterSeconds delays the migration into the measurement window so
	// pre-migration samples exist in the candidate run.
	StartAfterSeconds float64 `yaml:"start_after_seconds" json:"start_after_seconds"`
	// RunAs is "owner" (default) or "superuser".
	RunAs string `yaml:"run_as" json:"run_as"`
	// Transaction is "auto", "per-file" or "none".
	Transaction string `yaml:"transaction" json:"transaction"`
	// LockRetries reruns a transaction (or statement) that fails on
	// lock_timeout, as a deploy tool that retries lock_not_available does.
	LockRetries int `yaml:"lock_retries" json:"lock_retries"`
	// RetryBackoffMS times the attempt number is the pause before a retry.
	RetryBackoffMS int `yaml:"retry_backoff_ms" json:"retry_backoff_ms"`
}

// PolicyConfig maps evidence to a CI decision.
type PolicyConfig struct {
	// FailOn lists severities that block: info, warning, critical.
	FailOn []string `yaml:"fail_on" json:"fail_on"`
	// Inconclusive is "fail" (exit 5) or "pass" (exit 0 with a notice).
	Inconclusive string `yaml:"inconclusive" json:"inconclusive"`

	MaxErrorRateIncreasePctPoints *float64 `yaml:"max_error_rate_increase_pct_points" json:"max_error_rate_increase_pct_points,omitempty"`
	MaxP95LatencyRatio            *float64 `yaml:"max_p95_latency_ratio" json:"max_p95_latency_ratio,omitempty"`
	MaxP99LatencyRatio            *float64 `yaml:"max_p99_latency_ratio" json:"max_p99_latency_ratio,omitempty"`
	MaxLockWaitMS                 *float64 `yaml:"max_lock_wait_ms" json:"max_lock_wait_ms,omitempty"`
	MaxThroughputDropPct          *float64 `yaml:"max_throughput_drop_pct" json:"max_throughput_drop_pct,omitempty"`
	MaxMigrationSeconds           *float64 `yaml:"max_migration_seconds" json:"max_migration_seconds,omitempty"`
	// MaxQueuedSessions bounds how many workload sessions may queue behind
	// the migration's lock request at once.
	MaxQueuedSessions *int `yaml:"max_queued_sessions" json:"max_queued_sessions,omitempty"`
	// MaxExtraWALMB bounds the WAL a candidate phase writes beyond its
	// paired baseline, a proxy for replication lag.
	MaxExtraWALMB *float64 `yaml:"max_extra_wal_mb" json:"max_extra_wal_mb,omitempty"`

	// Rules overrides the severity of individual rules, or disables them
	// with "off". Keys are rule IDs such as "PS101".
	Rules map[string]string `yaml:"rules" json:"rules,omitempty"`
	// Suppressions points at a file of audited suppressions.
	Suppressions string `yaml:"suppressions" json:"suppressions,omitempty"`
}

// ReportConfig selects artifact output.
type ReportConfig struct {
	Directory string   `yaml:"directory" json:"directory"`
	Formats   []string `yaml:"formats" json:"formats"`
}

// PrivacyConfig controls what may be persisted in artifacts.
type PrivacyConfig struct {
	PersistRawQueryText    bool     `yaml:"persist_raw_query_text" json:"persist_raw_query_text"`
	PersistQueryParameters bool     `yaml:"persist_query_parameters" json:"persist_query_parameters"`
	RedactPatterns         []string `yaml:"redact_patterns" json:"redact_patterns,omitempty"`
}

// TelemetryConfig controls server-side sampling.
type TelemetryConfig struct {
	// SampleIntervalMS is the polling interval for pg_locks and
	// pg_stat_activity. Lock waits shorter than this may be missed.
	SampleIntervalMS int `yaml:"sample_interval_ms" json:"sample_interval_ms"`
}
