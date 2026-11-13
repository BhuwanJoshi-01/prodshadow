package risk

import (
	"math"
	"sort"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/experiment"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/stats"
)

// UnstableCV is the baseline p95 coefficient of variation above which
// latency comparisons are inconclusive.
const UnstableCV = 0.5

// Quality describes how much the experiment can be trusted.
type Quality struct {
	Representation     string   `json:"representation"`
	FixtureRows        int64    `json:"fixture_rows"`
	DatabaseBytes      int64    `json:"database_bytes"`
	RepeatsRequested   int      `json:"repeats_requested"`
	PairsCompleted     int      `json:"pairs_completed"`
	MinSamples         int      `json:"min_samples"`
	MinSamplesRequired int      `json:"min_samples_required"`
	UndersampledScenarios []string `json:"undersampled_scenarios,omitempty"`
	MaxBaselineNoiseCV *float64 `json:"max_baseline_noise_cv,omitempty"`
	NoisyScenarios     []string `json:"noisy_scenarios,omitempty"`
	SampleIntervalMS   int      `json:"sample_interval_ms"`
	SamplerMaxGapMS    float64  `json:"sampler_max_gap_ms"`
	SamplerFailures    int      `json:"sampler_failures"`
	ConnectFailures    int      `json:"connect_failures"`
	Limitations        []string `json:"limitations"`
}

func assessQuality(res *experiment.Result, comps []Comparison, representation string, minSamples, repeats, interval int) Quality {
	q := Quality{Representation: representation, MinSamplesRequired: minSamples, RepeatsRequested: repeats, SampleIntervalMS: interval}
	q.Limitations = []string{
		"Lock waits shorter than the sampling interval may be missed by server-side sampling; client-side latency covers them.",
		"Durations of rewrites and index builds scale with data size and hardware; a small fixture understates them.",
		"Passing this experiment does not guarantee a safe production migration.",
	}
	if representation == "manual-fixture" || representation == "synthetic" {
		q.Limitations = append(q.Limitations, "The fixture is "+representation+"; it is not claimed to match production data distribution or scale.")
	}
	if res == nil {
		return q
	}
	for _, t := range res.Tables {
		q.FixtureRows += t.RowsEstimate
	}
	q.DatabaseBytes = res.Server.DatabaseBytes

	cands := 0
	q.MinSamples = math.MaxInt
	under := map[string]bool{}
	for _, ph := range res.Phases {
		if ph.Kind == experiment.Candidate {
			cands++
		}
		q.SamplerMaxGapMS = math.Max(q.SamplerMaxGapMS, ph.Locks.MaxGapMS)
		q.SamplerFailures += ph.Locks.FailedSamples
		q.ConnectFailures += ph.ConnectFailures
		for _, s := range ph.Scenarios {
			if s.Background {
				continue
			}
			ok := s.Ops - s.Errors
			q.MinSamples = min(q.MinSamples, ok)
			if ok < minSamples {
				under[s.ID] = true
			}
		}
	}
	if q.MinSamples == math.MaxInt {
		q.MinSamples = 0
	}
	q.PairsCompleted = cands
	for id := range under {
		q.UndersampledScenarios = append(q.UndersampledScenarios, id)
	}
	sort.Strings(q.UndersampledScenarios)

	maxCV := math.NaN()
	for _, c := range comps {
		if c.Background || c.BaselineNoiseCV == nil {
			continue
		}
		if math.IsNaN(maxCV) || *c.BaselineNoiseCV > maxCV {
			maxCV = *c.BaselineNoiseCV
		}
		if *c.BaselineNoiseCV > UnstableCV {
			q.NoisyScenarios = append(q.NoisyScenarios, c.Scenario)
		}
	}
	q.MaxBaselineNoiseCV = ptr(maxCV)
	q.SamplerMaxGapMS = stats.Round(q.SamplerMaxGapMS, 1)
	return q
}
