package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/report"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/risk"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/workload"
)

func TestSummaryShowsStallsAndPredictions(t *testing.T) {
	rep := &report.Report{
		Decision: risk.Decision{Outcome: "block", ExitCode: ExitBlock, Message: "Blocking findings: OBS001."},
		Comparisons: []risk.Comparison{
			{Scenario: "update_status", During: &risk.Side{MaxObservedMS: 820}},
			{Scenario: "get_order", During: &risk.Side{MaxObservedMS: 3}},
			{Scenario: "nightly", Background: true, During: &risk.Side{MaxObservedMS: 6000}},
		},
		PredictedConflicts: []workload.Conflict{{Session: "update_status", StatementID: "1.1", Table: "orders", MigrationLock: "SHARE", WorkloadLock: "ROW EXCLUSIVE"}},
	}
	var buf bytes.Buffer
	writeSummary(&buf, rep, nil)
	out := buf.String()
	for _, want := range []string{"during the migration (longest operation; predicted conflict):", "update_status                 820 ms  1.1 SHARE on orders", "get_order                       3 ms  none"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "nightly") {
		t.Errorf("background sessions are not listed:\n%s", out)
	}
}
