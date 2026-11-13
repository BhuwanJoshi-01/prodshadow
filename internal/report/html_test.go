package report

import (
	"strings"
	"testing"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/risk"
)

func TestHTMLEscapesUntrustedText(t *testing.T) {
	r := Build(sampleInput(t))
	// Finding messages can quote server errors and identifiers chosen by
	// whoever wrote the migration.
	r.Findings = append(r.Findings, risk.Finding{RuleID: "OBS003", Rule: "migration-failed", Severity: "critical",
		Confidence: "high", Evidence: "observed", Message: `relation "<script>alert(1)</script>" does not exist`,
		DocsURL: "javascript:alert(1)"})
	r.Project = `<img src=x onerror=alert(1)>`
	out, err := r.HTML()
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"<script>alert(1)</script>", "<img src=x", `href="javascript:`} {
		if strings.Contains(out, bad) {
			t.Errorf("unescaped content %q in HTML report", bad)
		}
	}
	if !strings.Contains(out, "&lt;script&gt;") {
		t.Error("expected escaped script tag")
	}
}

func TestHTMLIsSelfContained(t *testing.T) {
	out, err := Build(sampleInput(t)).HTML()
	if err != nil {
		t.Fatal(err)
	}
	for _, external := range []string{"<script src", "<link ", "@import", "http://", "https://fonts"} {
		if strings.Contains(out, external) {
			t.Errorf("HTML report must not load external resources (%q)", external)
		}
	}
	if !strings.Contains(out, "<svg") || !strings.Contains(out, "prefers-color-scheme: dark") {
		t.Error("expected the timeline chart and dark mode styles")
	}
}
