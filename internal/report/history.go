package report

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/experiment"
)

// HistoryEntry summarises one run for a timeline of stored reports.
type HistoryEntry struct {
	Source        string    `json:"source,omitempty"`
	RunID         string    `json:"run_id"`
	Project       string    `json:"project"`
	GitSHA        string    `json:"git_sha,omitempty"`
	StartedAt     time.Time `json:"started_at"`
	Decision      string    `json:"decision"`
	PostgresMajor string    `json:"postgres_major"`
	Migration     []string  `json:"migration_files"`
	FixtureHash   string    `json:"fixture_hash"`
	WorkloadHash  string    `json:"workload_hash,omitempty"`
	// Worst values over the candidate phases.
	MigrationMS        float64 `json:"migration_ms"`
	MaxLockWaitMS      float64 `json:"max_lock_wait_ms"`
	MaxBlockedSessions int     `json:"max_blocked_by_migration"`
	// Unsuppressed findings by severity.
	Critical     int `json:"critical"`
	Warning      int `json:"warning"`
	Info         int `json:"info"`
	Inconclusive int `json:"inconclusive"`
}

// HistoryOf summarises a report. source is where it was read from.
func HistoryOf(r *Report, source string) HistoryEntry {
	e := HistoryEntry{
		Source: source, RunID: r.RunID, Project: r.Project, GitSHA: r.GitSHA, StartedAt: r.StartedAt,
		Decision: r.Decision.Outcome, PostgresMajor: r.Engine.PostgresMajor, Migration: r.Migration.Files,
		FixtureHash: r.Fixture.Hash, WorkloadHash: r.Workload.Hash,
	}
	for _, p := range r.Phases {
		if p.Kind != string(experiment.Candidate) {
			continue
		}
		if p.Migration != nil {
			e.MigrationMS = max(e.MigrationMS, p.Migration.DurationMS)
		}
		e.MaxLockWaitMS = max(e.MaxLockWaitMS, p.Locks.MaxLockWaitMS)
		e.MaxBlockedSessions = max(e.MaxBlockedSessions, p.Locks.MaxBlockedByMigration)
	}
	for _, f := range r.Findings {
		if f.Suppressed {
			continue
		}
		switch f.Severity {
		case evidence.Critical:
			e.Critical++
		case evidence.Warning:
			e.Warning++
		case evidence.Info:
			e.Info++
		case evidence.Inconclusive:
			e.Inconclusive++
		}
	}
	return e
}

// SortHistory orders entries by start time, oldest first, then by run ID.
func SortHistory(entries []HistoryEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		if !entries[i].StartedAt.Equal(entries[j].StartedAt) {
			return entries[i].StartedAt.Before(entries[j].StartedAt)
		}
		return entries[i].RunID < entries[j].RunID
	})
}

// WriteHistory prints entries as an aligned table, oldest first. A run whose
// fixture, workload or PostgreSQL version differs from the run before it is
// marked, because its numbers are not directly comparable.
func WriteHistory(w io.Writer, entries []HistoryEntry) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "STARTED (UTC)\tPROJECT\tGIT\tDECISION\tMIGRATION\tLOCK WAIT\tBLOCKED\tFINDINGS C/W/I\t")
	changed := false
	var prev *HistoryEntry
	for i := range entries {
		e := entries[i]
		mark := ""
		if prev != nil && (prev.FixtureHash != e.FixtureHash || prev.WorkloadHash != e.WorkloadHash || prev.PostgresMajor != e.PostgresMajor) {
			mark, changed = "*", true
		}
		prev = &entries[i]
		git := e.GitSHA
		if len(git) > 8 {
			git = git[:8]
		}
		findings := fmt.Sprintf("%d/%d/%d", e.Critical, e.Warning, e.Info)
		if e.Inconclusive > 0 {
			findings += fmt.Sprintf(" +%d inconclusive", e.Inconclusive)
		}
		fmt.Fprintf(tw, "%s%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\t\n",
			e.StartedAt.UTC().Format("2006-01-02 15:04"), mark, e.Project, orDash(git), e.Decision,
			fmtMS(e.MigrationMS), fmtMS(e.MaxLockWaitMS), e.MaxBlockedSessions, findings)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if changed {
		_, err := fmt.Fprintln(w, "\n* the fixture, workload or PostgreSQL version changed since the previous run; compare with care.")
		return err
	}
	return nil
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}
