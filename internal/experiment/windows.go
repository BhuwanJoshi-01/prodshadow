package experiment

import (
	"time"

	"github.com/BhuwanJoshi-01/prodshadow/internal/migrate"
	"github.com/BhuwanJoshi-01/prodshadow/internal/workload"
)

// migrationWindows splits a candidate measurement into the part before the
// migration started and the part while it ran. Offsets in the migration
// outcome are relative to the measurement start. When the migration did
// not run, both windows are empty.
func migrationWindows(out *migrate.Outcome, measured time.Duration) (pre, during workload.Window, ok bool) {
	if out == nil || out.Started.IsZero() {
		return workload.Window{}, workload.Window{}, false
	}
	start := time.Duration(out.OffsetMS * float64(time.Millisecond))
	end := start + time.Duration(out.DurationMS*float64(time.Millisecond))
	if start < 0 {
		start = 0
	}
	if end > measured {
		end = measured
	}
	// Operations that start within a short margin after the migration
	// ends can still be waiting behind its locks; include them.
	end += 50 * time.Millisecond
	if end > measured {
		end = measured
	}
	if end <= start {
		end = start + time.Millisecond
	}
	if start == 0 {
		return workload.Window{}, workload.Window{From: 0, To: end}, true
	}
	return workload.Window{From: 0, To: start}, workload.Window{From: start, To: end}, true
}
