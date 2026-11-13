package sqlinspect

import (
	"testing"
	"time"
)

// findings analyses the given files and returns findings for one rule.
func findings(t *testing.T, rule string, opts Options, files ...string) []Finding {
	t.Helper()
	if opts.Now.IsZero() {
		opts.Now = time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	}
	var out []Finding
	for _, f := range Analyze(parse(t, files...), opts) {
		if f.RuleID == rule {
			out = append(out, f)
		}
	}
	return out
}

func expectCount(t *testing.T, got []Finding, want int) {
	t.Helper()
	if len(got) != want {
		t.Fatalf("expected %d findings, got %d: %+v", want, len(got), got)
	}
}
