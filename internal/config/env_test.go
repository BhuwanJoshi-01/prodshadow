package config

import (
	"slices"
	"testing"
)

func mapLookup(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

func TestApplyEnvOverrides(t *testing.T) {
	var c Config
	c.ApplyDefaults()
	applied, ps := c.ApplyEnv(mapLookup(map[string]string{
		"PRODSHADOW_ISOLATION_PROVIDER":           "podman",
		"PRODSHADOW_WORKLOAD_REPEATS":             "5",
		"PRODSHADOW_WORKLOAD_MEASUREMENT_SECONDS": "12.5",
		"PRODSHADOW_REPORT_FORMATS":               "json, html,,",
		"PRODSHADOW_UNRELATED":                    "ignored",
	}))
	if len(ps) != 0 {
		t.Fatalf("unexpected problems: %v", ps)
	}
	if c.Postgres.Isolation.Provider != "podman" || c.Workload.Repeats != 5 || c.Workload.MeasurementSeconds != 12.5 {
		t.Fatalf("overrides not applied: %+v", c)
	}
	if !slices.Equal(c.Report.Formats, []string{"json", "html"}) {
		t.Fatalf("formats = %v", c.Report.Formats)
	}
	if len(applied) != 4 {
		t.Fatalf("applied = %v", applied)
	}
}

func TestApplyEnvRejectsBadNumbers(t *testing.T) {
	var c Config
	c.ApplyDefaults()
	_, ps := c.ApplyEnv(mapLookup(map[string]string{"PRODSHADOW_WORKLOAD_CONCURRENCY": "many"}))
	if !ps.HasErrors() || ps[0].Path != "workload.concurrency" {
		t.Fatalf("expected error at workload.concurrency, got %v", ps)
	}
	if c.Workload.Concurrency != DefaultConcurrency {
		t.Fatal("invalid override must not change the value")
	}
}

func TestEnvOverridesNeverTouchPolicy(t *testing.T) {
	for _, n := range EnvOverrideNames() {
		if slices.Contains([]string{"PRODSHADOW_POLICY_FAIL_ON", "PRODSHADOW_POLICY_INCONCLUSIVE"}, n) {
			t.Fatalf("policy must not be overridable from the environment: %s", n)
		}
	}
}
