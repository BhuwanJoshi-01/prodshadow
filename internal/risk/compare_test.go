package risk

import "testing"

func TestCompareBlocking(t *testing.T) {
	cs := Compare(blockingResult())
	if len(cs) != 1 {
		t.Fatalf("expected one comparison, got %d", len(cs))
	}
	c := cs[0]
	if c.Baseline.Repeats != 3 || c.Candidate.Repeats != 3 || c.Baseline.Ops != 6000 {
		t.Fatalf("aggregation wrong: %+v", c)
	}
	if c.Baseline.P95MS != 1.05 || c.Candidate.P95MS != 2.5 {
		t.Fatalf("medians wrong: base %v cand %v", c.Baseline.P95MS, c.Candidate.P95MS)
	}
	if c.P95Ratio == nil || *c.P95Ratio != 2.381 {
		t.Fatalf("p95 ratio = %v", c.P95Ratio)
	}
	if c.During == nil || c.StallMS != 1518 {
		t.Fatalf("stall = %v during %+v", c.StallMS, c.During)
	}
	if c.BaselineNoiseCV == nil || *c.BaselineNoiseCV > 0.06 {
		t.Fatalf("noise = %v", c.BaselineNoiseCV)
	}
	if c.ThroughputChangePct == nil || *c.ThroughputChangePct != -10 {
		t.Fatalf("throughput change = %v", c.ThroughputChangePct)
	}
}

func TestCompareEmpty(t *testing.T) {
	if Compare(nil) != nil {
		t.Fatal("nil result should produce no comparisons")
	}
}
