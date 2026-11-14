package workload

import (
	"math"
	"sort"
	"time"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/stats"
)

// Window selects operations by start time relative to the measurement
// start. A zero To means "until the end of the measurement window".
type Window struct {
	From, To time.Duration
}

// ScenarioStats summarises one scenario within a window.
type ScenarioStats struct {
	ID         string         `json:"id"`
	Mode       string         `json:"mode"`
	Background bool           `json:"background,omitempty"`
	Ops        int            `json:"ops"`
	Errors     int            `json:"errors"`
	ErrorRate  float64        `json:"error_rate"`
	ByClass    map[string]int `json:"errors_by_class,omitempty"`
	// Interrupted operations were cut off at the end of the run; their
	// latency is a lower bound and they are not counted as errors.
	Interrupted int     `json:"interrupted,omitempty"`
	Throughput  float64 `json:"throughput_per_sec"`
	// Latency covers successful operations.
	Latency stats.Summary `json:"latency"`
	// MaxObservedMS includes failed and interrupted operations, so a
	// session stuck behind a lock is visible even if it never succeeded.
	MaxObservedMS float64 `json:"max_observed_ms"`
	// SampleError is the first error message of the scenario in the run,
	// if any operation failed.
	SampleError string `json:"sample_error,omitempty"`

	hist *stats.Histogram
}

// Histogram exposes the latency histogram of successful operations.
func (s *ScenarioStats) Histogram() *stats.Histogram { return s.hist }

// Summarize aggregates operations that started inside w.
func Summarize(res *Result, w Window) []ScenarioStats {
	to := w.To
	if to == 0 {
		to = res.MeasuredDuration()
	}
	out := make([]ScenarioStats, len(res.Scenarios))
	for i, sc := range res.Scenarios {
		out[i] = ScenarioStats{ID: sc.ID, Mode: sc.Mode, Background: sc.Background, ByClass: map[string]int{}, hist: stats.NewHistogram(),
			SampleError: res.SampleErrors[sc.ID]}
	}
	for _, op := range res.Ops {
		start := time.Duration(op.Start)
		if start < w.From || start >= to || int(op.Scenario) >= len(out) {
			continue
		}
		s := &out[op.Scenario]
		lat := time.Duration(op.Latency)
		s.MaxObservedMS = math.Max(s.MaxObservedMS, float64(lat)/float64(time.Millisecond))
		switch op.Err {
		case OK:
			s.Ops++
			s.hist.Record(lat)
		case ErrInterrupted:
			s.Interrupted++
		default:
			s.Ops++
			s.Errors++
			s.ByClass[op.Err.String()]++
		}
	}
	secs := (to - w.From).Seconds()
	for i := range out {
		s := &out[i]
		if s.Ops > 0 {
			s.ErrorRate = stats.Round(float64(s.Errors)/float64(s.Ops), 6)
		}
		if secs > 0 {
			s.Throughput = stats.Round(float64(s.Ops-s.Errors)/secs, 3)
		}
		s.Latency = s.hist.Summarize()
		s.MaxObservedMS = stats.Round(s.MaxObservedMS, 3)
		if len(s.ByClass) == 0 {
			s.ByClass = nil
		}
	}
	return out
}

// Bucket is one interval of the foreground timeline.
type Bucket struct {
	OffsetSec     float64 `json:"offset_sec"`
	Ops           int     `json:"ops"`
	Errors        int     `json:"errors"`
	P95MS         float64 `json:"p95_ms"`
	MaxObservedMS float64 `json:"max_observed_ms"`
}

// Timeline buckets foreground operations by start time across the
// measurement window, which shows when traffic stalled.
func Timeline(res *Result, width time.Duration) []Bucket {
	if width <= 0 {
		width = time.Second
	}
	n := int(math.Ceil(float64(res.MeasuredDuration()) / float64(width)))
	if n <= 0 {
		return nil
	}
	buckets := make([]Bucket, n)
	hists := make([]*stats.Histogram, n)
	for i := range buckets {
		buckets[i].OffsetSec = stats.Round((time.Duration(i) * width).Seconds(), 3)
		hists[i] = stats.NewHistogram()
	}
	for _, op := range res.Ops {
		if int(op.Scenario) >= len(res.Scenarios) || res.Scenarios[op.Scenario].Background || op.Start < 0 {
			continue
		}
		i := int(time.Duration(op.Start) / width)
		if i >= n {
			continue
		}
		b := &buckets[i]
		lat := float64(op.Latency) / float64(time.Millisecond)
		b.MaxObservedMS = math.Max(b.MaxObservedMS, stats.Round(lat, 3))
		switch op.Err {
		case OK:
			b.Ops++
			hists[i].Record(time.Duration(op.Latency))
		case ErrInterrupted:
		default:
			b.Ops++
			b.Errors++
		}
	}
	for i := range buckets {
		if hists[i].Count() > 0 {
			buckets[i].P95MS = stats.Round(hists[i].Quantile(0.95), 3)
		}
	}
	return buckets
}

// SortedErrorClasses returns error class names in a stable order.
func SortedErrorClasses(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
