package report

import (
	"strings"
	"testing"

	"github.com/BhuwanJoshi-01/prodshadow/internal/experiment"
	"github.com/BhuwanJoshi-01/prodshadow/internal/migrate"
	"github.com/BhuwanJoshi-01/prodshadow/internal/risk"
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

func TestHTMLCompatibilitySection(t *testing.T) {
	r := Build(sampleInput(t))
	out, _ := r.HTML()
	if strings.Contains(out, "Application compatibility") {
		t.Fatal("no section without probes")
	}
	for i := range r.Phases {
		if r.Phases[i].Kind == "candidate" {
			r.Phases[i].Compatibility = []experiment.Compatibility{
				{Version: "v1", Probe: "get_product", BeforeOK: true, AfterSQLState: "42703", AfterError: `column "<b>legacy_sku</b>" does not exist`},
			}
		}
	}
	out, err := r.HTML()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<h2>Application compatibility</h2>", "<code>get_product</code>", `class="sev-critical">failed (42703: column &#34;&lt;b&gt;legacy_sku&lt;/b&gt;&#34; does not exist)`} {
		if !strings.Contains(out, want) {
			t.Errorf("HTML lacks %q", want)
		}
	}
}

func TestHTMLShowsLockRetries(t *testing.T) {
	r := Build(sampleInput(t))
	for i := range r.Phases {
		if m := r.Phases[i].Migration; m != nil {
			m.Retries = []migrate.Retry{{StatementID: "1.1", Attempt: 1}, {StatementID: "1.1", Attempt: 2}}
		}
	}
	out, err := r.HTML()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "<small>2 lock retries</small>") {
		t.Fatal("HTML lacks the retry count")
	}
}
