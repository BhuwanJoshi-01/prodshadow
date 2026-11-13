package stats

import (
	"math"
	"sort"
)

// Median returns the median of xs, or NaN for an empty slice.
func Median(xs []float64) float64 {
	if len(xs) == 0 {
		return math.NaN()
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	m := len(s) / 2
	if len(s)%2 == 1 {
		return s[m]
	}
	return (s[m-1] + s[m]) / 2
}

// Mean returns the arithmetic mean, or NaN for an empty slice.
func Mean(xs []float64) float64 {
	if len(xs) == 0 {
		return math.NaN()
	}
	sum := 0.0
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}

// StdDev returns the sample standard deviation (n-1), or NaN when fewer
// than two values are given.
func StdDev(xs []float64) float64 {
	if len(xs) < 2 {
		return math.NaN()
	}
	m := Mean(xs)
	ss := 0.0
	for _, x := range xs {
		ss += (x - m) * (x - m)
	}
	return math.Sqrt(ss / float64(len(xs)-1))
}

// CV returns the coefficient of variation (stddev / mean). It is the
// measure of baseline noise used to decide whether a difference between
// baseline and candidate is meaningful.
func CV(xs []float64) float64 {
	m := Mean(xs)
	if m == 0 || math.IsNaN(m) {
		return math.NaN()
	}
	return StdDev(xs) / math.Abs(m)
}

// MAD returns the median absolute deviation, a robust spread estimate.
func MAD(xs []float64) float64 {
	if len(xs) == 0 {
		return math.NaN()
	}
	med := Median(xs)
	dev := make([]float64, len(xs))
	for i, x := range xs {
		dev[i] = math.Abs(x - med)
	}
	return Median(dev)
}

// Ratio returns a/b, or NaN when b is zero or either value is NaN.
func Ratio(a, b float64) float64 {
	if b == 0 || math.IsNaN(a) || math.IsNaN(b) {
		return math.NaN()
	}
	return a / b
}

// Round rounds to the given number of decimals, keeping NaN.
func Round(v float64, decimals int) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return v
	}
	p := math.Pow(10, float64(decimals))
	return math.Round(v*p) / p
}
