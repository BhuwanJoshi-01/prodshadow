package stats

import (
	"math"
	"testing"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestMedianMeanStdDev(t *testing.T) {
	xs := []float64{4, 1, 3, 2}
	if Median(xs) != 2.5 || Mean(xs) != 2.5 {
		t.Fatalf("median=%v mean=%v", Median(xs), Mean(xs))
	}
	if xs[0] != 4 {
		t.Fatal("Median must not reorder its input")
	}
	if Median([]float64{3, 1, 2}) != 2 {
		t.Fatal("odd median wrong")
	}
	if !approx(StdDev([]float64{2, 4, 4, 4, 5, 5, 7, 9}), 2.138089935299395) {
		t.Fatalf("stddev = %v", StdDev([]float64{2, 4, 4, 4, 5, 5, 7, 9}))
	}
}

func TestEmptyInputsAreNaN(t *testing.T) {
	for name, v := range map[string]float64{
		"median": Median(nil), "mean": Mean(nil), "stddev": StdDev([]float64{1}),
		"cv": CV(nil), "mad": MAD(nil), "ratio": Ratio(1, 0),
	} {
		if !math.IsNaN(v) {
			t.Errorf("%s should be NaN, got %v", name, v)
		}
	}
}

func TestCVAndMAD(t *testing.T) {
	if !approx(CV([]float64{10, 10, 10}), 0) {
		t.Fatal("constant series should have zero CV")
	}
	if MAD([]float64{1, 1, 2, 2, 4, 6, 9}) != 1 {
		t.Fatalf("mad = %v", MAD([]float64{1, 1, 2, 2, 4, 6, 9}))
	}
}

func TestRound(t *testing.T) {
	if Round(1.23456, 2) != 1.23 || !math.IsNaN(Round(math.NaN(), 2)) {
		t.Fatal("round wrong")
	}
}
