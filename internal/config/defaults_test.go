package config

import "testing"

func TestApplyDefaultsOnEmptyConfig(t *testing.T) {
	var c Config
	c.ApplyDefaults()

	if c.Version != CurrentVersion {
		t.Errorf("version = %d", c.Version)
	}
	if c.Postgres.Image != "postgres:17-alpine" {
		t.Errorf("image = %q", c.Postgres.Image)
	}
	if c.Postgres.Isolation.Network != "none" {
		t.Errorf("network = %q", c.Postgres.Isolation.Network)
	}
	if c.Workload.Repeats != 3 || c.Workload.Concurrency != 16 {
		t.Errorf("workload defaults = %+v", c.Workload)
	}
	if got := c.Policy.FailOn; len(got) != 1 || got[0] != "critical" {
		t.Errorf("fail_on = %v", got)
	}
	if c.Policy.MaxLockWaitMS == nil || *c.Policy.MaxLockWaitMS != 2000 {
		t.Errorf("max lock wait default missing")
	}
	if c.Report.Formats[0] != "json" {
		t.Errorf("formats = %v", c.Report.Formats)
	}
}

func TestApplyDefaultsKeepsExplicitValues(t *testing.T) {
	zero := 0.0
	c := Config{
		Postgres: PostgresConfig{Version: "15"},
		Workload: WorkloadConfig{Repeats: 5},
		Policy:   PolicyConfig{MaxErrorRateIncreasePctPoints: &zero},
	}
	c.ApplyDefaults()

	if c.Postgres.Image != "postgres:15-alpine" {
		t.Errorf("image should follow version, got %q", c.Postgres.Image)
	}
	if c.Workload.Repeats != 5 {
		t.Errorf("repeats overwritten: %d", c.Workload.Repeats)
	}
	if *c.Policy.MaxErrorRateIncreasePctPoints != 0 {
		t.Errorf("explicit zero threshold must be preserved")
	}
}

func TestApplyDefaultsIsIdempotent(t *testing.T) {
	var a, b Config
	a.ApplyDefaults()
	b.ApplyDefaults()
	b.ApplyDefaults()
	if a.Postgres.Image != b.Postgres.Image || a.Postgres.Isolation != b.Postgres.Isolation {
		t.Fatal("defaults changed on second application")
	}
	if *a.Policy.MaxP95LatencyRatio != *b.Policy.MaxP95LatencyRatio {
		t.Fatal("policy defaults changed on second application")
	}
}
