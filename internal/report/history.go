package report

import (
	_ "embed"
	"fmt"
	"html/template"
	"io"
	"math"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/BhuwanJoshi-01/prodshadow/internal/evidence"
	"github.com/BhuwanJoshi-01/prodshadow/internal/experiment"
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
	marks, changed := comparability(entries)
	for i, e := range entries {
		mark := ""
		if marks[i] {
			mark = "*"
		}
		fmt.Fprintf(tw, "%s%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\t\n",
			e.StartedAt.UTC().Format("2006-01-02 15:04"), mark, e.Project, orDash(shortSHA(e.GitSHA)), e.Decision,
			fmtMS(e.MigrationMS), fmtMS(e.MaxLockWaitMS), e.MaxBlockedSessions, e.FindingCounts())
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

// WriteHistoryMarkdown renders entries as a Markdown table for job
// summaries and pull request comments.
func WriteHistoryMarkdown(w io.Writer, entries []HistoryEntry) error {
	var b strings.Builder
	b.WriteString("| Started (UTC) | Project | Git | Decision | Migration | Lock wait | Blocked | Findings C/W/I |\n|---|---|---|---|---|---|---|---|\n")
	marks, changed := comparability(entries)
	for i, e := range entries {
		mark := ""
		if marks[i] {
			mark = " \\*"
		}
		git := shortSHA(e.GitSHA)
		if git != "" {
			git = "`" + git + "`"
		}
		fmt.Fprintf(&b, "| %s%s | %s | %s | %s | %s | %s | %d | %s |\n",
			e.StartedAt.UTC().Format("2006-01-02 15:04"), mark, esc(e.Project), orDash(git), esc(e.Decision),
			fmtMS(e.MigrationMS), fmtMS(e.MaxLockWaitMS), e.MaxBlockedSessions, e.FindingCounts())
	}
	if changed {
		b.WriteString("\n\\* the fixture, workload or PostgreSQL version changed since the previous run; compare with care.\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// comparability marks entries whose fixture, workload or PostgreSQL version
// differs from the entry before, and reports whether any does.
func comparability(entries []HistoryEntry) ([]bool, bool) {
	marks := make([]bool, len(entries))
	changed := false
	for i := 1; i < len(entries); i++ {
		p, e := entries[i-1], entries[i]
		if p.FixtureHash != e.FixtureHash || p.WorkloadHash != e.WorkloadHash || p.PostgresMajor != e.PostgresMajor {
			marks[i], changed = true, true
		}
	}
	return marks, changed
}

func shortSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

// FindingCounts formats the unsuppressed findings as critical/warning/info.
func (e HistoryEntry) FindingCounts() string {
	s := fmt.Sprintf("%d/%d/%d", e.Critical, e.Warning, e.Info)
	if e.Inconclusive > 0 {
		s += fmt.Sprintf(" +%d inconclusive", e.Inconclusive)
	}
	return s
}

//go:embed history.html.tmpl
var historyTemplate string

var parsedHistoryHTML = template.Must(template.New("history").Funcs(template.FuncMap{
	"ms":    fmtMS,
	"short": shortSHA,
}).Parse(historyTemplate))

// WriteHistoryHTML renders entries as a self-contained page with a chart of
// migration duration and lock wait per run and the run table.
func WriteHistoryHTML(w io.Writer, entries []HistoryEntry) error {
	marks, _ := comparability(entries)
	project := ""
	for i, e := range entries {
		if i == 0 {
			project = e.Project
		} else if e.Project != project {
			project = ""
			break
		}
	}
	migration := make([]float64, len(entries))
	lockWait := make([]float64, len(entries))
	for i, e := range entries {
		migration[i], lockWait[i] = e.MigrationMS, e.MaxLockWaitMS
	}
	data := struct {
		Project        string
		Entries        []HistoryEntry
		Marks          []bool
		MigrationChart template.HTML
		LockChart      template.HTML
	}{project, entries, marks, seriesSVG(migration, marks), seriesSVG(lockWait, marks)}
	return parsedHistoryHTML.Execute(w, data)
}

// seriesSVG draws one value per run as a line with dots; runs that are not
// comparable with the previous run get a highlighted dot.
func seriesSVG(values []float64, marks []bool) template.HTML {
	const width, height, pad = 480.0, 160.0, 24.0
	top := 0.0
	for _, v := range values {
		top = math.Max(top, v)
	}
	if top == 0 {
		top = 1
	}
	x := func(i int) float64 {
		if len(values) == 1 {
			return width / 2
		}
		return pad + float64(i)*(width-2*pad)/float64(len(values)-1)
	}
	y := func(v float64) float64 { return height - pad - v/top*(height-2*pad) }
	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="0 0 %.0f %.0f" role="img">`, width, height)
	fmt.Fprintf(&b, `<text x="4" y="14">%s</text>`, template.HTMLEscapeString(fmtMS(top)))
	if len(values) > 1 {
		b.WriteString(`<polyline class="line" points="`)
		for i, v := range values {
			fmt.Fprintf(&b, "%.1f,%.1f ", x(i), y(v))
		}
		b.WriteString(`"/>`)
	}
	for i, v := range values {
		class := "dot"
		if i < len(marks) && marks[i] {
			class = "changed"
		}
		fmt.Fprintf(&b, `<circle class="%s" cx="%.1f" cy="%.1f" r="3"/>`, class, x(i), y(v))
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String()) //nolint:gosec // built from numbers and escaped text only
}
