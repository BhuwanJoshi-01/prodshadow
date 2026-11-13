// Package stats provides latency histograms and the small set of summary
// statistics used to compare baseline and candidate runs.
package stats

import (
	"math"
	"time"
)

// growth is the relative width of histogram buckets: each bucket is 1%
// wider than the previous one, so percentiles are accurate to about 1%.
const growth = 1.01

var logGrowth = math.Log(growth)

// maxBucket covers values up to roughly one hour in microseconds.
const maxBucket = 2200

// Histogram records durations in log-spaced buckets. It is mergeable,
// constant in size and not safe for concurrent use.
type Histogram struct {
	counts [maxBucket + 1]uint64
	n      uint64
	sum    float64
	min    float64
	max    float64
}

// NewHistogram returns an empty histogram.
func NewHistogram() *Histogram { return &Histogram{min: math.Inf(1)} }

func bucketOf(us float64) int {
	if us < 1 {
		return 0
	}
	b := int(math.Log(us)/logGrowth) + 1
	if b > maxBucket {
		return maxBucket
	}
	return b
}

// bucketValue returns a representative value (geometric midpoint) in µs.
func bucketValue(b int) float64 {
	if b == 0 {
		return 0.5
	}
	lo := math.Pow(growth, float64(b-1))
	return lo * math.Sqrt(growth)
}

// Record adds one duration.
func (h *Histogram) Record(d time.Duration) {
	us := float64(d) / float64(time.Microsecond)
	if us < 0 {
		us = 0
	}
	h.counts[bucketOf(us)]++
	h.n++
	h.sum += us
	h.min = math.Min(h.min, us)
	h.max = math.Max(h.max, us)
}

// Merge adds all observations from o.
func (h *Histogram) Merge(o *Histogram) {
	if o == nil || o.n == 0 {
		return
	}
	for i, c := range o.counts {
		h.counts[i] += c
	}
	h.n += o.n
	h.sum += o.sum
	h.min = math.Min(h.min, o.min)
	h.max = math.Max(h.max, o.max)
}

// Count returns the number of observations.
func (h *Histogram) Count() uint64 { return h.n }

// Quantile returns the q-quantile (0..1) in milliseconds. The exact
// minimum and maximum are returned for q <= 0 and q >= 1.
func (h *Histogram) Quantile(q float64) float64 {
	if h.n == 0 {
		return math.NaN()
	}
	if q <= 0 {
		return h.min / 1000
	}
	if q >= 1 {
		return h.max / 1000
	}
	rank := uint64(math.Ceil(q * float64(h.n)))
	var cum uint64
	for b, c := range h.counts {
		cum += c
		if cum >= rank {
			v := bucketValue(b)
			// Clamp to the observed range for small samples.
			return math.Min(math.Max(v, h.min), h.max) / 1000
		}
	}
	return h.max / 1000
}

// Summary condenses a histogram for reports. Values are milliseconds.
type Summary struct {
	Count  uint64  `json:"count"`
	MinMS  float64 `json:"min_ms"`
	MeanMS float64 `json:"mean_ms"`
	P50MS  float64 `json:"p50_ms"`
	P95MS  float64 `json:"p95_ms"`
	P99MS  float64 `json:"p99_ms"`
	MaxMS  float64 `json:"max_ms"`
}

// Summarize returns count, mean and key percentiles.
func (h *Histogram) Summarize() Summary {
	if h.n == 0 {
		return Summary{}
	}
	return Summary{
		Count:  h.n,
		MinMS:  round3(h.min / 1000),
		MeanMS: round3(h.sum / float64(h.n) / 1000),
		P50MS:  round3(h.Quantile(0.50)),
		P95MS:  round3(h.Quantile(0.95)),
		P99MS:  round3(h.Quantile(0.99)),
		MaxMS:  round3(h.max / 1000),
	}
}

func round3(v float64) float64 { return math.Round(v*1000) / 1000 }
