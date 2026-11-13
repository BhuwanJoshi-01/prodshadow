package workload

import (
	"math"
	"testing"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/gen"
)

func TestCompileOrdersAndWeights(t *testing.T) {
	s, err := Parse([]byte(blueprintWorkload))
	if err != nil {
		t.Fatal(err)
	}
	fg, bg, err := compile(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(fg) != 3 || len(bg) != 1 || !bg[0].info.Background || bg[0].hold.Milliseconds() != 8000 {
		t.Fatalf("unexpected compiled plan fg=%d bg=%d", len(fg), len(bg))
	}
	if fg[2].cum != 1 || math.Abs(fg[0].cum-0.6) > 1e-9 {
		t.Fatalf("cumulative weights wrong: %v %v", fg[0].cum, fg[2].cum)
	}
	if !fg[2].txn || fg[2].stmts[0].params != 1 || fg[1].stmts[0].params != 2 {
		t.Fatalf("statement parameters wrong: %+v", fg[1].stmts)
	}
}

func TestPickFollowsWeights(t *testing.T) {
	s, _ := Parse([]byte(blueprintWorkload))
	fg, _, _ := compile(s)
	r := gen.Stream(3, "pick")
	counts := make([]int, len(fg))
	const n = 50000
	for i := 0; i < n; i++ {
		counts[pick(fg, r.Float64())]++
	}
	for i, want := range []float64{0.6, 0.3, 0.1} {
		if got := float64(counts[i]) / n; math.Abs(got-want) > 0.01 {
			t.Errorf("scenario %d chosen %.3f of the time, want %.2f", i, got, want)
		}
	}
	if pick(fg, 0.9999999) != 2 || pick(fg, 0) != 0 {
		t.Fatal("boundary draws mapped to the wrong scenario")
	}
}
