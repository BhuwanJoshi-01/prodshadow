// Package risk turns experiment evidence into transparent findings and a CI
// decision. Every finding carries its severity, confidence, evidence kind,
// the observed value and the threshold it was compared with; nothing is
// collapsed into an opaque score.
package risk

import (
	"math"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/experiment"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/stats"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/workload"
)

// Side summarises one scenario across all repeats of one phase kind.
// Percentiles are medians of the per-repeat values; counts are totals.
type Side struct {
	Repeats       int     `json:"repeats"`
	Ops           int     `json:"ops"`
	Errors        int     `json:"errors"`
	ErrorRate     float64 `json:"error_rate"`
	Throughput    float64 `json:"throughput_per_sec"`
	P50MS         float64 `json:"p50_ms"`
	P95MS         float64 `json:"p95_ms"`
	P99MS         float64 `json:"p99_ms"`
	MaxObservedMS float64 `json:"max_observed_ms"`
}

// Comparison is the paired result for one scenario.
type Comparison struct {
	Scenario   string `json:"scenario"`
	Mode       string `json:"mode"`
	Background bool   `json:"background,omitempty"`
	Baseline   Side   `json:"baseline"`
	Candidate  Side   `json:"candidate"`
	// During covers only the time the migration was running.
	During *Side `json:"during_migration,omitempty"`

	P95Ratio            *float64 `json:"p95_ratio,omitempty"`
	P99Ratio            *float64 `json:"p99_ratio,omitempty"`
	ErrorRateDeltaPP    float64  `json:"error_rate_delta_pct_points"`
	ThroughputChangePct *float64 `json:"throughput_change_pct,omitempty"`
	// BaselineNoiseCV is the coefficient of variation of the baseline p95
	// across repeats; nil with fewer than two repeats.
	BaselineNoiseCV *float64 `json:"baseline_noise_cv,omitempty"`
	// Stall is the longest wait in excess of the baseline p99 seen by
	// this scenario while the migration ran.
	StallMS float64 `json:"stall_ms"`
}

func ptr(v float64) *float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	r := stats.Round(v, 3)
	return &r
}

// Compare builds per-scenario comparisons from experiment phases.
func Compare(res *experiment.Result) []Comparison {
	if res == nil || len(res.Phases) == 0 {
		return nil
	}
	n := len(res.Phases[0].Scenarios)
	var out []Comparison
	for i := 0; i < n; i++ {
		var base, cand, during []workload.ScenarioStats
		for _, ph := range res.Phases {
			if i >= len(ph.Scenarios) {
				continue
			}
			if ph.Kind == experiment.Baseline {
				base = append(base, ph.Scenarios[i])
				continue
			}
			cand = append(cand, ph.Scenarios[i])
			if i < len(ph.DuringMigration) {
				during = append(during, ph.DuringMigration[i])
			}
		}
		first := res.Phases[0].Scenarios[i]
		c := Comparison{Scenario: first.ID, Mode: first.Mode, Background: first.Background,
			Baseline: side(base), Candidate: side(cand)}
		if len(during) > 0 {
			d := side(during)
			c.During = &d
			c.StallMS = stats.Round(math.Max(0, d.MaxObservedMS-c.Baseline.P99MS), 1)
		}
		c.P95Ratio = ptr(stats.Ratio(c.Candidate.P95MS, c.Baseline.P95MS))
		c.P99Ratio = ptr(stats.Ratio(c.Candidate.P99MS, c.Baseline.P99MS))
		c.ErrorRateDeltaPP = stats.Round((c.Candidate.ErrorRate-c.Baseline.ErrorRate)*100, 3)
		if c.Baseline.Throughput > 0 {
			c.ThroughputChangePct = ptr((c.Candidate.Throughput - c.Baseline.Throughput) / c.Baseline.Throughput * 100)
		}
		var p95s []float64
		for _, b := range base {
			if b.Latency.Count > 0 {
				p95s = append(p95s, b.Latency.P95MS)
			}
		}
		c.BaselineNoiseCV = ptr(stats.CV(p95s))
		out = append(out, c)
	}
	return out
}

func side(xs []workload.ScenarioStats) Side {
	s := Side{Repeats: len(xs)}
	var p50, p95, p99, thr []float64
	for _, x := range xs {
		s.Ops += x.Ops
		s.Errors += x.Errors
		s.MaxObservedMS = math.Max(s.MaxObservedMS, x.MaxObservedMS)
		thr = append(thr, x.Throughput)
		if x.Latency.Count > 0 {
			p50 = append(p50, x.Latency.P50MS)
			p95 = append(p95, x.Latency.P95MS)
			p99 = append(p99, x.Latency.P99MS)
		}
	}
	if s.Ops > 0 {
		s.ErrorRate = stats.Round(float64(s.Errors)/float64(s.Ops), 6)
	}
	s.Throughput = zeroNaN(stats.Round(stats.Median(thr), 3))
	s.P50MS = zeroNaN(stats.Round(stats.Median(p50), 3))
	s.P95MS = zeroNaN(stats.Round(stats.Median(p95), 3))
	s.P99MS = zeroNaN(stats.Round(stats.Median(p99), 3))
	return s
}

func zeroNaN(v float64) float64 {
	if math.IsNaN(v) {
		return 0
	}
	return v
}
