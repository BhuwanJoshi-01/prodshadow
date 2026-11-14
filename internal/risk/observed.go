package risk

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/config"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/experiment"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/migrate"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/pglock"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/sqlinspect"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/stats"
)

const docLockTimeout = "https://www.postgresql.org/docs/current/runtime-config-client.html#GUC-LOCK-TIMEOUT"

func rule(id string) ObservedRule {
	for _, r := range ObservedRules {
		if r.ID == id {
			return r
		}
	}
	return ObservedRule{ID: id, Name: strings.ToLower(id)}
}

func newFinding(id string, conf evidence.Confidence, reason, msg string) Finding {
	r := rule(id)
	kind := evidence.Observed
	if strings.HasPrefix(id, "QUAL") {
		kind = evidence.Quality
	}
	return Finding{RuleID: r.ID, Rule: r.Name, Severity: r.Severity, Confidence: conf, ConfidenceReason: reason, Evidence: kind, Message: msg}
}

func candidates(res *experiment.Result) []experiment.PhaseResult {
	var out []experiment.PhaseResult
	for _, ph := range res.Phases {
		if ph.Kind == experiment.Candidate {
			out = append(out, ph)
		}
	}
	return out
}

// consistency describes how many candidate repeats showed an effect and
// maps that to a confidence level.
func consistency(hits, total int) (evidence.Confidence, string) {
	reason := fmt.Sprintf("observed in %d of %d candidate repeats", hits, total)
	switch {
	case total >= 3 && hits == total:
		return evidence.High, reason
	case hits*2 > total:
		return evidence.Medium, reason
	default:
		return evidence.Low, reason
	}
}

func locate(f *Finding, stmts []*sqlinspect.Statement, id string) {
	for _, s := range stmts {
		if s.ID == id {
			f.StatementID, f.File, f.Line = s.ID, s.File, s.StartLine
			if t := s.Tables(); len(t) > 0 {
				f.Table = t[0]
			}
			if l := s.StrongestLock(); l != 0 {
				f.Lock = l.String()
			}
			return
		}
	}
}

// observedFindings evaluates experiment rules.
func observedFindings(cfg *config.Config, res *experiment.Result, comps []Comparison, stmts []*sqlinspect.Statement) []Finding {
	var out []Finding
	cands := candidates(res)
	if len(cands) == 0 {
		return nil
	}
	p := cfg.Policy

	// Lock-queue evidence: server-side waits and client-side stalls.
	worstWait, blockedRepeats, maxQueued := 0.0, 0, 0
	var blockers []string
	var relation, mode string
	for _, ph := range cands {
		// Logged waits are exact; sampled waits are lower bounds.
		worstWait = math.Max(worstWait, math.Max(ph.Locks.MaxLockWaitMS, ph.LoggedWaits.MaxWorkloadWaitMS))
		if ph.Locks.MaxBlockedByMigration > 0 {
			blockedRepeats++
		}
		maxQueued = max(maxQueued, ph.Locks.MaxBlockedByMigration)
		blockers = append(blockers, ph.Locks.MigrationBlockedBy...)
		// Relations are sorted by longest wait; the first is normally the
		// migration's own request.
		if relation == "" && len(ph.Locks.Relations) > 0 {
			relation, mode = ph.Locks.Relations[0].Relation, ph.Locks.Relations[0].Mode
		}
	}
	blockers = uniq(blockers)
	worstStall, stallScenario := 0.0, ""
	for _, c := range comps {
		if !c.Background && c.StallMS > worstStall {
			worstStall, stallScenario = c.StallMS, c.Scenario
		}
	}
	observed := math.Max(worstWait, worstStall)
	source := fmt.Sprintf("server-side lock wait; client-side stall %.0f ms", worstStall)
	if worstStall > worstWait {
		source = fmt.Sprintf("client-side stall of scenario %s beyond its baseline p99; longest server-side lock wait %.0f ms", stallScenario, worstWait)
	}

	if p.MaxLockWaitMS != nil && observed > *p.MaxLockWaitMS {
		hits := 0
		for _, ph := range cands {
			if math.Max(ph.Locks.MaxLockWaitMS, ph.LoggedWaits.MaxWorkloadWaitMS) > *p.MaxLockWaitMS || stallOf(ph, comps) > *p.MaxLockWaitMS {
				hits++
			}
		}
		conf, reason := consistency(hits, len(cands))
		if blockedRepeats == 0 {
			conf = conf.Lower(evidence.Medium)
			reason += "; no server-side lock wait was sampled, the stall was measured client-side"
		}
		f := newFinding("OBS001", conf, reason, fmt.Sprintf("Workload sessions waited up to %.0f ms while the migration ran (%s), exceeding the %.0f ms budget.", observed, source, *p.MaxLockWaitMS))
		f.Observed, f.Threshold, f.Unit = ptr(observed), ptr(*p.MaxLockWaitMS), "ms"
		locate(&f, stmts, responsibleStatement(cands, stmts))
		if relation != "" {
			f.Table = relation
		}
		f.Remediation = "Keep blocking DDL short: set a low lock_timeout with retries, avoid running it while long transactions are open, or use a non-blocking variant."
		out = append(out, f)
	}

	if maxQueued > 0 {
		conf, reason := consistency(blockedRepeats, len(cands))
		msg := fmt.Sprintf("Up to %d workload session(s) queued behind the migration's lock request", maxQueued)
		if relation != "" {
			msg += fmt.Sprintf(" (%s on %s)", mode, relation)
		}
		msg += fmt.Sprintf("; the longest sampled wait was %.0f ms.", worstWait)
		if len(blockers) > 0 {
			msg += " The migration itself waited for: " + strings.Join(blockers, ", ") + "."
		}
		f := newFinding("OBS011", conf, reason, msg)
		f.Observed, f.Unit = ptr(float64(maxQueued)), "sessions"
		locate(&f, stmts, responsibleStatement(cands, stmts))
		if relation != "" {
			f.Table = relation
		}
		out = append(out, f)
	}

	if limit := p.MaxQueuedSessions; limit != nil && maxQueued > *limit {
		hits := 0
		for _, ph := range cands {
			if ph.Locks.MaxBlockedByMigration > *limit {
				hits++
			}
		}
		conf, reason := consistency(hits, len(cands))
		reason += "; sampled counts are a lower bound"
		f := newFinding("OBS012", conf, reason, fmt.Sprintf("Up to %d workload session(s) queued behind the migration's lock request at once, more than the %d allowed by policy.max_queued_sessions. Each queued session holds a connection, so a pool of that size would be exhausted.", maxQueued, *limit))
		f.Observed, f.Threshold, f.Unit = ptr(float64(maxQueued)), ptr(float64(*limit)), "sessions"
		locate(&f, stmts, responsibleStatement(cands, stmts))
		if relation != "" {
			f.Table = relation
		}
		f.Remediation = "Lower lock_timeout so the migration gives up before the queue grows, retry it, and run it when long transactions are not open."
		out = append(out, f)
	}

	out = append(out, migrationFindings(cfg, cands, stmts, blockers)...)
	out = append(out, compatibilityFindings(cands)...)
	out = append(out, regressionFindings(cfg, comps, len(cands))...)

	deadlocks := int64(0)
	for _, ph := range cands {
		deadlocks += ph.Counters.Deadlocks
	}
	if deadlocks > 0 {
		f := newFinding("OBS010", evidence.High, "counted by pg_stat_database", fmt.Sprintf("%d deadlock(s) occurred in candidate phases.", deadlocks))
		f.Observed, f.Unit = ptr(float64(deadlocks)), "deadlocks"
		out = append(out, f)
	}
	return out
}

// responsibleStatement picks the migration statement most likely to have
// caused waits: the longest-running statement whose predicted lock blocks
// writes or reads, or else the longest-running statement overall.
func responsibleStatement(cands []experiment.PhaseResult, stmts []*sqlinspect.Statement) string {
	blocking := map[string]bool{}
	for _, s := range stmts {
		switch s.StrongestLock().Impact() {
		case pglock.ImpactWrites, pglock.ImpactAllTraffic:
			blocking[s.ID] = true
		}
	}
	longest := map[string]float64{}
	for _, ph := range cands {
		if ph.Migration == nil {
			continue
		}
		for _, so := range ph.Migration.Statements {
			longest[so.StatementID] = math.Max(longest[so.StatementID], so.DurationMS)
		}
	}
	best, bestBlocking, bestDur := "", false, -1.0
	for id, d := range longest {
		b := blocking[id]
		if (b && !bestBlocking) || (b == bestBlocking && (d > bestDur || (d == bestDur && id < best))) {
			best, bestBlocking, bestDur = id, b, d
		}
	}
	return best
}

func stallOf(ph experiment.PhaseResult, comps []Comparison) float64 {
	worst := 0.0
	for i, d := range ph.DuringMigration {
		if d.Background || i >= len(comps) {
			continue
		}
		worst = math.Max(worst, d.MaxObservedMS-comps[i].Baseline.P99MS)
	}
	return worst
}

func migrationFindings(cfg *config.Config, cands []experiment.PhaseResult, stmts []*sqlinspect.Statement, blockers []string) []Finding {
	var out []Finding
	byState := map[string][]*migrate.Outcome{}
	var durations []float64
	for _, ph := range cands {
		if ph.Migration == nil {
			continue
		}
		durations = append(durations, ph.Migration.DurationMS)
		if ph.Migration.Status == migrate.Failed {
			byState[ph.Migration.SQLState] = append(byState[ph.Migration.SQLState], ph.Migration)
		}
	}
	states := make([]string, 0, len(byState))
	for s := range byState {
		states = append(states, s)
	}
	sort.Strings(states)
	for _, state := range states {
		outs := byState[state]
		first := outs[0]
		conf, reason := consistency(len(outs), len(cands))
		var f Finding
		switch state {
		case "55P03":
			msg := fmt.Sprintf("The migration could not acquire its lock within lock_timeout (%d ms)", cfg.Migration.LockTimeoutMS)
			if len(blockers) > 0 {
				msg += " because " + strings.Join(blockers, ", ") + " held a conflicting lock"
			}
			msg += ". In production, without a lock_timeout, it would have waited — and every query queued behind it."
			f = newFinding("OBS002", conf, reason, msg)
			f.Remediation = "Run the statement with a short lock_timeout and retry with backoff, and avoid deploying while long transactions are open."
			f.DocsURL = docLockTimeout
		case "57014":
			f = newFinding("OBS004", conf, reason, fmt.Sprintf("A migration statement exceeded statement_timeout (%d ms).", cfg.Migration.StatementTimeoutMS))
			f.Remediation = "Split the statement, batch the data change, or use a variant that does not need to finish within one statement."
		default:
			f = newFinding("OBS003", conf, reason, fmt.Sprintf("Migration statement %s failed with SQLSTATE %s: %s", first.FailedStatement, state, first.Error))
		}
		locate(&f, stmts, first.FailedStatement)
		out = append(out, f)
	}
	if p := cfg.Policy.MaxMigrationSeconds; p != nil && len(durations) > 0 {
		med := stats.Median(durations) / 1000
		if med > *p {
			f := newFinding("OBS009", evidence.Medium, "median duration across candidate repeats; durations depend on fixture size and hardware",
				fmt.Sprintf("The migration took %.1f s (median), longer than the %.1f s budget.", med, *p))
			f.Observed, f.Threshold, f.Unit = ptr(med), ptr(*p), "s"
			out = append(out, f)
		}
	}
	return out
}

func regressionFindings(cfg *config.Config, comps []Comparison, repeats int) []Finding {
	var out []Finding
	p := cfg.Policy
	for _, c := range comps {
		if c.Background {
			continue
		}
		noise := 0.0
		if c.BaselineNoiseCV != nil {
			noise = *c.BaselineNoiseCV
		}
		check := func(id, label string, ratio, limit *float64) {
			if ratio == nil || limit == nil || *ratio <= *limit {
				return
			}
			// A regression must exceed both the policy and three times
			// the baseline's own variation to count as a finding.
			band := math.Max(*limit, 1+3*noise)
			if *ratio <= band || noise > UnstableCV {
				f := newFinding(id, evidence.Low, fmt.Sprintf("baseline noise %.0f%%; noise band %.2f×", noise*100, band),
					fmt.Sprintf("%s latency of %s rose %.2f× (limit %.2f×), but baseline repeats varied by %.0f%%, so the change cannot be distinguished from noise.", label, c.Scenario, *ratio, *limit, noise*100))
				f.Severity = evidence.Inconclusive
				f.Scenario, f.Observed, f.Threshold, f.Unit = c.Scenario, ratio, limit, "ratio"
				out = append(out, f)
				return
			}
			conf, reason := evidence.High, fmt.Sprintf("median of %d repeats; baseline noise %.0f%%", repeats, noise*100)
			if repeats < 3 || noise > 0.2 {
				conf = evidence.Medium
			}
			f := newFinding(id, conf, reason, fmt.Sprintf("%s latency of %s rose %.2f× (limit %.2f×).", label, c.Scenario, *ratio, *limit))
			f.Scenario, f.Observed, f.Threshold, f.Unit = c.Scenario, ratio, limit, "ratio"
			out = append(out, f)
		}
		check("OBS005", "p95", c.P95Ratio, p.MaxP95LatencyRatio)
		check("OBS006", "p99", c.P99Ratio, p.MaxP99LatencyRatio)

		if p.MaxErrorRateIncreasePctPoints != nil && c.ErrorRateDeltaPP > *p.MaxErrorRateIncreasePctPoints {
			f := newFinding("OBS007", evidence.High, fmt.Sprintf("%d failed of %d candidate operations", c.Candidate.Errors, c.Candidate.Ops),
				fmt.Sprintf("The error rate of %s rose by %.2f percentage points (limit %.2f).", c.Scenario, c.ErrorRateDeltaPP, *p.MaxErrorRateIncreasePctPoints))
			f.Scenario, f.Observed, f.Threshold, f.Unit = c.Scenario, ptr(c.ErrorRateDeltaPP), ptr(*p.MaxErrorRateIncreasePctPoints), "pct_points"
			out = append(out, f)
		}
		if p.MaxThroughputDropPct != nil && c.ThroughputChangePct != nil && -*c.ThroughputChangePct > *p.MaxThroughputDropPct {
			f := newFinding("OBS008", evidence.Medium, "median throughput across repeats",
				fmt.Sprintf("Throughput of %s fell by %.1f%% (limit %.1f%%).", c.Scenario, -*c.ThroughputChangePct, *p.MaxThroughputDropPct))
			f.Scenario, f.Observed, f.Threshold, f.Unit = c.Scenario, ptr(-*c.ThroughputChangePct), ptr(*p.MaxThroughputDropPct), "pct"
			out = append(out, f)
		}
	}
	return out
}

func uniq(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}

// compatibilityFindings reports application versions whose probes worked
// before the migration and failed after it (OBS013), and probes that
// already failed before it (QUAL009), which leaves their version unchecked.
func compatibilityFindings(cands []experiment.PhaseResult) []Finding {
	type probeKey struct{ version, probe string }
	broken := map[probeKey]experiment.Compatibility{}
	hits := map[probeKey]int{}
	invalid := map[probeKey]experiment.Compatibility{}
	var order []probeKey
	runs := 0
	for _, ph := range cands {
		if len(ph.Compatibility) > 0 {
			runs++
		}
		for _, c := range ph.Compatibility {
			k := probeKey{c.Version, c.Probe}
			if _, seen := hits[k]; !seen {
				order = append(order, k)
				hits[k] = 0
			}
			switch {
			case !c.BeforeOK:
				invalid[k] = c
			case c.Broken():
				hits[k]++
				broken[k] = c
			}
		}
	}
	var out []Finding
	byVersion := map[string][]probeKey{}
	var versions []string
	for _, k := range order {
		if _, ok := broken[k]; ok {
			if len(byVersion[k.version]) == 0 {
				versions = append(versions, k.version)
			}
			byVersion[k.version] = append(byVersion[k.version], k)
		}
	}
	for _, v := range versions {
		keys := byVersion[v]
		var parts []string
		minHits := runs
		for _, k := range keys {
			c := broken[k]
			parts = append(parts, fmt.Sprintf("%s (%s)", k.probe, probeFailure(c.AfterSQLState, c.AfterError)))
			minHits = min(minHits, hits[k])
		}
		conf, reason := consistency(minHits, runs)
		f := newFinding("OBS013", conf, reason, fmt.Sprintf("Application version %s stops working once the migration has run: %s worked before it and failed after it. Instances of that version that are still deployed will fail these queries.", v, strings.Join(parts, "; ")))
		f.Observed, f.Unit = ptr(float64(len(keys))), "probes"
		f.Remediation = "Make the change backward compatible (expand, migrate the code, then contract in a later release), or deploy the migration only after no instance of this version is running."
		out = append(out, f)
	}
	for _, k := range order {
		c, ok := invalid[k]
		if !ok {
			continue
		}
		f := newFinding("QUAL009", evidence.High, "the probe failed on the unchanged schema",
			fmt.Sprintf("Compatibility probe %s of version %s failed before the migration ran (%s), so the probe does not match the fixture and that version's compatibility was not checked.", k.probe, k.version, probeFailure(c.BeforeSQLState, c.BeforeError)))
		out = append(out, f)
	}
	return out
}

func probeFailure(state, msg string) string {
	if state == "" {
		return msg
	}
	return state + ": " + msg
}
