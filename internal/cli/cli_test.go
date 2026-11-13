package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type result struct {
	code           int
	stdout, stderr string
}

func run(t *testing.T, dir string, env map[string]string, args ...string) result {
	t.Helper()
	var out, errOut bytes.Buffer
	code := Main(context.Background(), args, &Env{
		Stdout: &out, Stderr: &errOut, Dir: dir,
		Getenv: func(k string) (string, bool) { v, ok := env[k]; return v, ok },
	})
	return result{code, out.String(), errOut.String()}
}

func initProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if r := run(t, dir, nil, "init", "--project", "demo"); r.code != ExitOK {
		t.Fatalf("init failed: %+v", r)
	}
	return dir
}

func TestUsageAndUnknownCommand(t *testing.T) {
	if r := run(t, "", nil); r.code != ExitInvalid || !strings.Contains(r.stdout, "Commands:") {
		t.Fatalf("no-arg usage: %+v", r)
	}
	if r := run(t, "", nil, "--help"); r.code != ExitOK {
		t.Fatalf("--help: %+v", r)
	}
	if r := run(t, "", nil, "deploy"); r.code != ExitInvalid || !strings.Contains(r.stderr, "unknown command") {
		t.Fatalf("unknown command: %+v", r)
	}
	if r := run(t, "", nil, "validate", "-h"); r.code != ExitOK || !strings.Contains(r.stderr, "Usage: prodshadow validate") {
		t.Fatalf("command help: %+v", r)
	}
	if r := run(t, "", nil, "validate", "stray"); r.code != ExitInvalid {
		t.Fatalf("stray argument: %+v", r)
	}
}

func TestVersionAndSchemaAndRules(t *testing.T) {
	if r := run(t, "", nil, "version", "--json"); r.code != ExitOK || !strings.Contains(r.stdout, `"report_schema"`) {
		t.Fatalf("version: %+v", r)
	}
	r := run(t, "", nil, "schema", "report")
	var doc map[string]any
	if r.code != ExitOK || json.Unmarshal([]byte(r.stdout), &doc) != nil {
		t.Fatalf("schema: %+v", r)
	}
	if r := run(t, "", nil, "schema", "nope"); r.code != ExitInvalid {
		t.Fatalf("unknown schema: %+v", r)
	}
	r = run(t, "", nil, "rules")
	for _, id := range []string{"PS101", "OBS002", "QUAL001"} {
		if !strings.Contains(r.stdout, id) {
			t.Errorf("rules output lacks %s", id)
		}
	}
}

func TestInitRefusesToOverwrite(t *testing.T) {
	dir := initProject(t)
	for _, f := range []string{"prodshadow.yaml", "fixtures/schema.sql", "fixtures/seed.sql", "workloads/workload.yaml", "migrations/0001_add_order_note.sql"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing %s", f)
		}
	}
	if r := run(t, dir, nil, "init"); r.code != ExitInvalid || !strings.Contains(r.stderr, "already exists") {
		t.Fatalf("second init should refuse: %+v", r)
	}
	if r := run(t, dir, nil, "init", "--force", "--project", "demo"); r.code != ExitOK {
		t.Fatalf("--force should overwrite: %+v", r)
	}
	cfg, _ := os.ReadFile(filepath.Join(dir, "prodshadow.yaml"))
	if !strings.Contains(string(cfg), "project: demo") {
		t.Fatal("project name not templated")
	}
}

func TestValidate(t *testing.T) {
	dir := initProject(t)
	if r := run(t, dir, nil, "validate"); r.code != ExitOK || !strings.Contains(r.stdout, "is valid") {
		t.Fatalf("validate: %+v", r)
	}
	if r := run(t, dir, nil, "validate", "--strict"); r.code != ExitInvalid {
		t.Fatalf("strict mode should fail on the unpinned image warning: %+v", r)
	}
	os.WriteFile(filepath.Join(dir, "workloads/workload.yaml"), []byte("version: 1\nscenarios: []\n"), 0o644)
	r := run(t, dir, nil, "validate", "--json")
	if r.code != ExitInvalid || !strings.Contains(r.stdout, `"valid": false`) || !strings.Contains(r.stdout, "workload.file") {
		t.Fatalf("broken workload not reported: %+v", r)
	}
	if r := run(t, dir, nil, "validate", "--config", "missing.yaml"); r.code != ExitInvalid {
		t.Fatalf("missing config: %+v", r)
	}
}

func TestValidateRejectsEnvOverrideErrors(t *testing.T) {
	dir := initProject(t)
	r := run(t, dir, map[string]string{"PRODSHADOW_WORKLOAD_REPEATS": "many"}, "validate")
	if r.code != ExitInvalid || !strings.Contains(r.stdout, "PRODSHADOW_WORKLOAD_REPEATS") {
		t.Fatalf("bad env override: %+v", r)
	}
}

func TestInspect(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "m.sql"), []byte("CREATE INDEX idx ON orders (customer_id);\nALTER TABLE orders ADD COLUMN x int;\n"), 0o644)
	r := run(t, dir, nil, "inspect", "--migration", "m.sql")
	if r.code != ExitOK || !strings.Contains(r.stdout, "PS101") || !strings.Contains(r.stdout, "m.sql:1") {
		t.Fatalf("inspect text: %+v", r)
	}
	if r := run(t, dir, nil, "inspect", "--migration", "m.sql", "--fail-on", "warning"); r.code != ExitBlock {
		t.Fatalf("--fail-on warning should block: %+v", r)
	}
	if r := run(t, dir, nil, "inspect", "--migration", "m.sql", "--fail-on", "critical"); r.code != ExitOK {
		t.Fatalf("--fail-on critical should pass: %+v", r)
	}
	r = run(t, dir, nil, "inspect", "--migration", "m.sql", "--format", "json")
	var out struct {
		Statements []map[string]any `json:"statements"`
		Findings   []map[string]any `json:"findings"`
	}
	if r.code != ExitOK || json.Unmarshal([]byte(r.stdout), &out) != nil || len(out.Statements) != 2 {
		t.Fatalf("inspect json: %+v", r)
	}
	if strings.Contains(r.stdout, "CREATE INDEX idx") {
		t.Fatal("raw SQL must not appear in inspect JSON")
	}
	if r := run(t, dir, nil, "inspect", "--migration", "m.sql", "--format", "markdown"); !strings.Contains(r.stdout, "| PS101") && !strings.Contains(r.stdout, "PS101 create-index") {
		t.Fatalf("inspect markdown: %+v", r)
	}
	if r := run(t, dir, nil, "inspect"); r.code != ExitInvalid {
		t.Fatalf("no migrations should be invalid: %+v", r)
	}
	if r := run(t, dir, nil, "inspect", "--migration", "m.sql", "--transaction", "sometimes"); r.code != ExitInvalid {
		t.Fatalf("bad transaction mode: %+v", r)
	}
}

func TestInspectUsesConfigOverrides(t *testing.T) {
	dir := initProject(t)
	cfg := filepath.Join(dir, "prodshadow.yaml")
	raw, _ := os.ReadFile(cfg)
	os.WriteFile(cfg, []byte(strings.Replace(string(raw), "  fail_on: [critical]\n", "  fail_on: [critical]\n  rules:\n    PS114: off\n", 1)), 0o644)
	r := run(t, dir, nil, "inspect", "--config", "prodshadow.yaml")
	if r.code != ExitOK || strings.Contains(r.stdout, "PS114") {
		t.Fatalf("rule override not applied: %+v", r)
	}
}

func TestVerifyWithoutWorkloadIsInconclusive(t *testing.T) {
	dir := initProject(t)
	cfg := filepath.Join(dir, "prodshadow.yaml")
	raw, _ := os.ReadFile(cfg)
	os.WriteFile(cfg, []byte(strings.Replace(string(raw), "  file: ./workloads/workload.yaml\n", "", 1)), 0o644)
	r := run(t, dir, nil, "verify", "--quiet")
	if r.code != ExitInconclusive || !strings.Contains(r.stdout, "INCONCLUSIVE") {
		t.Fatalf("verify without workload: %+v", r)
	}
	rep, err := os.ReadFile(filepath.Join(dir, "prodshadow-results", "report.json"))
	if err != nil || !strings.Contains(string(rep), `"QUAL007"`) {
		t.Fatalf("report missing QUAL007: %v", err)
	}
}

func TestVerifyInvalidConfig(t *testing.T) {
	dir := initProject(t)
	cfg := filepath.Join(dir, "prodshadow.yaml")
	raw, _ := os.ReadFile(cfg)
	os.WriteFile(cfg, []byte(strings.Replace(string(raw), "provider: docker", "provider: vm", 1)), 0o644)
	if r := run(t, dir, nil, "verify", "--quiet"); r.code != ExitInvalid {
		t.Fatalf("invalid config should exit 3: %+v", r)
	}
}

func TestVerifyUnavailableProviderIsInfrastructureFailure(t *testing.T) {
	dir := initProject(t)
	r := run(t, dir, map[string]string{"PRODSHADOW_ISOLATION_BINARY": filepath.Join(dir, "no-such-docker")}, "verify", "--quiet")
	if r.code != ExitInfra || !strings.Contains(r.stdout, "provider is not usable") {
		t.Fatalf("missing engine should exit 4: %+v", r)
	}
	rep, err := os.ReadFile(filepath.Join(dir, "prodshadow-results", "report.json"))
	if err != nil || !strings.Contains(string(rep), `"status": "failed"`) {
		t.Fatalf("failed report not written: %v", err)
	}
}

func TestReportRerender(t *testing.T) {
	dir := initProject(t)
	cfg := filepath.Join(dir, "prodshadow.yaml")
	raw, _ := os.ReadFile(cfg)
	os.WriteFile(cfg, []byte(strings.Replace(string(raw), "  file: ./workloads/workload.yaml\n", "", 1)), 0o644)
	run(t, dir, nil, "verify", "--quiet")
	if r := run(t, dir, nil, "report", "--format", "markdown"); r.code != ExitOK || !strings.Contains(r.stdout, "# ProdShadow report: demo") {
		t.Fatalf("report markdown: %+v", r)
	}
	if r := run(t, dir, nil, "report", "--format", "html", "--output", "out.html"); r.code != ExitOK {
		t.Fatalf("report html: %+v", r)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "out.html")); !strings.Contains(string(b), "<!doctype html>") {
		t.Fatal("html not written")
	}
	if r := run(t, dir, nil, "report", "--input", "nope.json"); r.code != ExitInvalid {
		t.Fatalf("missing input: %+v", r)
	}
}

func TestStepSummaryIsWritten(t *testing.T) {
	dir := initProject(t)
	cfg := filepath.Join(dir, "prodshadow.yaml")
	raw, _ := os.ReadFile(cfg)
	os.WriteFile(cfg, []byte(strings.Replace(string(raw), "  file: ./workloads/workload.yaml\n", "", 1)), 0o644)
	summary := filepath.Join(dir, "summary.md")
	run(t, dir, map[string]string{"GITHUB_STEP_SUMMARY": summary}, "verify", "--quiet")
	if b, err := os.ReadFile(summary); err != nil || !strings.Contains(string(b), "ProdShadow report") {
		t.Fatalf("step summary not written: %v", err)
	}
}
