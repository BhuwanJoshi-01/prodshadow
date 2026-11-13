package workload

import (
	"testing"
	"time"
)

func sampleResult() *Result {
	ms := func(d int) int64 { return int64(time.Duration(d) * time.Millisecond) }
	start := time.Unix(0, 0)
	return &Result{
		Scenarios:    []ScenarioInfo{{ID: "read", Mode: "read"}, {ID: "blocker", Mode: "background", Background: true}},
		MeasureStart: start,
		MeasureEnd:   start.Add(4 * time.Second),
		Ops: []Op{
			{Scenario: 0, Start: -ms(500), Latency: ms(1)}, // warm-up, excluded
			{Scenario: 0, Start: ms(100), Latency: ms(2)},
			{Scenario: 0, Start: ms(200), Latency: ms(4)},
			{Scenario: 0, Start: ms(1500), Latency: ms(1500), Err: ErrLockTimeout},
			{Scenario: 0, Start: ms(2500), Latency: ms(3000), Err: ErrInterrupted},
			{Scenario: 0, Start: ms(3900), Latency: ms(3)},
			{Scenario: 1, Start: ms(0), Latency: ms(10)},
			{Scenario: 0, Start: ms(4100), Latency: ms(1)}, // after window
		},
	}
}

func TestSummarizeWholeWindow(t *testing.T) {
	got := Summarize(sampleResult(), Window{})
	r := got[0]
	if r.Ops != 4 || r.Errors != 1 || r.Interrupted != 1 || r.ByClass["lock_timeout"] != 1 {
		t.Fatalf("unexpected counts %+v", r)
	}
	if r.ErrorRate != 0.25 || r.Throughput != 0.75 {
		t.Fatalf("rate=%v throughput=%v", r.ErrorRate, r.Throughput)
	}
	if r.Latency.Count != 3 || r.MaxObservedMS != 3000 {
		t.Fatalf("latency %+v max %v", r.Latency, r.MaxObservedMS)
	}
	if !got[1].Background || got[1].Ops != 1 {
		t.Fatalf("background stats %+v", got[1])
	}
}

func TestSummarizeSubWindow(t *testing.T) {
	got := Summarize(sampleResult(), Window{From: time.Second, To: 2 * time.Second})
	if got[0].Ops != 1 || got[0].Errors != 1 || got[0].Throughput != 0 {
		t.Fatalf("unexpected window stats %+v", got[0])
	}
}

func TestTimeline(t *testing.T) {
	b := Timeline(sampleResult(), time.Second)
	if len(b) != 4 {
		t.Fatalf("expected 4 buckets, got %d", len(b))
	}
	if b[0].Ops != 2 || b[1].Errors != 1 || b[2].Ops != 0 || b[2].MaxObservedMS != 3000 || b[3].Ops != 1 {
		t.Fatalf("unexpected buckets %+v", b)
	}
}
