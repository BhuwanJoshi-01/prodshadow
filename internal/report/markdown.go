package report

import (
	"fmt"
	"sort"
	"strings"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/risk"
)

// Disclaimer is printed at the top of every human-readable report.
const Disclaimer = "ProdShadow measures how a migration behaves in a disposable database under a modelled workload. A passing result is not a guarantee that the migration is safe in production."

// Markdown renders the report for pull request comments and CI summaries.
func (r *Report) Markdown() string {
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }

	w("# ProdShadow report: %s\n\n", esc(r.Project))
	w("**Decision: %s** · exit code %d · status `%s`\n\n", strings.ToUpper(r.Decision.Outcome), r.Decision.ExitCode, r.Status)
	w("%s\n\n", r.Decision.Message)
	w("> %s\n\n", Disclaimer)

	r.mdSummary(w)
	r.mdErrors(w)
	r.mdFindings(w)
	r.mdComparisons(w)
	r.mdMigration(w)
	r.mdLocks(w)
	r.mdCompatibility(w)
	r.mdTimeline(w)
	r.mdQuality(w)
	r.mdReproduction(w)
	return b.String()
}

type writer func(format string, args ...any)

func (r *Report) mdSummary(w writer) {
	pg := r.Engine.PostgresMajor
	if r.Engine.ServerVersion != "" {
		if f := strings.Fields(r.Engine.ServerVersion); len(f) > 1 {
			pg = f[1]
		}
	}
	rows := int64(0)
	for _, t := range r.Fixture.Tables {
		rows += t.RowsEstimate
	}
	w("| Run | PostgreSQL | Runtime | Fixture | Workload | Duration |\n|---|---|---|---|---|---|\n")
	w("| `%s` | %s | %s, network %s | %s, %s rows | %s, %d sessions, %d × %.0f s | %s |\n\n",
		r.RunID, esc(pg), r.Engine.Runtime, r.Engine.Network, r.Representativeness.Level, fmtInt(rows),
		r.Workload.Mode, r.Workload.Concurrency, r.Workload.Repeats, r.Workload.MeasurementSec, fmtDuration(r.DurationSec))
}

func (r *Report) mdErrors(w writer) {
	if len(r.Errors) == 0 {
		return
	}
	w("## Errors\n\n")
	for _, e := range r.Errors {
		w("- %s\n", esc(e))
	}
	w("\n")
}

func (r *Report) mdFindings(w writer) {
	var active, suppressed []risk.Finding
	for _, f := range r.Findings {
		if f.Suppressed {
			suppressed = append(suppressed, f)
		} else {
			active = append(active, f)
		}
	}
	w("## Findings\n\n")
	if len(active) == 0 {
		w("No findings.\n\n")
	} else {
		w("| Severity | Rule | Confidence | Evidence | Where | Finding |\n|---|---|---|---|---|---|\n")
		for _, f := range active {
			conf := string(f.Confidence)
			if f.ConfidenceReason != "" {
				conf += " (" + f.ConfidenceReason + ")"
			}
			where := f.Where("–")
			if f.Scenario != "" {
				where = "scenario `" + f.Scenario + "`"
			}
			w("| %s | %s %s | %s | %s | %s | %s |\n", severityLabel(f.Severity), f.RuleID, f.Rule, esc(conf), f.Evidence, esc(where), esc(f.Message))
		}
		w("\n")
		var remediations []risk.Finding
		for _, f := range active {
			if f.Remediation != "" && f.Severity != evidence.Info {
				remediations = append(remediations, f)
			}
		}
		if len(remediations) > 0 {
			w("### Remediation\n\n")
			for _, f := range remediations {
				w("- **%s** (%s): %s", f.RuleID, location(f.File, f.Line), f.Remediation)
				if f.DocsURL != "" {
					w(" [PostgreSQL docs](%s)", f.DocsURL)
				}
				w("\n")
			}
			w("\n")
		}
	}
	if len(suppressed) > 0 {
		w("<details><summary>%d suppressed finding(s)</summary>\n\n", len(suppressed))
		for _, f := range suppressed {
			exp := ""
			if f.SuppressionExpiry != "" {
				exp = ", expires " + f.SuppressionExpiry
			}
			w("- %s %s at %s — %s (reason: %s%s)\n", f.RuleID, f.Rule, location(f.File, f.Line), esc(f.Message), esc(f.SuppressionReason), exp)
		}
		w("\n</details>\n\n")
	}
	if len(r.Decision.ExpiredSuppressions) > 0 {
		w("**Expired suppressions:** ")
		ids := make([]string, 0, len(r.Decision.ExpiredSuppressions))
		for _, s := range r.Decision.ExpiredSuppressions {
			ids = append(ids, fmt.Sprintf("%s (expired %s)", s.Rule, s.Expires))
		}
		w("%s. Renew or remove them.\n\n", strings.Join(ids, ", "))
	}
}

func severityLabel(s evidence.Severity) string {
	switch s {
	case evidence.Critical:
		return "**critical**"
	case evidence.Inconclusive:
		return "*inconclusive*"
	default:
		return string(s)
	}
}

func (r *Report) mdComparisons(w writer) {
	if len(r.Comparisons) == 0 {
		return
	}
	w("## Scenario comparison\n\n")
	w("Medians across %d repeat(s). *During migration* covers only the time the migration ran.\n\n", r.Workload.Repeats)
	w("| Scenario | Mode | p95 baseline → candidate | p95 ratio | Max during migration | Error rate Δ | Throughput Δ | Baseline noise | Ops (B / C) |\n|---|---|---|---|---|---|---|---|---|\n")
	for _, c := range r.Comparisons {
		during := "–"
		if c.During != nil {
			during = fmtMS(c.During.MaxObservedMS)
		}
		w("| %s | %s | %s → %s | %s | %s | %+.2f pp | %s | %s | %s / %s |\n",
			esc(c.Scenario), c.Mode, fmtMS(c.Baseline.P95MS), fmtMS(c.Candidate.P95MS), fmtRatio(c.P95Ratio), during,
			c.ErrorRateDeltaPP, fmtPct(c.ThroughputChangePct), fmtCV(c.BaselineNoiseCV), fmtInt(int64(c.Baseline.Ops)), fmtInt(int64(c.Candidate.Ops)))
	}
	w("\n")
}

func (r *Report) mdMigration(w writer) {
	w("## Migration\n\n")
	w("Executed with `lock_timeout = %d ms`, `statement_timeout = %d ms`, transaction mode `%s`, as `%s`.\n\n",
		r.Migration.LockTimeoutMS, r.Migration.StatementTimeoutMS, r.Migration.Transaction, r.Migration.RunAs)
	if len(r.Migration.Statements) == 0 {
		w("No statements.\n\n")
		return
	}
	outcomes := map[string][]string{}
	durations := map[string][]float64{}
	for _, p := range r.Phases {
		if p.Migration == nil {
			continue
		}
		for _, s := range p.Migration.Statements {
			label := string(s.Status)
			if s.SQLState != "" {
				label += " " + s.SQLState
			}
			outcomes[s.StatementID] = append(outcomes[s.StatementID], label)
			durations[s.StatementID] = append(durations[s.StatementID], s.DurationMS)
		}
	}
	w("| ID | Location | Statement | Predicted locks | Outcome per repeat | Duration (max) |\n|---|---|---|---|---|---|\n")
	for _, s := range r.Migration.Statements {
		var locks []string
		for _, l := range s.Locks {
			locks = append(locks, fmt.Sprintf("%s on %s", l.Mode, l.Table))
		}
		if !s.Parsed {
			locks = []string{"not analysed (parse error)"}
		}
		lockText := strings.Join(locks, "; ")
		if lockText == "" {
			lockText = "–"
		}
		dur := "–"
		if ds := durations[s.ID]; len(ds) > 0 {
			sort.Float64s(ds)
			dur = fmtMS(ds[len(ds)-1])
		}
		out := "not executed"
		if o := outcomes[s.ID]; len(o) > 0 {
			out = summarizeOutcomes(o)
		}
		loc := s.File + ":" + fmt.Sprint(s.StartLine)
		if s.EndLine > s.StartLine {
			loc += fmt.Sprintf("-%d", s.EndLine)
		}
		w("| %s | %s | %s | %s | %s | %s |\n", s.ID, esc(loc), esc(s.Tag), esc(lockText), esc(out), dur)
	}
	w("\n")
}

// summarizeOutcomes collapses ["succeeded", "succeeded", "failed 55P03"]
// into "succeeded ×2, failed 55P03 ×1".
func summarizeOutcomes(xs []string) string {
	counts := map[string]int{}
	var order []string
	for _, x := range xs {
		if counts[x] == 0 {
			order = append(order, x)
		}
		counts[x]++
	}
	parts := make([]string, len(order))
	for i, o := range order {
		parts[i] = fmt.Sprintf("%s ×%d", o, counts[o])
	}
	return strings.Join(parts, ", ")
}

func (r *Report) mdLocks(w writer) {
	var rows []Phase
	for _, p := range r.Phases {
		if p.Kind == "candidate" {
			rows = append(rows, p)
		}
	}
	if len(rows) == 0 {
		return
	}
	w("## Lock telemetry (candidate phases)\n\n")
	w("Sampled every %d ms from `pg_stat_activity` and `pg_locks`; waits shorter than the interval can be missed. *Logged* waits come from the server log (`log_lock_waits`), which records every wait longer than `deadlock_timeout` exactly.\n\n", rows[0].Locks.SampleIntervalMS)
	w("| Repeat | Migration waited | Migration blocked by | Sessions queued behind migration | Longest workload lock wait | Logged waits | Longest open transaction | Samples |\n|---|---|---|---|---|---|---|---|\n")
	for _, p := range rows {
		blockedBy := strings.Join(p.Locks.MigrationBlockedBy, ", ")
		if blockedBy == "" {
			blockedBy = "–"
		}
		logged := "–"
		if p.LoggedWaits.Events > 0 {
			logged = fmt.Sprintf("%d (workload max %s)", p.LoggedWaits.Events, fmtMS(p.LoggedWaits.MaxWorkloadWaitMS))
		}
		longest := fmtMS(p.Locks.MaxXactMS)
		if p.Locks.MaxXactApp != "" {
			longest += " (" + p.Locks.MaxXactApp + ")"
		}
		w("| %d | %s | %s | %d | %s | %s | %s | %d |\n", p.Repeat+1, fmtMS(p.Locks.MigrationMaxWaitMS), esc(blockedBy),
			p.Locks.MaxBlockedByMigration, fmtMS(p.Locks.MaxLockWaitMS), logged, esc(longest), p.Locks.Samples)
	}
	w("\n")
}

// mdCompatibility shows each compatibility probe once with how many
// candidate repeats it broke in.
func (r *Report) mdCompatibility(w writer) {
	rows := r.CompatibilityRows()
	if len(rows) == 0 {
		return
	}
	w("## Application compatibility\n\n")
	w("Each probe ran before and after the migration in every candidate phase, as the application role, in a transaction that was rolled back.\n\n")
	w("| Version | Probe | Before | After | Broken in |\n|---|---|---|---|---|\n")
	for _, rw := range rows {
		w("| %s | %s | %s | %s | %d of %d |\n", esc(rw.Version), esc(rw.Probe), esc(rw.BeforeStatus()), esc(rw.AfterStatus()), rw.Broken, rw.Runs)
	}
	w("\n")
}

func (r *Report) mdTimeline(w writer) {
	var worst *Phase
	for i := range r.Phases {
		p := &r.Phases[i]
		if p.Kind != "candidate" {
			continue
		}
		if worst == nil || maxBucket(p) > maxBucket(worst) {
			worst = p
		}
	}
	if worst == nil || len(worst.Timeline) == 0 {
		return
	}
	start, end := -1.0, -1.0
	if m := worst.Migration; m != nil {
		start, end = m.OffsetMS/1000, (m.OffsetMS+m.DurationMS)/1000
	}
	w("## Timeline (candidate repeat %d)\n\n", worst.Repeat+1)
	w("Foreground operations per second; `M` marks seconds in which the migration was running.\n\n```text\n")
	w("  sec      ops  errors      p95       max\n")
	for _, bkt := range worst.Timeline {
		mark := " "
		if start >= 0 && bkt.OffsetSec+1 > start && bkt.OffsetSec <= end {
			mark = "M"
		}
		w("%s %4.0f %8d %7d %8s %9s\n", mark, bkt.OffsetSec, bkt.Ops, bkt.Errors, fmtMS(bkt.P95MS), fmtMS(bkt.MaxObservedMS))
	}
	w("```\n\n")
}

func maxBucket(p *Phase) float64 {
	m := 0.0
	for _, b := range p.Timeline {
		if b.MaxObservedMS > m {
			m = b.MaxObservedMS
		}
	}
	return m
}

func (r *Report) mdQuality(w writer) {
	q := r.Quality
	w("## Experiment quality\n\n")
	w("- Representativeness: **%s**\n", r.Representativeness.Level)
	w("- Pairs completed: %d of %d\n", q.PairsCompleted, q.RepeatsRequested)
	w("- Minimum successful operations per scenario and phase: %s (required %s)\n", fmtInt(int64(q.MinSamples)), fmtInt(int64(q.MinSamplesRequired)))
	w("- Largest baseline noise (p95 coefficient of variation): %s\n", fmtCV(q.MaxBaselineNoiseCV))
	w("- Telemetry: interval %d ms, largest gap %s, failed samples %d\n", q.SampleIntervalMS, fmtMS(q.SamplerMaxGapMS), q.SamplerFailures)
	if q.ConnectFailures > 0 {
		w("- Workload connection failures: %d\n", q.ConnectFailures)
	}
	w("\n**Limitations**\n\n")
	for _, l := range r.Representativeness.Limitations {
		w("- %s\n", l)
	}
	w("\n")
}

func (r *Report) mdReproduction(w writer) {
	w("## Reproduction\n\n")
	w("| Item | Value |\n|---|---|\n")
	w("| Tool | %s |\n", esc(r.Tool.String()))
	w("| Report schema | %s |\n", r.SchemaVersion)
	if r.GitSHA != "" {
		w("| Git commit | `%s` |\n", r.GitSHA)
	}
	w("| Image | `%s` |\n", esc(r.Engine.Image))
	if r.Engine.ServerVersion != "" {
		w("| Server | %s |\n", esc(r.Engine.ServerVersion))
	}
	if r.Engine.SettingsHash != "" {
		w("| Settings hash | `%s` |\n", r.Engine.SettingsHash)
	}
	w("| Resource limits | %.4g CPUs, %d MiB memory, %d MiB disk |\n", r.Engine.CPUs, r.Engine.MemoryMB, r.Engine.DiskMB)
	if r.Fixture.Hash != "" {
		w("| Fixture hash | `%s` |\n", r.Fixture.Hash)
	}
	if r.Workload.Hash != "" {
		w("| Workload hash | `%s` |\n", r.Workload.Hash)
	}
	w("| Workload seed | %d |\n", r.Workload.Seed)
	w("| Parser | %s |\n\n", r.Engine.ParserGrammar)
}
