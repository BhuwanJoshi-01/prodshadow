package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
)

// hostOS is the operating system validation assumes; tests override it.
var hostOS = runtime.GOOS

// SupportedPostgresMajors is the compatibility matrix exercised in CI.
var SupportedPostgresMajors = []string{"15", "16", "17", "18"}

var (
	projectRe    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	identifierRe = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)
	gucRe        = regexp.MustCompile(`^[a-z_][a-z0-9_]*(\.[a-z_][a-z0-9_]*)?$`)
	majorRe      = regexp.MustCompile(`^[0-9]{2}$`)
	ruleIDRe     = regexp.MustCompile(`^[A-Z]{2,5}[0-9]{3}$`)
)

// unsafeExtensions can reach the network, the host filesystem or run
// untrusted procedural code; they are refused in disposable fixtures.
var unsafeExtensions = map[string]string{
	"dblink":       "opens outbound database connections",
	"postgres_fdw": "opens outbound database connections",
	"file_fdw":     "reads server-side files",
	"adminpack":    "writes server-side files",
	"plpython3u":   "runs untrusted Python",
	"plperlu":      "runs untrusted Perl",
	"pltclu":       "runs untrusted Tcl",
	"plv8":         "runs arbitrary JavaScript",
	"pg_net":       "performs outbound HTTP requests",
	"http":         "performs outbound HTTP requests",
}

// reservedSettings are controlled by the isolation manager or execute shell
// commands, so they cannot be set through configuration.
var reservedSettings = map[string]string{
	"listen_addresses":          "managed by the isolation provider",
	"port":                      "managed by the isolation provider",
	"unix_socket_directories":   "managed by the isolation provider",
	"unix_socket_permissions":   "managed by the isolation provider",
	"unix_socket_group":         "managed by the isolation provider",
	"hba_file":                  "managed by the isolation provider",
	"ident_file":                "managed by the isolation provider",
	"data_directory":            "managed by the isolation provider",
	"config_file":               "managed by the isolation provider",
	"archive_command":           "executes shell commands",
	"archive_library":           "loads arbitrary code",
	"restore_command":           "executes shell commands",
	"archive_cleanup_command":   "executes shell commands",
	"recovery_end_command":      "executes shell commands",
	"ssl_passphrase_command":    "executes shell commands",
	"local_preload_libraries":   "loads arbitrary code",
	"session_preload_libraries": "loads arbitrary code",
	"dynamic_library_path":      "loads arbitrary code",
	"prodshadow.run_token":      "reserved for the ownership marker",
}

var (
	validProviders      = []string{"docker", "podman", "local"}
	validNetworks       = []string{"none", "loopback"}
	validRepresentation = []string{"manual-fixture", "synthetic"}
	validRunAs          = []string{"owner", "superuser"}
	validTransaction    = []string{"auto", "per-file", "none"}
	validSeverities     = []string{"info", "warning", "critical"}
	validInconclusive   = []string{"fail", "pass"}
	validFormats        = []string{"json", "markdown", "html", "sarif", "junit"}
	validRuleSetting    = []string{"off", "info", "warning", "critical"}
)

// Validate checks semantic constraints and referenced files. It returns
// all problems found rather than stopping at the first one.
func (c *Config) Validate() Problems {
	var ps Problems
	c.validateRoot(&ps)
	c.validatePostgres(&ps)
	c.validateFixture(&ps)
	c.validateWorkload(&ps)
	c.validateMigration(&ps)
	c.validatePolicy(&ps)
	c.validateReport(&ps)
	c.validatePrivacy(&ps)
	c.validateTelemetry(&ps)
	for i := range ps {
		if ps[i].Line == 0 {
			ps[i].Line = c.LineOf(ps[i].Path)
		}
	}
	return ps
}

func (c *Config) validateRoot(ps *Problems) {
	if c.Version != CurrentVersion {
		ps.errorf("version", "unsupported configuration version %d (this build understands %d)", c.Version, CurrentVersion)
	}
	if c.Project == "" {
		ps.errorf("project", "project name is required")
	} else if !projectRe.MatchString(c.Project) {
		ps.errorf("project", "project %q must be 1-64 characters of letters, digits, '.', '_' or '-'", c.Project)
	}
}

func (c *Config) validatePostgres(ps *Problems) {
	pg := c.Postgres
	switch {
	case !majorRe.MatchString(pg.Version):
		ps.errorf("postgres.version", "version %q must be a PostgreSQL major version such as \"17\"", pg.Version)
	case pg.Version < "13":
		ps.errorf("postgres.version", "PostgreSQL %s is end-of-life and not supported", pg.Version)
	case !slices.Contains(SupportedPostgresMajors, pg.Version):
		ps.warnf("postgres.version", "PostgreSQL %s is outside the tested matrix %v; results are best effort", pg.Version, SupportedPostgresMajors)
	}

	if pg.Image != "" {
		if strings.ContainsAny(pg.Image, " \t\n") {
			ps.errorf("postgres.image", "image reference must not contain whitespace")
		} else if !strings.Contains(pg.Image, "@sha256:") {
			ps.warnf("postgres.image", "image %q is not pinned by digest; pin it in CI for reproducible results", pg.Image)
		}
		if tag := imageTagMajor(pg.Image); tag != "" && majorRe.MatchString(pg.Version) && tag != pg.Version {
			ps.warnf("postgres.image", "image tag major %s differs from postgres.version %s", tag, pg.Version)
		}
	}

	if !identifierRe.MatchString(pg.Database) {
		ps.errorf("postgres.database", "database %q must be a lower-case SQL identifier", pg.Database)
	}
	if strings.HasPrefix(pg.Database, "ps_") || pg.Database == "postgres" || strings.HasPrefix(pg.Database, "template") {
		ps.errorf("postgres.database", "database name %q is reserved", pg.Database)
	}

	for i, ext := range pg.Extensions {
		path := fmt.Sprintf("postgres.extensions[%d]", i)
		if !identifierRe.MatchString(ext) {
			ps.errorf(path, "extension %q is not a valid identifier", ext)
			continue
		}
		if why, bad := unsafeExtensions[ext]; bad {
			ps.errorf(path, "extension %q %s and is not permitted in a disposable fixture", ext, why)
		}
	}

	for key := range pg.Settings {
		path := "postgres.settings." + key
		if !gucRe.MatchString(key) {
			ps.errorf(path, "%q is not a valid server setting name", key)
			continue
		}
		if why, reserved := reservedSettings[key]; reserved {
			ps.errorf(path, "setting %q cannot be configured: %s", key, why)
		}
		if strings.ContainsFunc(pg.Settings[key], func(r rune) bool { return r < 0x20 || r == 0x7f }) {
			ps.errorf(path, "setting values must not contain control characters")
		}
	}

	iso := pg.Isolation
	if !slices.Contains(validProviders, iso.Provider) {
		ps.errorf("postgres.isolation.provider", "provider %q must be one of %v", iso.Provider, validProviders)
	}
	if !slices.Contains(validNetworks, iso.Network) {
		ps.errorf("postgres.isolation.network", "network %q must be one of %v", iso.Network, validNetworks)
	}
	if iso.MemoryMB < 256 || iso.MemoryMB > 262144 {
		ps.errorf("postgres.isolation.memory_mb", "memory_mb %d must be between 256 and 262144", iso.MemoryMB)
	}
	if iso.CPUs <= 0 || iso.CPUs > 128 {
		ps.errorf("postgres.isolation.cpus", "cpus %g must be greater than 0 and at most 128", iso.CPUs)
	}
	if iso.EphemeralDiskMB < 256 {
		ps.errorf("postgres.isolation.ephemeral_disk_mb", "ephemeral_disk_mb %d must be at least 256", iso.EphemeralDiskMB)
	}
	if iso.PidsLimit < 64 {
		ps.errorf("postgres.isolation.pids_limit", "pids_limit %d must be at least 64", iso.PidsLimit)
	}
	if iso.StartupTimeoutSeconds < 5 || iso.StartupTimeoutSeconds > 1800 {
		ps.errorf("postgres.isolation.startup_timeout_seconds", "startup_timeout_seconds %d must be between 5 and 1800", iso.StartupTimeoutSeconds)
	}
	if iso.Provider == "local" {
		ps.warnf("postgres.isolation.provider", "the local provider cannot enforce memory, CPU or disk caps; prefer docker or podman in CI")
	}
	if iso.Provider != "local" && iso.Network == "none" && hostOS != "linux" {
		ps.warnf("postgres.isolation.network", "network \"none\" shares a unix socket with the container and only works on Linux; use \"loopback\" on %s", hostOS)
	}
}

// imageTagMajor extracts a leading major version from an image tag such as
// "postgres:17.2-alpine". It returns "" when the tag does not start with one.
func imageTagMajor(image string) string {
	ref := image
	if at := strings.Index(ref, "@"); at >= 0 {
		ref = ref[:at]
	}
	colon := strings.LastIndex(ref, ":")
	if colon < 0 || strings.Contains(ref[colon:], "/") {
		return ""
	}
	tag := ref[colon+1:]
	end := 0
	for end < len(tag) && tag[end] >= '0' && tag[end] <= '9' {
		end++
	}
	if end == 0 {
		return ""
	}
	return tag[:end]
}

func (c *Config) validateFixture(ps *Problems) {
	f := c.Fixture
	if f.Schema == "" && f.SeedSQL == "" && f.Generator == nil {
		ps.warnf("fixture", "no schema, seed or generator configured; verification will be inconclusive")
	}
	c.checkFile(ps, "fixture.schema", f.Schema)
	c.checkFile(ps, "fixture.seed_sql", f.SeedSQL)
	if f.Generator != nil {
		if f.Generator.Path == "" {
			ps.errorf("fixture.generator.path", "generator path is required when generator is set")
		}
		c.checkFile(ps, "fixture.generator.path", f.Generator.Path)
	}
	if !slices.Contains(validRepresentation, f.Representation) {
		ps.errorf("fixture.representation", "representation %q must be one of %v", f.Representation, validRepresentation)
	}
	if f.MaxRows < 0 {
		ps.errorf("fixture.max_rows", "max_rows must not be negative")
	}
}

func (c *Config) validateWorkload(ps *Problems) {
	w := c.Workload
	if w.File == "" {
		ps.warnf("workload.file", "no workload configured; verification will be inconclusive")
	}
	c.checkFile(ps, "workload.file", w.File)
	if w.WarmupSeconds < 0 || w.WarmupSeconds > 3600 {
		ps.errorf("workload.warmup_seconds", "warmup_seconds %g must be between 0 and 3600", w.WarmupSeconds)
	}
	if w.MeasurementSeconds < 5 || w.MeasurementSeconds > 3600 {
		ps.errorf("workload.measurement_seconds", "measurement_seconds %g must be between 5 and 3600", w.MeasurementSeconds)
	}
	if w.Concurrency < 1 || w.Concurrency > 512 {
		ps.errorf("workload.concurrency", "concurrency %d must be between 1 and 512", w.Concurrency)
	}
	if w.ArrivalRate < 0 {
		ps.errorf("workload.arrival_rate", "arrival_rate must not be negative")
	}
	if w.Repeats < 1 || w.Repeats > 50 {
		ps.errorf("workload.repeats", "repeats %d must be between 1 and 50", w.Repeats)
	} else if w.Repeats < 3 {
		ps.warnf("workload.repeats", "fewer than 3 repeats cannot estimate baseline noise; findings will carry low confidence")
	}
	if w.MinSamplesPerScenario < 1 {
		ps.errorf("workload.min_samples_per_scenario", "min_samples_per_scenario must be at least 1")
	}
}

func (c *Config) validateMigration(ps *Problems) {
	m := c.Migration
	for i, f := range m.Files {
		path := fmt.Sprintf("migration.files[%d]", i)
		if !strings.ContainsAny(f, "*?[") {
			c.checkFile(ps, path, f)
			continue
		}
		matches, err := filepath.Glob(c.Resolve(f))
		switch {
		case err != nil:
			ps.errorf(path, "invalid pattern %q: %v", f, err)
		case len(matches) == 0:
			ps.errorf(path, "pattern %q matches no files", f)
		}
	}
	if m.LockTimeoutMS < 1 || m.LockTimeoutMS > 600_000 {
		ps.errorf("migration.lock_timeout_ms", "lock_timeout_ms %d must be between 1 and 600000", m.LockTimeoutMS)
	}
	if m.StatementTimeoutMS < 1 || m.StatementTimeoutMS > 86_400_000 {
		ps.errorf("migration.statement_timeout_ms", "statement_timeout_ms %d must be between 1 and 86400000", m.StatementTimeoutMS)
	}
	if m.StatementTimeoutMS < m.LockTimeoutMS {
		ps.warnf("migration.statement_timeout_ms", "statement_timeout_ms is shorter than lock_timeout_ms; lock waits will surface as statement timeouts")
	}
	if m.LockRetries < 0 || m.LockRetries > 20 {
		ps.errorf("migration.lock_retries", "lock_retries %d must be between 0 and 20", m.LockRetries)
	}
	if m.RetryBackoffMS < 0 || m.RetryBackoffMS > 60_000 {
		ps.errorf("migration.retry_backoff_ms", "retry_backoff_ms %d must be between 0 and 60000", m.RetryBackoffMS)
	}
	if m.StartAfterSeconds < 0 {
		ps.errorf("migration.start_after_seconds", "start_after_seconds must not be negative")
	} else if c.Workload.MeasurementSeconds > 0 && m.StartAfterSeconds >= c.Workload.MeasurementSeconds {
		ps.errorf("migration.start_after_seconds", "start_after_seconds %g must be shorter than measurement_seconds %g", m.StartAfterSeconds, c.Workload.MeasurementSeconds)
	}
	if !slices.Contains(validRunAs, m.RunAs) {
		ps.errorf("migration.run_as", "run_as %q must be one of %v", m.RunAs, validRunAs)
	}
	if !slices.Contains(validTransaction, m.Transaction) {
		ps.errorf("migration.transaction", "transaction %q must be one of %v", m.Transaction, validTransaction)
	}
}

func (c *Config) validatePolicy(ps *Problems) {
	p := c.Policy
	for i, s := range p.FailOn {
		if !slices.Contains(validSeverities, s) {
			ps.errorf(fmt.Sprintf("policy.fail_on[%d]", i), "severity %q must be one of %v", s, validSeverities)
		}
	}
	if !slices.Contains(validInconclusive, p.Inconclusive) {
		ps.errorf("policy.inconclusive", "inconclusive %q must be one of %v", p.Inconclusive, validInconclusive)
	} else if p.Inconclusive == "pass" {
		ps.warnf("policy.inconclusive", "inconclusive results will not fail CI; they are still reported")
	}
	nonNegative(ps, "policy.max_error_rate_increase_pct_points", p.MaxErrorRateIncreasePctPoints)
	nonNegative(ps, "policy.max_lock_wait_ms", p.MaxLockWaitMS)
	nonNegative(ps, "policy.max_migration_seconds", p.MaxMigrationSeconds)
	nonNegative(ps, "policy.max_extra_wal_mb", p.MaxExtraWALMB)
	if v := p.MaxQueuedSessions; v != nil && *v < 0 {
		ps.errorf("policy.max_queued_sessions", "must not be negative")
	}
	atLeastOne(ps, "policy.max_p95_latency_ratio", p.MaxP95LatencyRatio)
	atLeastOne(ps, "policy.max_p99_latency_ratio", p.MaxP99LatencyRatio)
	if v := p.MaxThroughputDropPct; v != nil && (*v < 0 || *v > 100) {
		ps.errorf("policy.max_throughput_drop_pct", "max_throughput_drop_pct %g must be between 0 and 100", *v)
	}
	for id, setting := range p.Rules {
		path := "policy.rules." + id
		if !ruleIDRe.MatchString(id) {
			ps.errorf(path, "%q is not a rule identifier such as PS101", id)
		}
		if !slices.Contains(validRuleSetting, setting) {
			ps.errorf(path, "rule setting %q must be one of %v", setting, validRuleSetting)
		}
	}
	c.checkFile(ps, "policy.suppressions", p.Suppressions)
}

func nonNegative(ps *Problems, path string, v *float64) {
	if v != nil && *v < 0 {
		ps.errorf(path, "must not be negative")
	}
}

func atLeastOne(ps *Problems, path string, v *float64) {
	if v != nil && *v < 1 {
		ps.errorf(path, "ratio %g must be at least 1", *v)
	}
}

func (c *Config) validateReport(ps *Problems) {
	if strings.TrimSpace(c.Report.Directory) == "" {
		ps.errorf("report.directory", "report directory is required")
	}
	seen := map[string]bool{}
	for i, f := range c.Report.Formats {
		path := fmt.Sprintf("report.formats[%d]", i)
		if !slices.Contains(validFormats, f) {
			ps.errorf(path, "format %q must be one of %v", f, validFormats)
		}
		if seen[f] {
			ps.warnf(path, "format %q is listed more than once", f)
		}
		seen[f] = true
	}
}

func (c *Config) validatePrivacy(ps *Problems) {
	if c.Privacy.PersistRawQueryText {
		ps.warnf("privacy.persist_raw_query_text", "raw query text will be written to artifacts")
	}
	if c.Privacy.PersistQueryParameters {
		ps.warnf("privacy.persist_query_parameters", "query parameters will be written to artifacts")
	}
	for i, pat := range c.Privacy.RedactPatterns {
		if _, err := regexp.Compile(pat); err != nil {
			ps.errorf(fmt.Sprintf("privacy.redact_patterns[%d]", i), "invalid regular expression: %v", err)
		}
	}
}

func (c *Config) validateTelemetry(ps *Problems) {
	if v := c.Telemetry.SampleIntervalMS; v < 10 || v > 5000 {
		ps.errorf("telemetry.sample_interval_ms", "sample_interval_ms %d must be between 10 and 5000", v)
	}
}

// checkFile verifies that an optional file reference exists and is a
// regular file.
func (c *Config) checkFile(ps *Problems, path, ref string) {
	if ref == "" {
		return
	}
	fi, err := os.Stat(c.Resolve(ref))
	switch {
	case err != nil:
		ps.errorf(path, "cannot read %s: %v", ref, unwrapPathError(err))
	case !fi.Mode().IsRegular():
		ps.errorf(path, "%s is not a regular file", ref)
	}
}

func unwrapPathError(err error) error {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}
