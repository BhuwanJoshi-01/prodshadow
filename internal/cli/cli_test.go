package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/fixture"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/isolation"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/report"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/workload"
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
	writeFile(t, filepath.Join(dir, "workloads/workload.yaml"), "version: 1\nscenarios: []\n")
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
	writeFile(t, filepath.Join(dir, "m.sql"), "CREATE INDEX idx ON orders (customer_id);\nALTER TABLE orders ADD COLUMN x int;\n")
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
	if r := run(t, dir, nil, "inspect", "--migration", "m.sql", "--format", "markdown"); !strings.Contains(r.stdout, "[PS101](") {
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
	writeFile(t, cfg, strings.Replace(string(raw), "  fail_on: [critical]\n", "  fail_on: [critical]\n  rules:\n    PS114: off\n", 1))
	r := run(t, dir, nil, "inspect", "--config", "prodshadow.yaml")
	if r.code != ExitOK || strings.Contains(r.stdout, "PS114") {
		t.Fatalf("rule override not applied: %+v", r)
	}
}

func TestVerifyWithoutWorkloadIsInconclusive(t *testing.T) {
	dir := initProject(t)
	cfg := filepath.Join(dir, "prodshadow.yaml")
	raw, _ := os.ReadFile(cfg)
	writeFile(t, cfg, strings.Replace(string(raw), "  file: ./workloads/workload.yaml\n", "", 1))
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
	writeFile(t, cfg, strings.Replace(string(raw), "provider: docker", "provider: vm", 1))
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
	writeFile(t, cfg, strings.Replace(string(raw), "  file: ./workloads/workload.yaml\n", "", 1))
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
	writeFile(t, cfg, strings.Replace(string(raw), "  file: ./workloads/workload.yaml\n", "", 1))
	summary := filepath.Join(dir, "summary.md")
	run(t, dir, map[string]string{"GITHUB_STEP_SUMMARY": summary}, "verify", "--quiet")
	if b, err := os.ReadFile(summary); err != nil || !strings.Contains(string(b), "ProdShadow report") {
		t.Fatalf("step summary not written: %v", err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestInspectSARIF(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "m.sql"), "\nCREATE INDEX idx ON orders (customer_id);\n")
	r := run(t, dir, nil, "inspect", "--migration", "m.sql", "--format", "sarif")
	if r.code != ExitOK || !strings.Contains(r.stdout, `"ruleId": "PS101"`) || !strings.Contains(r.stdout, `"startLine": 2`) {
		t.Fatalf("inspect sarif: %+v", r)
	}
}

func TestValidateChecksConnectionBudget(t *testing.T) {
	dir := initProject(t)
	r := run(t, dir, map[string]string{"PRODSHADOW_WORKLOAD_CONCURRENCY": "200"}, "validate")
	if r.code != ExitInvalid || !strings.Contains(r.stdout, "max_connections") {
		t.Fatalf("connection budget not checked: %+v", r)
	}
}

func TestPlan(t *testing.T) {
	dir := initProject(t)
	r := run(t, dir, nil, "plan")
	for _, want := range []string{"Schedule   B1 C1 C2 B2 B3 C3", "group 1: wrapped in one transaction", "ACCESS EXCLUSIVE on orders", "8 sessions + 1 background", "Estimate   about 3m48s"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("plan output lacks %q:\n%s", want, r.stdout)
		}
	}
	r = run(t, dir, nil, "plan", "--json")
	var out struct {
		Sessions         int     `json:"sessions"`
		EstimatedSeconds float64 `json:"estimated_seconds"`
	}
	if r.code != ExitOK || json.Unmarshal([]byte(r.stdout), &out) != nil || out.Sessions != 9 || out.EstimatedSeconds != 228 {
		t.Fatalf("plan json: %+v %s", out, r.stdout)
	}
}

func TestDiffRequiresBothReports(t *testing.T) {
	if r := run(t, t.TempDir(), nil, "diff", "--base", "a.json"); r.code != ExitInvalid {
		t.Fatalf("missing --head: %+v", r)
	}
	dir := initProject(t)
	cfg := filepath.Join(dir, "prodshadow.yaml")
	raw, _ := os.ReadFile(cfg)
	writeFile(t, cfg, strings.Replace(string(raw), "  file: ./workloads/workload.yaml\n", "", 1))
	run(t, dir, nil, "verify", "--quiet")
	rep := filepath.Join("prodshadow-results", "report.json")
	r := run(t, dir, nil, "diff", "--base", rep, "--head", rep)
	if r.code != ExitOK || !strings.Contains(r.stdout, "INCONCLUSIVE → INCONCLUSIVE") || !strings.Contains(r.stdout, "**Resolved (0)**") {
		t.Fatalf("diff of a report with itself: %+v", r)
	}
}

func TestInspectExpandsGlobsInOrder(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "migrations"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "migrations", "0002_b.sql"), "CREATE INDEX b ON t (b);\n")
	writeFile(t, filepath.Join(dir, "migrations", "0001_a.sql"), "CREATE INDEX a ON t (a);\n")
	r := run(t, dir, nil, "inspect", "--migration", "migrations/*.sql", "--format", "json")
	if r.code != ExitOK {
		t.Fatalf("inspect glob: %+v", r)
	}
	first := strings.Index(r.stdout, "0001_a.sql")
	second := strings.Index(r.stdout, "0002_b.sql")
	if first < 0 || second < 0 || first > second {
		t.Fatalf("files not expanded in lexical order:\n%s", r.stdout)
	}
	if r := run(t, dir, nil, "inspect", "--migration", "nothing/*.sql"); r.code != ExitInvalid || !strings.Contains(r.stderr, "matches no files") {
		t.Fatalf("empty glob: %+v", r)
	}
}

func TestKeepWarnsInCI(t *testing.T) {
	dir := initProject(t)
	cfg := filepath.Join(dir, "prodshadow.yaml")
	raw, _ := os.ReadFile(cfg)
	writeFile(t, cfg, strings.Replace(string(raw), "  file: ./workloads/workload.yaml\n", "", 1))
	r := run(t, dir, map[string]string{"CI": "true"}, "verify", "--keep", "--quiet")
	if !strings.Contains(r.stderr, "do not use it in CI") {
		t.Fatalf("expected a CI warning for --keep: %+v", r)
	}
}

func TestInspectNamesBlockedStatements(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "m.sql"), "CREATE INDEX i ON orders (a);\n")
	r := run(t, dir, nil, "inspect", "--migration", "m.sql")
	if !strings.Contains(r.stdout, "blocks: INSERT/UPDATE/DELETE/MERGE") || strings.Contains(r.stdout, "blocks: SELECT,") {
		t.Fatalf("inspect should name the blocked statements: %s", r.stdout)
	}
}

func TestRulesSelection(t *testing.T) {
	r := run(t, "", nil, "rules", "ps124", "--json")
	var got []ruleInfo
	if r.code != ExitOK || json.Unmarshal([]byte(r.stdout), &got) != nil || len(got) != 1 || got[0].ID != "PS124" {
		t.Fatalf("rules ps124 --json: %+v", r)
	}
	if !strings.HasSuffix(got[0].Page, "/docs/rules/PS124.md") {
		t.Errorf("page = %q", got[0].Page)
	}
	r = run(t, "", nil, "rules", "OBS002", "cascade-drop")
	if r.code != ExitOK || !strings.Contains(r.stdout, "OBS002 ") || !strings.Contains(r.stdout, "PS124 cascade-drop") || !strings.Contains(r.stdout, "details:") {
		t.Fatalf("rules detail: %+v", r)
	}
	r = run(t, "", nil, "rules", "--kind", "quality")
	if r.code != ExitOK || strings.Contains(r.stdout, "PS101") || !strings.Contains(r.stdout, "QUAL001") {
		t.Fatalf("rules --kind quality: %+v", r)
	}
	if r := run(t, "", nil, "rules", "--kind", "static", "OBS002"); r.code != ExitInvalid {
		t.Fatalf("observed rule with --kind static: %+v", r)
	}
	if r := run(t, "", nil, "rules", "PS999"); r.code != ExitInvalid || !strings.Contains(r.stderr, "unknown rule") {
		t.Fatalf("unknown rule: %+v", r)
	}
	if r := run(t, "", nil, "rules", "--kind", "nope"); r.code != ExitInvalid {
		t.Fatalf("invalid kind: %+v", r)
	}
}

func TestVersionShort(t *testing.T) {
	r := run(t, "", nil, "version", "--short")
	if r.code != ExitOK || strings.Count(r.stdout, "\n") != 1 || strings.Contains(r.stdout, " ") {
		t.Fatalf("version --short: %+v", r)
	}
}

func TestInspectReadsStdin(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Main(context.Background(), []string{"inspect", "--migration", "-", "--format", "json"}, &Env{
		Stdin:  strings.NewReader("ALTER TABLE orders ADD COLUMN note text NOT NULL;\n"),
		Stdout: &out, Stderr: &errOut,
		Getenv: func(string) (string, bool) { return "", false },
	})
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	var doc struct {
		Statements []struct {
			File string `json:"file"`
		} `json:"statements"`
		Findings []struct {
			RuleID string `json:"rule_id"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Statements) != 1 || doc.Statements[0].File != "<stdin>" || len(doc.Findings) == 0 {
		t.Fatalf("unexpected output: %s", out.String())
	}
	if r := run(t, "", nil, "inspect", "--migration", "-"); r.code != ExitInvalid {
		t.Fatalf("no stdin: %+v", r)
	}
}

func TestHistory(t *testing.T) {
	dir := t.TempDir()
	write := func(name, id, project string, at time.Time) {
		r := report.Report{SchemaVersion: report.SchemaVersion, RunID: id, Project: project, StartedAt: at}
		r.Decision.Outcome = "pass"
		b, _ := json.Marshal(r)
		writeFile(t, filepath.Join(dir, "reports", name), string(b))
	}
	if err := os.MkdirAll(filepath.Join(dir, "reports"), 0o755); err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC)
	write("b.json", "run-b", "shop", day.Add(24*time.Hour))
	write("a.json", "run-a", "shop", day)
	write("c.json", "run-c", "billing", day.Add(48*time.Hour))
	writeFile(t, filepath.Join(dir, "reports", "notes.json"), `{"hello": "world"}`)

	r := run(t, dir, nil, "history", "reports", "--project", "shop", "--json")
	var got []report.HistoryEntry
	if r.code != ExitOK || json.Unmarshal([]byte(r.stdout), &got) != nil || len(got) != 2 || got[0].RunID != "run-a" || got[1].RunID != "run-b" {
		t.Fatalf("history --project shop: %+v", r)
	}
	if !strings.Contains(r.stderr, "skipping") || !strings.Contains(r.stderr, "notes.json") {
		t.Errorf("non-report file not reported: %q", r.stderr)
	}
	r = run(t, dir, nil, "history", "--last", "1", "reports")
	if r.code != ExitOK || !strings.Contains(r.stdout, "billing") || strings.Contains(r.stdout, "2026-11-02") {
		t.Fatalf("history --last 1: %+v", r)
	}
	if r := run(t, dir, nil, "history", "reports/notes.json"); r.code != ExitInvalid {
		t.Fatalf("named non-report: %+v", r)
	}
	if r := run(t, dir, nil, "history"); r.code != ExitInvalid {
		t.Fatalf("no arguments: %+v", r)
	}
}

func TestCompletionScripts(t *testing.T) {
	flags := commandFlags(context.Background())
	for _, c := range commands() {
		if c.name == "schema" || c.name == "completion" {
			continue
		}
		if len(flags[c.name]) == 0 {
			t.Errorf("no flags found for %s", c.name)
		}
	}
	r := run(t, "", nil, "completion", "bash")
	if r.code != ExitOK || !strings.Contains(r.stdout, "complete -o bashdefault -o default -F _prodshadow prodshadow") ||
		!regexp.MustCompile(`inspect\) opts="[^"]*--fail-on [^"]*--migration [^"]*--quiet`).MatchString(r.stdout) {
		t.Fatalf("bash completion: %+v", r)
	}
	r = run(t, "", nil, "completion", "zsh")
	if r.code != ExitOK || !strings.HasPrefix(r.stdout, "#compdef prodshadow\n") {
		t.Fatalf("zsh completion: %+v", r)
	}
	r = run(t, "", nil, "completion", "fish")
	if r.code != ExitOK || !strings.Contains(r.stdout, "-n '__fish_seen_subcommand_from history' -l last -d 'only show the N most recent runs (0 shows all)'") {
		t.Fatalf("fish completion: %+v", r)
	}
	if r := run(t, "", nil, "completion", "powershell"); r.code != ExitInvalid {
		t.Fatalf("unsupported shell: %+v", r)
	}
}

func TestPlanListsCompatibilityProbes(t *testing.T) {
	r := run(t, "", nil, "plan", "--config", "../../examples/rolling-deploy/prodshadow.yaml")
	if r.code != ExitOK || !strings.Contains(r.stdout, "Versions   v1: 2 probe(s); v2: 1 probe(s)") {
		t.Fatalf("plan: %+v", r)
	}
	if r := run(t, initProject(t), nil, "plan"); strings.Contains(r.stdout, "Versions") {
		t.Fatalf("no probes, no line: %s", r.stdout)
	}
}

func TestLocks(t *testing.T) {
	r := run(t, "", nil, "locks")
	if r.code != ExitOK || strings.Count(r.stdout, "\n") < 9 || !strings.Contains(r.stdout, "ACCESS EXCLUSIVE") {
		t.Fatalf("locks: %+v", r)
	}
	for _, spelling := range [][]string{{"share", "row", "exclusive"}, {"SHARE_ROW_EXCLUSIVE"}, {"share-row-exclusive"}, {"ShareRowExclusiveLock"}} {
		r := run(t, "", nil, append([]string{"locks"}, spelling...)...)
		if r.code != ExitOK || !strings.HasPrefix(r.stdout, "SHARE ROW EXCLUSIVE (ShareRowExclusiveLock in pg_locks)") {
			t.Fatalf("locks %v: %+v", spelling, r)
		}
	}
	r = run(t, "", nil, "locks", "--json", "access", "share")
	var got []lockInfo
	if r.code != ExitOK || json.Unmarshal([]byte(r.stdout), &got) != nil || len(got) != 1 || len(got[0].Conflicts) != 1 || got[0].Conflicts[0] != "ACCESS EXCLUSIVE" {
		t.Fatalf("locks --json access share: %+v", r)
	}
	if r := run(t, "", nil, "locks", "shared"); r.code != ExitInvalid {
		t.Fatalf("unknown mode: %+v", r)
	}
}

func TestInspectJUnit(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "m.sql"), "-- prodshadow:ignore PS114 reason=\"lock_timeout is set by the deploy tool\"\nCREATE INDEX orders_note_idx ON orders (note);\nALTER TABLE orders ADD COLUMN region text;\n")
	r := run(t, dir, nil, "inspect", "--migration", "m.sql", "--format", "junit", "--fail-on", "warning")
	if r.code != ExitBlock || !strings.HasPrefix(r.stdout, "<?xml") || !strings.Contains(r.stdout, `<failure message=`) || !strings.Contains(r.stdout, "PS101 create-index-blocks-writes (m.sql:2)") {
		t.Fatalf("inspect --format junit --fail-on warning: %+v", r)
	}
	if !strings.Contains(r.stdout, `<skipped message="suppressed: lock_timeout is set by the deploy tool">`) {
		t.Errorf("suppressed finding should be skipped:\n%s", r.stdout)
	}
	r = run(t, dir, nil, "inspect", "--migration", "m.sql", "--format", "junit")
	if r.code != ExitOK || strings.Contains(r.stdout, "<failure") {
		t.Fatalf("without --fail-on nothing fails: %+v", r)
	}
	writeFile(t, filepath.Join(dir, "ok.sql"), "SELECT 1;\n")
	if r := run(t, dir, nil, "inspect", "--migration", "ok.sql", "--format", "junit"); !strings.Contains(r.stdout, `name="no static findings"`) {
		t.Fatalf("empty result needs a test case: %+v", r)
	}
}

func TestValidateWarnsAboutUnreachableQueueBudget(t *testing.T) {
	dir := initProject(t)
	cfg := filepath.Join(dir, "prodshadow.yaml")
	raw, _ := os.ReadFile(cfg)
	writeFile(t, cfg, strings.Replace(string(raw), "  # max_queued_sessions: 10 ", "  max_queued_sessions: 20 #", 1))
	r := run(t, dir, nil, "validate")
	if r.code != ExitOK || !strings.Contains(r.stdout+r.stderr, "OBS012 can never fire") {
		t.Fatalf("validate: %+v", r)
	}
	writeFile(t, cfg, strings.Replace(string(raw), "  # max_queued_sessions: 10 ", "  max_queued_sessions: 4 #", 1))
	if r := run(t, dir, nil, "validate"); strings.Contains(r.stdout+r.stderr, "can never fire") {
		t.Fatalf("reachable budget warned: %+v", r)
	}
}

type fakeEngine struct{ isolation.Provider }

func TestDoctorResourceCheck(t *testing.T) {
	// resourceCheck only needs EngineResources, which reports ErrNoCleanup
	// for providers without an engine; such providers get no check.
	if _, ok := resourceCheck(context.Background(), fakeEngine{}, 1024, 2); ok {
		t.Fatal("providers without a container engine get no resources check")
	}
	cases := []struct {
		engineMB, ncpu, memoryMB int
		cpus                     float64
		ok, warn                 bool
	}{
		{16000, 4, 2048, 2, true, false},
		{2000, 4, 2048, 2, false, false}, // too little memory
		{16000, 1, 2048, 2, true, true},  // too few CPUs
		{3000, 4, 2048, 2, true, true},   // little headroom
	}
	for _, tc := range cases {
		c := compareResources(tc.engineMB, tc.ncpu, tc.memoryMB, tc.cpus)
		if c.ok != tc.ok || c.warn != tc.warn {
			t.Errorf("%+v: got ok=%v warn=%v (%s)", tc, c.ok, c.warn, c.detail)
		}
	}
}

func TestInspectQuiet(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "m.sql"), "CREATE INDEX orders_note_idx ON orders (note);\n")
	r := run(t, dir, nil, "inspect", "--migration", "m.sql", "--quiet")
	if r.code != ExitOK || strings.Contains(r.stdout, "statement(s) analysed") || strings.Contains(r.stdout, "locks:") || !strings.HasPrefix(r.stdout, "WARNING PS101") {
		t.Fatalf("inspect --quiet: %+v", r)
	}
}

func TestDraftWorkload(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "stats.csv"), "calls,mean_exec_time,query\n5400,0.2,\"SELECT id, status FROM orders WHERE id = $1\"\n800,0.9,\"UPDATE orders SET status = $1\n WHERE id = $2\"\n3,1.0,VACUUM orders\n")
	r := run(t, dir, nil, "draft-workload", "--stats", "stats.csv", "--name", "orders")
	if r.code != ExitOK {
		t.Fatalf("draft-workload: %+v", r)
	}
	writeFile(t, filepath.Join(dir, "workload.yaml"), r.stdout)
	if _, err := workload.Load(filepath.Join(dir, "workload.yaml")); err != nil {
		t.Fatalf("draft does not load: %v\n%s", err, r.stdout)
	}
	if !strings.Contains(r.stdout, "id: update_orders") || !strings.Contains(r.stdout, "VACUUM orders") {
		t.Errorf("draft:\n%s", r.stdout)
	}
	writeFile(t, filepath.Join(dir, "bad.csv"), "query,count\nSELECT 1,3\n")
	if r := run(t, dir, nil, "draft-workload", "--stats", "bad.csv"); r.code != ExitInvalid || !strings.Contains(r.stderr, "calls column") {
		t.Fatalf("missing calls column: %+v", r)
	}
	writeFile(t, filepath.Join(dir, "ddl.csv"), "query,calls\nCREATE INDEX i ON t (a),1\n")
	if r := run(t, dir, nil, "draft-workload", "--stats", "ddl.csv"); r.code != ExitInvalid {
		t.Fatalf("no usable statements: %+v", r)
	}
	if r := run(t, dir, nil, "draft-workload"); r.code != ExitInvalid {
		t.Fatalf("--stats is required: %+v", r)
	}
}

func TestInspectMarkdownLinksRules(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "m.sql"), "CREATE INDEX orders_note_idx ON orders (note);\n")
	r := run(t, dir, nil, "inspect", "--migration", "m.sql", "--format", "markdown")
	if r.code != ExitOK || !strings.Contains(r.stdout, "[PS101](https://github.com/BhuwanJoshi-01/Open-Source-1/blob/main/docs/rules/PS101.md) create-index-blocks-writes") {
		t.Fatalf("inspect --format markdown: %+v", r)
	}
}

// gitRepo creates a repository with one committed migration and returns it.
func gitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	gitRun := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	gitRun("init", "-q", "-b", "main")
	if err := os.MkdirAll(filepath.Join(dir, "migrations"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "migrations", "0001_old.sql"), "CREATE INDEX a ON t (a);\n")
	gitRun("add", ".")
	gitRun("commit", "-q", "-m", "old")
	gitRun("checkout", "-q", "-b", "feature")
	writeFile(t, filepath.Join(dir, "migrations", "0002_new.sql"), "CREATE INDEX CONCURRENTLY b ON t (b);\n")
	gitRun("add", ".")
	gitRun("commit", "-q", "-m", "new")
	return dir
}

func TestInspectChangedSince(t *testing.T) {
	dir := gitRepo(t)
	r := run(t, dir, nil, "inspect", "--migration", "migrations/*.sql", "--changed-since", "main", "--format", "json")
	var doc struct {
		Statements []struct {
			File string `json:"file"`
		} `json:"statements"`
	}
	if r.code != ExitOK || json.Unmarshal([]byte(r.stdout), &doc) != nil || len(doc.Statements) != 1 || !strings.HasSuffix(doc.Statements[0].File, "0002_new.sql") {
		t.Fatalf("inspect --changed-since main: %+v", r)
	}
	r = run(t, dir, nil, "inspect", "--migration", "migrations/*.sql", "--changed-since", "HEAD")
	if r.code != ExitOK || !strings.Contains(r.stdout, "No migration changed since HEAD") {
		t.Fatalf("nothing changed: %+v", r)
	}
	if r := run(t, dir, nil, "inspect", "--migration", "migrations/*.sql", "--changed-since", "no-such-ref"); r.code != ExitInvalid || !strings.Contains(r.stderr, "--changed-since") {
		t.Fatalf("unknown ref: %+v", r)
	}
	out := filepath.Join(dir, "written-by-git")
	if r := run(t, dir, nil, "inspect", "--migration", "migrations/*.sql", "--changed-since", "--output="+out); r.code != ExitInvalid || !strings.Contains(r.stderr, "invalid ref") {
		t.Fatalf("option-like ref: %+v", r)
	}
	if _, err := os.Stat(out); err == nil {
		t.Fatal("an option-like ref reached git")
	}
}

func TestPlanAndVerifyChangedSince(t *testing.T) {
	dir := gitRepo(t)
	init := run(t, dir, nil, "init", "--project", "demo", "--dir", "app")
	if init.code != ExitOK {
		t.Fatalf("init: %+v", init)
	}
	cfg := filepath.Join(dir, "app", "prodshadow.yaml")
	raw, _ := os.ReadFile(cfg)
	writeFile(t, cfg, strings.Replace(string(raw), "    - ./migrations/0001_add_order_note.sql\n", "    - ../migrations/*.sql\n", 1))
	r := run(t, dir, nil, "plan", "--config", "app/prodshadow.yaml", "--changed-since", "main")
	if r.code != ExitOK || !strings.Contains(r.stdout, "0002_new.sql") || strings.Contains(r.stdout, "0001_old.sql") {
		t.Fatalf("plan --changed-since: %+v", r)
	}
	// Nothing changed: verify exits before starting a server.
	r = run(t, dir, nil, "verify", "--config", "app/prodshadow.yaml", "--changed-since", "HEAD")
	if r.code != ExitOK || !strings.Contains(r.stdout, "nothing to verify") {
		t.Fatalf("verify --changed-since HEAD: %+v", r)
	}
}

func TestDraftFixture(t *testing.T) {
	dir := initProject(t)
	writeFile(t, filepath.Join(dir, "counts.csv"), "relname,reltuples\norders,2500000\n")
	r := run(t, dir, nil, "draft-fixture", "--schema", "fixtures/schema.sql", "--rows", "counts.csv")
	if r.code != ExitOK || !strings.Contains(r.stdout, "  - name: orders\n    rows: 25000\n") {
		t.Fatalf("draft-fixture: %+v", r)
	}
	path := filepath.Join(dir, "draft.yaml")
	writeFile(t, path, r.stdout)
	if _, err := fixture.LoadGeneratorFile(path, 50_000_000); err != nil {
		t.Fatalf("draft does not load: %v\n%s", err, r.stdout)
	}
	if r := run(t, dir, nil, "draft-fixture"); r.code != ExitInvalid {
		t.Fatalf("--schema is required: %+v", r)
	}
	writeFile(t, filepath.Join(dir, "bad.csv"), "table\norders\n")
	if r := run(t, dir, nil, "draft-fixture", "--schema", "fixtures/schema.sql", "--rows", "bad.csv"); r.code != ExitInvalid {
		t.Fatalf("missing count column: %+v", r)
	}
}

func TestValidateWarnsAboutUnknownWorkloadTables(t *testing.T) {
	dir := initProject(t)
	wl := filepath.Join(dir, "workloads", "workload.yaml")
	raw, _ := os.ReadFile(wl)
	writeFile(t, wl, strings.Replace(string(raw), "FROM orders", "FROM ordrs", 1))
	r := run(t, dir, nil, "validate")
	if r.code != ExitOK || !strings.Contains(r.stdout, "uses table ordrs, which the fixture schema does not create") {
		t.Fatalf("validate: %+v", r)
	}
	writeFile(t, wl, string(raw))
	if r := run(t, dir, nil, "validate"); strings.Contains(r.stdout, "does not create") {
		t.Fatalf("starter project warned: %+v", r)
	}
}
