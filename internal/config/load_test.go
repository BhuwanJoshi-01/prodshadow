package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleConfig = `version: 1
project: example-api
postgres:
  version: "17"
  image: "postgres:17"
  database: app_test
  isolation:
    provider: docker
    network: none
    memory_mb: 2048
    cpus: 2
    ephemeral_disk_mb: 4096
fixture:
  schema: ./fixtures/schema.sql
  seed_sql: ./fixtures/seed.sql
  generator:
    path: ./fixtures/generate.yaml
    seed: 90210
workload:
  file: ./workloads/api.yaml
  warmup_seconds: 10
  measurement_seconds: 45
  concurrency: 16
  repeats: 3
migration:
  files: [./migrations/0042_add_index.sql]
  lock_timeout_ms: 1500
  statement_timeout_ms: 60000
policy:
  fail_on: [critical]
  max_error_rate_increase_pct_points: 1
  max_p95_latency_ratio: 1.5
  max_lock_wait_ms: 2000
report:
  directory: ./prodshadow-results
  formats: [json, markdown]
privacy:
  persist_raw_query_text: false
  persist_query_parameters: false
`

func TestParseBlueprintExample(t *testing.T) {
	cfg, problems, err := Parse([]byte(sampleConfig), "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 0 {
		t.Fatalf("unexpected problems: %v", problems)
	}
	if cfg.Project != "example-api" || cfg.Fixture.Generator.Seed != 90210 {
		t.Fatalf("decoded config mismatch: %+v", cfg)
	}
	if got := cfg.Resolve(cfg.Migration.Files[0]); got != "/repo/migrations/0042_add_index.sql" {
		t.Fatalf("resolve = %s", got)
	}
	if cfg.LineOf("workload.concurrency") != 23 {
		t.Fatalf("line = %d", cfg.LineOf("workload.concurrency"))
	}
}

func TestParseTypeMismatchIsProblem(t *testing.T) {
	_, problems, err := Parse([]byte("workload:\n  concurrency: lots\n"), "/")
	if err != nil {
		t.Fatal(err)
	}
	if !problems.HasErrors() {
		t.Fatal("expected a type error problem")
	}
	if problems[0].Line != 2 {
		t.Fatalf("line = %d (%v)", problems[0].Line, problems)
	}
}

func TestParseRejectsEmptyAndScalar(t *testing.T) {
	if _, _, err := Parse([]byte(""), "/"); !errors.Is(err, ErrEmpty) {
		t.Fatalf("empty: %v", err)
	}
	if _, _, err := Parse([]byte("just a string"), "/"); err == nil || !strings.Contains(err.Error(), "mapping") {
		t.Fatalf("scalar: %v", err)
	}
	if _, _, err := Parse([]byte("a: [unterminated"), "/"); err == nil {
		t.Fatal("expected syntax error")
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prodshadow.yaml")
	if err := os.WriteFile(path, []byte(sampleConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Path() != path || cfg.BaseDir() != dir {
		t.Fatalf("path=%s base=%s", cfg.Path(), cfg.BaseDir())
	}
	if _, _, err := Load(dir); err == nil {
		t.Fatal("loading a directory must fail")
	}
	if _, _, err := Load(filepath.Join(dir, "missing.yaml")); !os.IsNotExist(err) {
		t.Fatalf("missing file error = %v", err)
	}
}

func TestResolveKeepsAbsolutePaths(t *testing.T) {
	c := &Config{baseDir: "/base"}
	if c.Resolve("/abs/file.sql") != "/abs/file.sql" {
		t.Fatal("absolute path changed")
	}
	if c.Resolve("") != "" {
		t.Fatal("empty path changed")
	}
	if c.Resolve("../x.sql") != "/x.sql" {
		t.Fatalf("got %s", c.Resolve("../x.sql"))
	}
}
