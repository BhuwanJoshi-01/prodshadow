package workload

import (
	"context"
	"testing"
	"time"
)

func TestWaitForEndWithoutExtension(t *testing.T) {
	r := &runner{opts: Options{Measurement: 50 * time.Millisecond}, measureStart: time.Now()}
	end := r.waitForEnd(context.Background())
	if got := end.Sub(r.measureStart); got != 50*time.Millisecond {
		t.Fatalf("window = %v", got)
	}
}

func TestWaitForEndEarlyExtensionAddsNoTail(t *testing.T) {
	ext := make(chan struct{})
	close(ext)
	r := &runner{opts: Options{Measurement: 80 * time.Millisecond, Extend: ext, Tail: 20 * time.Millisecond, MaxExtension: time.Second}, measureStart: time.Now()}
	end := r.waitForEnd(context.Background())
	if got := end.Sub(r.measureStart); got != 80*time.Millisecond {
		t.Fatalf("an early-finishing migration must not extend the window: %v", got)
	}
}

func TestWaitForEndLateExtension(t *testing.T) {
	ext := make(chan struct{})
	start := time.Now()
	go func() { time.Sleep(100 * time.Millisecond); close(ext) }()
	r := &runner{opts: Options{Measurement: 30 * time.Millisecond, Extend: ext, Tail: 40 * time.Millisecond, MaxExtension: time.Second}, measureStart: start}
	end := r.waitForEnd(context.Background())
	if got := end.Sub(start); got < 135*time.Millisecond || got > 400*time.Millisecond {
		t.Fatalf("window should end about tail after the extension closed: %v", got)
	}
}

func TestWaitForEndCapsExtension(t *testing.T) {
	r := &runner{opts: Options{Measurement: 10 * time.Millisecond, Extend: make(chan struct{}), MaxExtension: 30 * time.Millisecond}, measureStart: time.Now()}
	end := r.waitForEnd(context.Background())
	if got := end.Sub(r.measureStart); got > 300*time.Millisecond {
		t.Fatalf("extension not capped: %v", got)
	}
}
