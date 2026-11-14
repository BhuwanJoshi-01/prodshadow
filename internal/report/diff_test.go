package report

import (
	"strings"
	"testing"

	"github.com/BhuwanJoshi-01/prodshadow/internal/risk"
)

func TestCompareReports(t *testing.T) {
	base := Build(sampleInput(t))
	head := Build(sampleInput(t))
	head.RunID = "run-2"
	head.Decision.Outcome = "pass"
	// The alternative resolves the lock timeout and introduces a caveat.
	var kept []risk.Finding
	for _, f := range head.Findings {
		if f.RuleID != "OBS002" {
			kept = append(kept, f)
		}
	}
	head.Findings = append(kept, risk.Finding{RuleID: "PS121", Rule: "concurrent-index-caveats", Severity: "info", File: "m.sql", Line: 1})
	head.Comparisons[0].StallMS = 12
	head.PredictedConflicts = nil // the alternative conflicts with nothing

	d := Compare(base, head)
	if !d.Comparable || d.BaseDecision != "block" || d.HeadDecision != "pass" {
		t.Fatalf("unexpected header %+v", d)
	}
	if len(d.Resolved) != 1 || d.Resolved[0].RuleID != "OBS002" || len(d.Introduced) != 1 || d.Introduced[0].RuleID != "PS121" {
		t.Fatalf("resolved %v introduced %v", d.Resolved, d.Introduced)
	}
	if d.Scenarios[0].BaseStallMS <= d.Scenarios[0].HeadStallMS {
		t.Fatalf("stall should have dropped: %+v", d.Scenarios[0])
	}
	md := d.Markdown()
	if d.Scenarios[0].BaseConflict != "1.1 ACCESS EXCLUSIVE on orders" || d.Scenarios[0].HeadConflict != "none" {
		t.Fatalf("predicted conflicts: %+v", d.Scenarios[0])
	}
	for _, want := range []string{"BLOCK → PASS", "**Resolved (1)**", "OBS002", "**Introduced (1)**", "| read_order | 1.1 ACCESS EXCLUSIVE on orders → none |"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown lacks %q:\n%s", want, md)
		}
	}
}

func TestCompareFlagsDifferentInputs(t *testing.T) {
	base := Build(sampleInput(t))
	head := Build(sampleInput(t))
	head.Fixture.Hash = "sha256:other"
	if d := Compare(base, head); d.Comparable || !strings.Contains(strings.Join(d.Caveats, ","), "fixtures differ") {
		t.Fatalf("different fixtures must be flagged: %+v", d)
	}
}

func TestDiffKeepsApplicationVersionsApart(t *testing.T) {
	base := Build(sampleInput(t))
	head := Build(sampleInput(t))
	base.Findings = append(base.Findings, risk.Finding{RuleID: "OBS013", Rule: "app-version-incompatible", Severity: "critical", AppVersion: "v1"})
	head.Findings = append(head.Findings, risk.Finding{RuleID: "OBS013", Rule: "app-version-incompatible", Severity: "critical", AppVersion: "v2"})
	d := Compare(base, head)
	resolved := make([]string, 0, len(d.Resolved))
	introduced := make([]string, 0, len(d.Introduced))
	for _, f := range d.Resolved {
		resolved = append(resolved, f.Where)
	}
	for _, f := range d.Introduced {
		introduced = append(introduced, f.Where)
	}
	if len(resolved) != 1 || resolved[0] != "version v1" || len(introduced) != 1 || introduced[0] != "version v2" {
		t.Fatalf("resolved %v, introduced %v", resolved, introduced)
	}
}
