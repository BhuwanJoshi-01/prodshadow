package config

import (
	"fmt"
	"strconv"
	"strings"
)

// EnvPrefix prefixes every supported environment override.
const EnvPrefix = "PRODSHADOW_"

// envOverride maps one environment variable to one configuration field.
// The list is explicit on purpose: only operational knobs that commonly
// differ between a laptop and CI can be overridden, never policy.
type envOverride struct {
	name  string
	path  string
	apply func(c *Config, v string) error
}

var envOverrides = []envOverride{
	{"POSTGRES_IMAGE", "postgres.image", func(c *Config, v string) error { c.Postgres.Image = v; return nil }},
	{"ISOLATION_PROVIDER", "postgres.isolation.provider", func(c *Config, v string) error { c.Postgres.Isolation.Provider = v; return nil }},
	{"ISOLATION_NETWORK", "postgres.isolation.network", func(c *Config, v string) error { c.Postgres.Isolation.Network = v; return nil }},
	{"ISOLATION_BINARY", "postgres.isolation.binary", func(c *Config, v string) error { c.Postgres.Isolation.Binary = v; return nil }},
	{"WORKLOAD_REPEATS", "workload.repeats", intSetter(func(c *Config, n int) { c.Workload.Repeats = n })},
	{"WORKLOAD_CONCURRENCY", "workload.concurrency", intSetter(func(c *Config, n int) { c.Workload.Concurrency = n })},
	{"WORKLOAD_MEASUREMENT_SECONDS", "workload.measurement_seconds", floatSetter(func(c *Config, f float64) { c.Workload.MeasurementSeconds = f })},
	{"WORKLOAD_WARMUP_SECONDS", "workload.warmup_seconds", floatSetter(func(c *Config, f float64) { c.Workload.WarmupSeconds = f })},
	{"REPORT_DIRECTORY", "report.directory", func(c *Config, v string) error { c.Report.Directory = v; return nil }},
	{"REPORT_FORMATS", "report.formats", func(c *Config, v string) error { c.Report.Formats = splitList(v); return nil }},
}

func intSetter(set func(*Config, int)) func(*Config, string) error {
	return func(c *Config, v string) error {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return fmt.Errorf("expected an integer, got %q", v)
		}
		set(c, n)
		return nil
	}
}

func floatSetter(set func(*Config, float64)) func(*Config, string) error {
	return func(c *Config, v string) error {
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return fmt.Errorf("expected a number, got %q", v)
		}
		set(c, f)
		return nil
	}
}

func splitList(v string) []string {
	var out []string
	for _, part := range strings.Split(v, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ApplyEnv applies supported PRODSHADOW_* overrides using lookup (normally
// os.LookupEnv). It returns the names that were applied, so they can be
// recorded in the report, and problems for unparsable values.
func (c *Config) ApplyEnv(lookup func(string) (string, bool)) ([]string, Problems) {
	var applied []string
	var ps Problems
	for _, o := range envOverrides {
		name := EnvPrefix + o.name
		v, ok := lookup(name)
		if !ok || v == "" {
			continue
		}
		if err := o.apply(c, v); err != nil {
			ps.errorf(o.path, "%s: %v", name, err)
			continue
		}
		applied = append(applied, name)
	}
	return applied, ps
}

// EnvOverrideNames lists every supported override variable.
func EnvOverrideNames() []string {
	names := make([]string, len(envOverrides))
	for i, o := range envOverrides {
		names[i] = EnvPrefix + o.name
	}
	return names
}
