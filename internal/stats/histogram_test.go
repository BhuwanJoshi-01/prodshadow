package stats

import (
	"math"
	"math/rand/v2"
	"sort"
	"testing"
	"time"
)

func TestQuantilesWithinOnePercent(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	h := NewHistogram()
	var exact []float64
	for i := 0; i < 50000; i++ {
		// Log-normal latencies around a few milliseconds.
		ms := math.Exp(r.NormFloat64()*0.8 + 1.5)
		exact = append(exact, ms)
		h.Record(time.Duration(ms * float64(time.Millisecond)))
	}
	sort.Float64s(exact)
	for _, q := range []float64{0.5, 0.95, 0.99} {
		want := exact[int(math.Ceil(q*float64(len(exact))))-1]
		got := h.Quantile(q)
		if math.Abs(got-want)/want > 0.011 {
			t.Errorf("q%.2f = %.4f, exact %.4f", q, got, want)
		}
	}
}

func TestMergeEqualsCombinedRecording(t *testing.T) {
	a, b, all := NewHistogram(), NewHistogram(), NewHistogram()
	for i := 1; i <= 1000; i++ {
		d := time.Duration(i) * time.Microsecond * 37
		if i%2 == 0 {
			a.Record(d)
		} else {
			b.Record(d)
		}
		all.Record(d)
	}
	a.Merge(b)
	if a.Summarize() != all.Summarize() {
		t.Fatalf("merge mismatch:\n%+v\n%+v", a.Summarize(), all.Summarize())
	}
	a.Merge(nil)
	a.Merge(NewHistogram())
	if a.Count() != 1000 {
		t.Fatalf("count = %d", a.Count())
	}
}

func TestEmptyAndExtremeValues(t *testing.T) {
	h := NewHistogram()
	if !math.IsNaN(h.Quantile(0.5)) || h.Summarize() != (Summary{}) {
		t.Fatal("empty histogram should report NaN quantiles and a zero summary")
	}
	h.Record(-time.Second)
	h.Record(0)
	h.Record(3 * time.Hour)
	s := h.Summarize()
	if s.Count != 3 || s.MinMS != 0 || s.MaxMS != 3*3600*1000 {
		t.Fatalf("unexpected summary %+v", s)
	}
}

func TestSingleValueIsExact(t *testing.T) {
	h := NewHistogram()
	h.Record(42 * time.Millisecond)
	if s := h.Summarize(); s.P50MS != 42 || s.P99MS != 42 {
		t.Fatalf("single observation should be exact after clamping: %+v", s)
	}
}
