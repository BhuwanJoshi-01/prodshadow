package report

import (
	"strings"
	"testing"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/risk"
)

// A static-only run (no workload) still produces complete, honest reports.
func TestStaticOnlyReport(t *testing.T) {
	in := sampleInput(t)
	in.Result = nil
	in.Assessment = risk.Evaluate(risk.Input{Config: in.Config, Statements: in.Statements, NoExperiment: "no workload is configured"})
	r := Build(in)
	if r.Status != StatusInconclusive || len(r.Phases) != 0 || len(r.Comparisons) != 0 {
		t.Fatalf("unexpected static-only report: %s phases=%d", r.Status, len(r.Phases))
	}
	md := r.Markdown()
	for _, want := range []string{"INCONCLUSIVE", "QUAL007", "Inconclusive is not a pass"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown lacks %q", want)
		}
	}
	if strings.Contains(md, "## Timeline") || strings.Contains(md, "## Lock telemetry") {
		t.Error("sections without data should be omitted")
	}
	for _, render := range []func() ([]byte, error){r.JSON, r.SARIF, r.JUnit} {
		if _, err := render(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.HTML(); err != nil {
		t.Fatal(err)
	}
}
