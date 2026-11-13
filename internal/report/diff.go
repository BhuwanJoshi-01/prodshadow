package report

import (
	"fmt"
	"sort"
	"strings"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/risk"
)

// Diff compares two reports, typically a migration and its rewritten
// alternative, or the same migration before and after a fixture change.
type Diff struct {
	BaseRun      string         `json:"base_run"`
	HeadRun      string         `json:"head_run"`
	BaseDecision string         `json:"base_decision"`
	HeadDecision string         `json:"head_decision"`
	Resolved     []DiffFinding  `json:"resolved"`
	Introduced   []DiffFinding  `json:"introduced"`
	Persisting   []DiffFinding  `json:"persisting"`
	Scenarios    []DiffScenario `json:"scenarios"`
	// Comparable is false when the runs used different fixtures or
	// workloads, which makes the numbers hard to compare.
	Comparable bool     `json:"comparable"`
	Caveats    []string `json:"caveats,omitempty"`
}

// DiffFinding identifies a finding across reports.
type DiffFinding struct {
	RuleID   string `json:"rule_id"`
	Rule     string `json:"rule"`
	Severity string `json:"severity"`
	Where    string `json:"where"`
}

// DiffScenario compares one scenario's candidate measurements.
type DiffScenario struct {
	Scenario      string   `json:"scenario"`
	BaseP95MS     float64  `json:"base_p95_ms"`
	HeadP95MS     float64  `json:"head_p95_ms"`
	BaseStallMS   float64  `json:"base_stall_ms"`
	HeadStallMS   float64  `json:"head_stall_ms"`
	BaseErrorRate float64  `json:"base_error_rate"`
	HeadErrorRate float64  `json:"head_error_rate"`
	BaseRatio     *float64 `json:"base_p95_ratio,omitempty"`
	HeadRatio     *float64 `json:"head_p95_ratio,omitempty"`
}

// key identifies a finding independently of message wording. Statement
// locations are excluded because alternatives usually change line numbers.
func findingKey(f risk.Finding) string {
	return f.RuleID + "|" + f.Scenario + "|" + f.Table
}

func diffFinding(f risk.Finding) DiffFinding {
	where := location(f.File, f.Line)
	if f.Scenario != "" {
		where = "scenario " + f.Scenario
	}
	return DiffFinding{RuleID: f.RuleID, Rule: f.Rule, Severity: string(f.Severity), Where: where}
}

// Compare builds the difference between base and head.
func Compare(base, head *Report) Diff {
	d := Diff{BaseRun: base.RunID, HeadRun: head.RunID, BaseDecision: base.Decision.Outcome, HeadDecision: head.Decision.Outcome, Comparable: true}
	if base.Fixture.Hash != head.Fixture.Hash {
		d.Comparable = false
		d.Caveats = append(d.Caveats, "the fixtures differ")
	}
	if base.Workload.Hash != head.Workload.Hash {
		d.Comparable = false
		d.Caveats = append(d.Caveats, "the workloads differ")
	}
	if base.Engine.PostgresMajor != head.Engine.PostgresMajor {
		d.Comparable = false
		d.Caveats = append(d.Caveats, "the PostgreSQL versions differ")
	}

	active := func(r *Report) map[string]risk.Finding {
		m := map[string]risk.Finding{}
		for _, f := range r.Findings {
			if !f.Suppressed {
				m[findingKey(f)] = f
			}
		}
		return m
	}
	b, h := active(base), active(head)
	for k, f := range b {
		if _, ok := h[k]; ok {
			d.Persisting = append(d.Persisting, diffFinding(h[k]))
		} else {
			d.Resolved = append(d.Resolved, diffFinding(f))
		}
	}
	for k, f := range h {
		if _, ok := b[k]; !ok {
			d.Introduced = append(d.Introduced, diffFinding(f))
		}
	}
	for _, list := range []*[]DiffFinding{&d.Resolved, &d.Introduced, &d.Persisting} {
		sort.Slice(*list, func(i, j int) bool { return (*list)[i].RuleID+(*list)[i].Where < (*list)[j].RuleID+(*list)[j].Where })
		if *list == nil {
			*list = []DiffFinding{}
		}
	}

	headByName := map[string]risk.Comparison{}
	for _, c := range head.Comparisons {
		headByName[c.Scenario] = c
	}
	for _, bc := range base.Comparisons {
		hc, ok := headByName[bc.Scenario]
		if !ok || bc.Background {
			continue
		}
		d.Scenarios = append(d.Scenarios, DiffScenario{
			Scenario: bc.Scenario, BaseP95MS: bc.Candidate.P95MS, HeadP95MS: hc.Candidate.P95MS,
			BaseStallMS: bc.StallMS, HeadStallMS: hc.StallMS,
			BaseErrorRate: bc.Candidate.ErrorRate, HeadErrorRate: hc.Candidate.ErrorRate,
			BaseRatio: bc.P95Ratio, HeadRatio: hc.P95Ratio,
		})
	}
	return d
}

// Markdown renders the difference for a pull request comment.
func (d Diff) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "## ProdShadow comparison\n\n**Decision:** %s → %s (`%s` → `%s`)\n\n", strings.ToUpper(d.BaseDecision), strings.ToUpper(d.HeadDecision), d.BaseRun, d.HeadRun)
	if !d.Comparable {
		fmt.Fprintf(&b, "> The runs are not directly comparable: %s.\n\n", strings.Join(d.Caveats, ", "))
	}
	section := func(title string, fs []DiffFinding) {
		fmt.Fprintf(&b, "**%s (%d)**\n\n", title, len(fs))
		for _, f := range fs {
			fmt.Fprintf(&b, "- %s %s %s — %s\n", f.Severity, f.RuleID, f.Rule, esc(f.Where))
		}
		if len(fs) > 0 {
			b.WriteString("\n")
		}
	}
	section("Resolved", d.Resolved)
	section("Introduced", d.Introduced)
	section("Persisting", d.Persisting)
	if len(d.Scenarios) > 0 {
		b.WriteString("| Scenario | Candidate p95 | Stall during migration | Error rate |\n|---|---|---|---|\n")
		for _, s := range d.Scenarios {
			fmt.Fprintf(&b, "| %s | %s → %s | %s → %s | %.2f%% → %.2f%% |\n", esc(s.Scenario), fmtMS(s.BaseP95MS), fmtMS(s.HeadP95MS),
				fmtMS(s.BaseStallMS), fmtMS(s.HeadStallMS), s.BaseErrorRate*100, s.HeadErrorRate*100)
		}
		b.WriteString("\n")
	}
	return b.String()
}
