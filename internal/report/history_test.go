package report

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/migrate"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/risk"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/telemetry"
)

func historyReport(id string, at time.Time, fixture string, migrationMS float64) *Report {
	r := &Report{SchemaVersion: SchemaVersion, RunID: id, Project: "shop", GitSHA: "0123456789abcdef", StartedAt: at}
	r.Decision.Outcome = "pass"
	r.Engine.PostgresMajor = "17"
	r.Fixture.Hash = fixture
	r.Phases = []Phase{
		{Kind: "baseline", Locks: telemetry.Summary{MaxLockWaitMS: 9999}},
		{Kind: "candidate", Migration: &migrate.Outcome{DurationMS: migrationMS}, Locks: telemetry.Summary{MaxLockWaitMS: 40, MaxBlockedByMigration: 3}},
		{Kind: "candidate", Migration: &migrate.Outcome{DurationMS: migrationMS / 2}, Locks: telemetry.Summary{MaxLockWaitMS: 120, MaxBlockedByMigration: 1}},
	}
	r.Findings = []risk.Finding{
		{RuleID: "OBS002", Severity: evidence.Critical},
		{RuleID: "PS103", Severity: evidence.Warning},
		{RuleID: "PS114", Severity: evidence.Warning, Suppressed: true},
		{RuleID: "QUAL001", Severity: evidence.Inconclusive},
	}
	return r
}

func TestHistoryOfUsesWorstCandidatePhase(t *testing.T) {
	e := HistoryOf(historyReport("r1", time.Unix(0, 0), "f1", 800), "a.json")
	if e.MigrationMS != 800 || e.MaxLockWaitMS != 120 || e.MaxBlockedSessions != 3 {
		t.Fatalf("candidate maxima: %+v", e)
	}
	if e.Critical != 1 || e.Warning != 1 || e.Info != 0 || e.Inconclusive != 1 {
		t.Fatalf("finding counts (suppressed must not count): %+v", e)
	}
}

func TestWriteHistoryMarksIncomparableRuns(t *testing.T) {
	base := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	entries := []HistoryEntry{
		HistoryOf(historyReport("r2", base.Add(time.Hour), "f1", 900), ""),
		HistoryOf(historyReport("r1", base, "f1", 800), ""),
		HistoryOf(historyReport("r3", base.Add(2*time.Hour), "f2", 50), ""),
	}
	SortHistory(entries)
	if entries[0].RunID != "r1" || entries[2].RunID != "r3" {
		t.Fatalf("not sorted by start time: %v %v %v", entries[0].RunID, entries[1].RunID, entries[2].RunID)
	}
	var buf bytes.Buffer
	if err := WriteHistory(&buf, entries); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"2026-11-01 09:00 ", "2026-11-01 11:00*", "01234567 ", "1/1/0 +1 inconclusive", "compare with care"} {
		if !strings.Contains(out, want) {
			t.Errorf("history output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "2026-11-01 10:00*") {
		t.Errorf("unchanged fixture marked as changed:\n%s", out)
	}
}

func TestWriteHistoryMarkdown(t *testing.T) {
	base := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	entries := []HistoryEntry{
		HistoryOf(historyReport("r1", base, "f1", 800), ""),
		HistoryOf(historyReport("r2", base.Add(time.Hour), "f2", 900), ""),
	}
	var buf bytes.Buffer
	if err := WriteHistoryMarkdown(&buf, entries); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"| Started (UTC) |", "| 2026-11-01 09:00 | shop | `01234567` | pass | 800 ms |", "| 2026-11-01 10:00 \\* |", "compare with care"} {
		if !strings.Contains(out, want) {
			t.Errorf("markdown lacks %q:\n%s", want, out)
		}
	}
}

func TestWriteHistoryHTML(t *testing.T) {
	base := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	entries := []HistoryEntry{
		HistoryOf(historyReport("r1", base, "f1", 800), ""),
		HistoryOf(historyReport("r2", base.Add(time.Hour), "f1", 900), ""),
		HistoryOf(historyReport("r3", base.Add(2*time.Hour), "f2", 300), ""),
	}
	entries[1].Project = `<script>alert(1)</script>`
	var buf bytes.Buffer
	if err := WriteHistoryHTML(&buf, entries); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"<polyline", `class="changed"`, "2026-11-01 11:00 *", "&lt;script&gt;", "1/1/0"} {
		if !strings.Contains(out, want) {
			t.Errorf("HTML lacks %q", want)
		}
	}
	if strings.Contains(out, "<script>alert(1)") || strings.Contains(out, "http://") || strings.Contains(out, "https://") {
		t.Error("history page must escape text and load no external resources")
	}
	buf.Reset()
	if err := WriteHistoryHTML(&buf, entries[:1]); err != nil || strings.Contains(buf.String(), "<polyline") {
		t.Errorf("a single run draws a dot, not a line: %v", err)
	}
}
