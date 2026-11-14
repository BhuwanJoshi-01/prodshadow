package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// validConfig returns a configuration that passes validation with all
// referenced files created in a temporary directory.
func validConfig(t *testing.T) *Config {
	t.Helper()
	// Validation of network: none depends on the host OS; pin it so the
	// tests behave the same on every CI platform.
	old := hostOS
	hostOS = "linux"
	t.Cleanup(func() { hostOS = old })
	dir := t.TempDir()
	for _, f := range []string{"schema.sql", "seed.sql", "workload.yaml", "m1.sql"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("-- test\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	c := &Config{
		baseDir: dir,
		Project: "demo",
		Postgres: PostgresConfig{
			Version: "17",
			Image:   "postgres:17-alpine@sha256:" + strings.Repeat("a", 64),
		},
		Fixture:   FixtureConfig{Schema: "schema.sql", SeedSQL: "seed.sql"},
		Workload:  WorkloadConfig{File: "workload.yaml"},
		Migration: MigrationConfig{Files: []string{"m1.sql"}},
	}
	c.ApplyDefaults()
	return c
}

func problemAt(ps Problems, path string) *Problem {
	for i := range ps {
		if ps[i].Path == path {
			return &ps[i]
		}
	}
	return nil
}

func TestValidConfigHasNoProblems(t *testing.T) {
	c := validConfig(t)
	if ps := c.Validate(); len(ps) != 0 {
		t.Fatalf("unexpected problems:\n%v", ps)
	}
}

func TestValidateReportsEachInvalidField(t *testing.T) {
	cases := []struct {
		name  string
		mut   func(*Config)
		path  string
		level Level
	}{
		{"version", func(c *Config) { c.Version = 2 }, "version", LevelError},
		{"project missing", func(c *Config) { c.Project = "" }, "project", LevelError},
		{"project chars", func(c *Config) { c.Project = "bad name" }, "project", LevelError},
		{"pg major format", func(c *Config) { c.Postgres.Version = "17.2" }, "postgres.version", LevelError},
		{"pg eol", func(c *Config) { c.Postgres.Version = "11" }, "postgres.version", LevelError},
		{"pg outside matrix", func(c *Config) { c.Postgres.Version = "14" }, "postgres.version", LevelWarning},
		{"unpinned image", func(c *Config) { c.Postgres.Image = "postgres:17" }, "postgres.image", LevelWarning},
		{"database ident", func(c *Config) { c.Postgres.Database = "App-DB" }, "postgres.database", LevelError},
		{"database reserved", func(c *Config) { c.Postgres.Database = "postgres" }, "postgres.database", LevelError},
		{"unsafe extension", func(c *Config) { c.Postgres.Extensions = []string{"dblink"} }, "postgres.extensions[0]", LevelError},
		{"reserved setting", func(c *Config) { c.Postgres.Settings = map[string]string{"archive_command": "x"} }, "postgres.settings.archive_command", LevelError},
		{"control chars", func(c *Config) { c.Postgres.Settings = map[string]string{"work_mem": "4MB\nfsync=off"} }, "postgres.settings.work_mem", LevelError},
		{"provider", func(c *Config) { c.Postgres.Isolation.Provider = "kubernetes" }, "postgres.isolation.provider", LevelError},
		{"local provider", func(c *Config) { c.Postgres.Isolation.Provider = "local" }, "postgres.isolation.provider", LevelWarning},
		{"network", func(c *Config) { c.Postgres.Isolation.Network = "host" }, "postgres.isolation.network", LevelError},
		{"memory", func(c *Config) { c.Postgres.Isolation.MemoryMB = 64 }, "postgres.isolation.memory_mb", LevelError},
		{"no fixture", func(c *Config) { c.Fixture = FixtureConfig{Representation: "manual-fixture"} }, "fixture", LevelWarning},
		{"missing schema", func(c *Config) { c.Fixture.Schema = "nope.sql" }, "fixture.schema", LevelError},
		{"representation", func(c *Config) { c.Fixture.Representation = "production" }, "fixture.representation", LevelError},
		{"no workload", func(c *Config) { c.Workload.File = "" }, "workload.file", LevelWarning},
		{"measurement", func(c *Config) { c.Workload.MeasurementSeconds = 1 }, "workload.measurement_seconds", LevelError},
		{"concurrency", func(c *Config) { c.Workload.Concurrency = 1000 }, "workload.concurrency", LevelError},
		{"few repeats", func(c *Config) { c.Workload.Repeats = 1 }, "workload.repeats", LevelWarning},
		{"lock timeout", func(c *Config) { c.Migration.LockTimeoutMS = -1 }, "migration.lock_timeout_ms", LevelError},
		{"start after", func(c *Config) { c.Migration.StartAfterSeconds = 999 }, "migration.start_after_seconds", LevelError},
		{"run as", func(c *Config) { c.Migration.RunAs = "root" }, "migration.run_as", LevelError},
		{"fail on", func(c *Config) { c.Policy.FailOn = []string{"fatal"} }, "policy.fail_on[0]", LevelError},
		{"inconclusive pass", func(c *Config) { c.Policy.Inconclusive = "pass" }, "policy.inconclusive", LevelWarning},
		{"ratio", func(c *Config) { v := 0.5; c.Policy.MaxP95LatencyRatio = &v }, "policy.max_p95_latency_ratio", LevelError},
		{"rule id", func(c *Config) { c.Policy.Rules = map[string]string{"bad": "off"} }, "policy.rules.bad", LevelError},
		{"format", func(c *Config) { c.Report.Formats = []string{"pdf"} }, "report.formats[0]", LevelError},
		{"raw text", func(c *Config) { c.Privacy.PersistRawQueryText = true }, "privacy.persist_raw_query_text", LevelWarning},
		{"redact regex", func(c *Config) { c.Privacy.RedactPatterns = []string{"("} }, "privacy.redact_patterns[0]", LevelError},
		{"sample interval", func(c *Config) { c.Telemetry.SampleIntervalMS = 1 }, "telemetry.sample_interval_ms", LevelError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := validConfig(t)
			tc.mut(c)
			p := problemAt(c.Validate(), tc.path)
			if p == nil {
				t.Fatalf("no problem reported at %s; got %v", tc.path, c.Validate())
			}
			if p.Level != tc.level {
				t.Fatalf("level = %s, want %s (%s)", p.Level, tc.level, p.Message)
			}
		})
	}
}

func TestImageTagMajor(t *testing.T) {
	cases := map[string]string{
		"postgres:17":                     "17",
		"postgres:16.4-alpine":            "16",
		"postgres":                        "",
		"postgres:latest":                 "",
		"registry:5000/postgres":          "",
		"registry:5000/postgres:15":       "15",
		"postgres:18@sha256:" + "ab":      "18",
		"ghcr.io/acme/pg@sha256:abcdef01": "",
	}
	for in, want := range cases {
		if got := imageTagMajor(in); got != want {
			t.Errorf("imageTagMajor(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateAttachesLineNumbers(t *testing.T) {
	cfg, _, err := Parse([]byte("version: 1\nproject: demo\nworkload:\n  concurrency: 9999\n"), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p := problemAt(cfg.Validate(), "workload.concurrency")
	if p == nil || p.Line != 4 {
		t.Fatalf("expected line 4, got %+v", p)
	}
}

func TestNetworkNoneWarnsOutsideLinux(t *testing.T) {
	c := validConfig(t)
	hostOS = "darwin"
	p := problemAt(c.Validate(), "postgres.isolation.network")
	if p == nil || p.Level != LevelWarning {
		t.Fatalf("expected a warning on darwin, got %+v", p)
	}
	hostOS = "linux"
	if p := problemAt(c.Validate(), "postgres.isolation.network"); p != nil {
		t.Fatalf("no warning expected on linux: %+v", p)
	}
}

func TestMigrationGlobs(t *testing.T) {
	c := validConfig(t)
	c.Migration.Files = []string{"m*.sql"}
	if p := problemAt(c.Validate(), "migration.files[0]"); p != nil {
		t.Fatalf("matching glob reported: %+v", p)
	}
	c.Migration.Files = []string{"migrations/*.sql"}
	if p := problemAt(c.Validate(), "migration.files[0]"); p == nil || !strings.Contains(p.Message, "matches no files") {
		t.Fatalf("empty glob not reported: %+v", p)
	}
	c.Migration.Files = []string{"[.sql"}
	if p := problemAt(c.Validate(), "migration.files[0]"); p == nil {
		t.Fatal("malformed pattern not reported")
	}
}

func TestValidateLockRetries(t *testing.T) {
	c := validConfig(t)
	c.Migration.LockRetries, c.Migration.RetryBackoffMS = 21, -1
	ps := c.Validate()
	for _, path := range []string{"migration.lock_retries", "migration.retry_backoff_ms"} {
		if problemAt(ps, path) == nil {
			t.Errorf("expected a problem at %s: %v", path, ps)
		}
	}
}
