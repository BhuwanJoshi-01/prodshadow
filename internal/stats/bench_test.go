package stats

import (
	"testing"
	"time"
)

// Recording must stay cheap: every workload operation records one sample.
func BenchmarkHistogramRecord(b *testing.B) {
	h := NewHistogram()
	for i := 0; b.Loop(); i++ {
		h.Record(time.Duration(i%50000) * time.Microsecond)
	}
}

func BenchmarkHistogramSummarize(b *testing.B) {
	h := NewHistogram()
	for i := 0; i < 1_000_000; i++ {
		h.Record(time.Duration(i%50000) * time.Microsecond)
	}
	for b.Loop() {
		_ = h.Summarize()
	}
}
