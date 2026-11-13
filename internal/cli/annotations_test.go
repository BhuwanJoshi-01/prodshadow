package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/risk"
)

func TestWriteAnnotations(t *testing.T) {
	var buf bytes.Buffer
	writeAnnotations(&buf, []risk.Finding{
		{RuleID: "OBS002", Rule: "migration-lock-timeout", Severity: "critical", File: "m.sql", Line: 4, Message: "blocked\n::error::injected"},
		{RuleID: "PS101", Rule: "create-index-blocks-writes", Severity: "warning", File: "a,b.sql", Line: 1, Message: "100% sure"},
		{RuleID: "PS114", Severity: "info", File: "m.sql", Line: 4, Message: "skipped"},
		{RuleID: "PS109", Severity: "warning", Suppressed: true, Message: "skipped"},
	})
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected two annotations, got %q", lines)
	}
	if lines[0] != "::error title=ProdShadow OBS002 migration-lock-timeout,file=m.sql,line=4::blocked%0A::error::injected" {
		t.Fatalf("unexpected first annotation %q", lines[0])
	}
	if !strings.Contains(lines[1], "file=a%2Cb.sql") || !strings.HasSuffix(lines[1], "::100%25 sure") {
		t.Fatalf("properties or data not escaped: %q", lines[1])
	}
}

func TestAnnotationsOnlyInGitHubActions(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir+"/m.sql", "CREATE INDEX idx ON orders (customer_id);\n")
	if r := run(t, dir, nil, "inspect", "--migration", "m.sql"); strings.Contains(r.stdout, "::warning") {
		t.Fatal("annotations must not be printed outside GitHub Actions")
	}
	r := run(t, dir, map[string]string{"GITHUB_ACTIONS": "true"}, "inspect", "--migration", "m.sql")
	if !strings.Contains(r.stdout, "::warning title=ProdShadow PS101") {
		t.Fatalf("expected an annotation in GitHub Actions: %s", r.stdout)
	}
}
