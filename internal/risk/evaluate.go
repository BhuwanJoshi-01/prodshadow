package risk

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/config"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/experiment"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/pglock"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/sqlinspect"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/workload"
)

// Exit codes of the verify command.
const (
	ExitPass         = 0
	ExitInternal     = 1
	ExitBlock        = 2
	ExitInvalid      = 3
	ExitInfra        = 4
	ExitInconclusive = 5
)

// Input is the evidence to evaluate.
type Input struct {
	Config       *config.Config
	Result       *experiment.Result
	Statements   []*sqlinspect.Statement
	Static       []sqlinspect.Finding
	Suppressions []Suppression
	// Workload is used to check whether the experiment models the long
	// transactions that most lock-queue incidents need.
	Workload *workload.Spec
	// NoExperiment explains why no experiment ran (static-only runs).
	NoExperiment string
	Now          time.Time
}

// Decision is the CI outcome.
type Decision struct {
	Outcome      string   `json:"outcome"`
	ExitCode     int      `json:"exit_code"`
	Message      string   `json:"message"`
	FailedRules  []string `json:"failed_rules,omitempty"`
	Inconclusive []string `json:"inconclusive_rules,omitempty"`
	Suppressed   []string `json:"suppressed_rules,omitempty"`
	// ExpiredSuppressions are listed so that they get renewed or removed.
	ExpiredSuppressions []Suppression `json:"expired_suppressions,omitempty"`
}

// Assessment is the complete evaluation.
type Assessment struct {
	Comparisons []Comparison `json:"comparisons"`
	Quality     Quality      `json:"quality"`
	Findings    []Finding    `json:"findings"`
	Decision    Decision     `json:"decision"`
}

// Evaluate produces findings and a decision from static and observed
// evidence.
func Evaluate(in Input) Assessment {
	cfg := in.Config
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	var a Assessment
	if in.Result != nil {
		a.Comparisons = Compare(in.Result)
	}
	a.Quality = assessQuality(in.Result, a.Comparisons, cfg.Fixture.Representation,
		cfg.Workload.MinSamplesPerScenario, cfg.Workload.Repeats, cfg.Telemetry.SampleIntervalMS)

	findings := FromStatic(in.Static)
	if in.Result != nil {
		findings = append(findings, observedFindings(cfg, in.Result, a.Comparisons, in.Statements)...)
	}
	findings = append(findings, qualityFindings(cfg, a.Quality, in.NoExperiment)...)
	if in.Result != nil {
		findings = append(findings, failingScenarios(in.Result)...)
	}
	if in.NoExperiment == "" && in.Workload != nil && !modelsBlocker(cfg, in.Workload) {
		findings = append(findings, newFinding("QUAL008", evidence.High, "derived from the workload definition",
			"No session keeps a transaction open while the migration starts. Most migration outages need one: a long transaction makes the migration's lock request wait, and every query queues behind it. Add a background session with hold_ms to rehearse that case."))
	}
	if in.NoExperiment == "" && in.Workload != nil {
		findings = append(findings, writeBlockingUnexercised(in.Workload, in.Statements)...)
		findings = append(findings, tablesNotExercised(in.Workload, in.Statements)...)
		findings = append(findings, compatibilityNotChecked(in.Workload, in.Static)...)
	}

	findings = applyOverrides(findings, cfg.Policy.Rules)
	applyInlineAnnotations(findings, in.Statements, now)
	expired := applySuppressions(findings, in.Suppressions, now)
	sortFindings(findings)
	a.Findings = findings
	a.Decision = decide(cfg.Policy, findings)
	a.Decision.ExpiredSuppressions = expired
	return a
}

func qualityFindings(cfg *config.Config, q Quality, noExperiment string) []Finding {
	var out []Finding
	if noExperiment != "" {
		out = append(out, newFinding("QUAL007", evidence.High, "no observed evidence was collected",
			"Only static analysis was performed: "+noExperiment+". Configure a fixture and workload to measure the migration under load."))
		return out
	}
	if len(q.UndersampledScenarios) > 0 {
		f := newFinding("QUAL001", evidence.High, fmt.Sprintf("minimum %d successful operations per scenario and phase", q.MinSamplesRequired),
			fmt.Sprintf("Scenario(s) %s produced fewer than %d successful operations in at least one phase (minimum observed: %d); their latency comparison is not meaningful.",
				strings.Join(q.UndersampledScenarios, ", "), q.MinSamplesRequired, q.MinSamples))
		f.Observed, f.Threshold, f.Unit = ptr(float64(q.MinSamples)), ptr(float64(q.MinSamplesRequired)), "operations"
		out = append(out, f)
	}
	if len(q.NoisyScenarios) > 0 {
		f := newFinding("QUAL002", evidence.High, fmt.Sprintf("baseline p95 coefficient of variation above %.0f%%", UnstableCV*100),
			fmt.Sprintf("Baseline latency of %s varied too much between repeats to judge a regression. Increase measurement_seconds or repeats, or run on a quieter host.", strings.Join(q.NoisyScenarios, ", ")))
		f.Observed, f.Threshold, f.Unit = q.MaxBaselineNoiseCV, ptr(UnstableCV), "cv"
		out = append(out, f)
	}
	rep := newFinding("QUAL003", evidence.High, "declared in fixture.representation",
		fmt.Sprintf("Fixture representativeness: %s (%d rows). Lock behaviour depends on lock modes and concurrency and is reproduced; durations depend on data size and hardware and are not predictive of production.", q.Representation, q.FixtureRows))
	out = append(out, rep)
	if cfg.Workload.Repeats < 3 {
		out = append(out, newFinding("QUAL004", evidence.High, "configured repeats",
			fmt.Sprintf("Only %d repeat(s) were run; at least 3 are needed to estimate baseline noise.", cfg.Workload.Repeats)))
	}
	if q.SampleIntervalMS > 0 && (q.SamplerMaxGapMS > float64(5*q.SampleIntervalMS) || q.SamplerFailures > 0) {
		f := newFinding("QUAL005", evidence.Medium, "gap between telemetry samples",
			fmt.Sprintf("Telemetry samples were up to %.0f ms apart (interval %d ms, %d failed samples); the host may be overloaded, which also distorts latency.", q.SamplerMaxGapMS, q.SampleIntervalMS, q.SamplerFailures))
		f.Observed, f.Threshold, f.Unit = ptr(q.SamplerMaxGapMS), ptr(float64(5*q.SampleIntervalMS)), "ms"
		out = append(out, f)
	}
	if q.ConnectFailures > 0 {
		out = append(out, newFinding("QUAL006", evidence.High, "connection attempts that failed",
			fmt.Sprintf("%d workload connection attempt(s) failed; check max_connections and container memory.", q.ConnectFailures)))
	}
	return out
}

// modelsBlocker reports whether any session can hold a transaction open
// when the migration starts: a background session active at that moment, or
// a foreground scenario that holds its transaction.
func modelsBlocker(cfg *config.Config, wl *workload.Spec) bool {
	for _, sc := range wl.Scenarios {
		if sc.Transaction != nil && sc.Transaction.HoldMS > 0 {
			return true
		}
	}
	start := cfg.Migration.StartAfterSeconds
	for _, b := range wl.Background {
		begins := b.StartAfterSeconds
		ends := begins + float64(b.Transaction.HoldMS)/1000
		if begins <= start && (b.Repeat || ends > start) {
			return true
		}
	}
	return false
}

// applyInlineAnnotations lets a `prodshadow:ignore` comment above a
// statement suppress observed findings attributed to that statement, just
// as it does for static findings.
func applyInlineAnnotations(fs []Finding, stmts []*sqlinspect.Statement, now time.Time) {
	byID := map[string]*sqlinspect.Statement{}
	for _, s := range stmts {
		byID[s.ID] = s
	}
	for i := range fs {
		f := &fs[i]
		if f.Suppressed || f.StatementID == "" || f.Evidence == evidence.Static || f.Evidence == evidence.Heuristic {
			continue
		}
		if s := byID[f.StatementID]; s != nil {
			if a, ok := s.Suppresses(f.RuleID, now); ok {
				f.Suppressed, f.SuppressionReason, f.SuppressionExpiry = true, a.Reason, a.Expires
			}
		}
	}
}

// applyOverrides applies policy.rules severity settings; "off" removes the
// finding entirely.
func applyOverrides(fs []Finding, overrides map[string]string) []Finding {
	if len(overrides) == 0 {
		return fs
	}
	out := fs[:0]
	for _, f := range fs {
		setting, ok := overrides[f.RuleID]
		if ok && setting == "off" {
			continue
		}
		if sev, err := evidence.ParseSeverity(setting); ok && err == nil {
			f.Severity = sev
		}
		out = append(out, f)
	}
	return out
}

func sortFindings(fs []Finding) {
	sort.SliceStable(fs, func(i, j int) bool {
		if fs[i].Suppressed != fs[j].Suppressed {
			return !fs[i].Suppressed
		}
		if fs[i].Severity.Rank() != fs[j].Severity.Rank() {
			return fs[i].Severity.Rank() > fs[j].Severity.Rank()
		}
		return fs[i].RuleID < fs[j].RuleID
	})
}

func decide(p config.PolicyConfig, fs []Finding) Decision {
	var d Decision
	for _, f := range fs {
		if f.Suppressed {
			d.Suppressed = appendUnique(d.Suppressed, f.RuleID)
			continue
		}
		if f.Severity == evidence.Inconclusive {
			d.Inconclusive = appendUnique(d.Inconclusive, f.RuleID)
			continue
		}
		if slices.Contains(p.FailOn, string(f.Severity)) {
			d.FailedRules = appendUnique(d.FailedRules, f.RuleID)
		}
	}
	switch {
	case len(d.FailedRules) > 0:
		d.Outcome, d.ExitCode = "block", ExitBlock
		d.Message = fmt.Sprintf("Blocking findings: %s.", strings.Join(d.FailedRules, ", "))
	case len(d.Inconclusive) > 0:
		d.Outcome = "inconclusive"
		d.Message = fmt.Sprintf("The experiment could not reach a conclusion (%s). Inconclusive is not a pass.", strings.Join(d.Inconclusive, ", "))
		d.ExitCode = ExitInconclusive
		if p.Inconclusive == "pass" {
			d.ExitCode = ExitPass
			d.Message += " policy.inconclusive is \"pass\", so CI will not fail."
		}
	default:
		d.Outcome, d.ExitCode = "pass", ExitPass
		d.Message = "No configured threshold was exceeded in this experiment. This is not a guarantee that the migration is safe in production."
	}
	return d
}

func appendUnique(xs []string, x string) []string {
	if slices.Contains(xs, x) {
		return xs
	}
	return append(xs, x)
}

// writeBlockingUnexercised reports migrations whose strongest lock blocks
// writes but not reads, run against a workload without write scenarios:
// the experiment cannot show their effect (QUAL010).
func writeBlockingUnexercised(wl *workload.Spec, stmts []*sqlinspect.Statement) []Finding {
	for _, sc := range wl.Scenarios {
		if sc.Mode == "write" {
			return nil
		}
	}
	for _, s := range stmts {
		for _, l := range s.Locks {
			if l.Mode.Impact() != pglock.ImpactWrites {
				continue
			}
			f := newFinding("QUAL010", evidence.High, "derived from the migration's locks and the workload's scenario modes",
				fmt.Sprintf("Statement %s takes %s on %s, which blocks writes but not reads, and the workload has no write scenario, so the experiment cannot show the stall writers would see. Add a write scenario on %s.", s.ID, l.Mode, l.Table, l.Table))
			f.StatementID, f.File, f.Line, f.Table, f.Lock = s.ID, s.File, s.StartLine, l.Table, l.Mode.String()
			return []Finding{f}
		}
	}
	return nil
}

// tablesNotExercised reports tables on which the migration takes a lock
// that blocks reads or writes, but which no scenario or background session
// touches: the experiment says nothing about them (QUAL011).
func tablesNotExercised(wl *workload.Spec, stmts []*sqlinspect.Statement) []Finding {
	touched := map[string]bool{}
	var sqls []string
	for _, sc := range wl.Scenarios {
		sqls = append(sqls, sc.SQL)
		if sc.Transaction != nil {
			sqls = append(sqls, sc.Transaction.Statements...)
		}
	}
	for _, b := range wl.Background {
		sqls = append(sqls, b.Transaction.Statements...)
	}
	for _, q := range sqls {
		if q == "" {
			continue
		}
		st, err := sqlinspect.ParseOne(q)
		if err != nil {
			continue
		}
		for _, t := range st.Tables() {
			touched[t] = true
			touched[strings.TrimPrefix(t, "public.")] = true
		}
	}
	created := map[string]bool{}
	var missing []string
	var first *sqlinspect.Statement
	for _, s := range stmts {
		if ct := s.Node().GetCreateStmt(); ct != nil && ct.Relation != nil {
			created[ct.Relation.Relname] = true
		}
		for _, l := range s.Locks {
			impact := l.Mode.Impact()
			name := strings.TrimPrefix(l.Table, "public.")
			if (impact != pglock.ImpactWrites && impact != pglock.ImpactAllTraffic) || touched[name] || created[name] || slices.Contains(missing, name) {
				continue
			}
			if s.Kind == sqlinspect.KindDropIndex {
				continue // the lock is recorded against the index name
			}
			missing = append(missing, name)
			if first == nil {
				first = s
			}
		}
	}
	if len(missing) == 0 {
		return nil
	}
	f := newFinding("QUAL011", evidence.High, "derived from the migration's locks and the tables the workload's SQL reads or writes",
		fmt.Sprintf("The migration blocks traffic on %s, but no workload scenario or background session uses %s, so the experiment cannot show what waiting on %s would do. Add scenarios for the queries your application runs on %s.", strings.Join(missing, ", "), pronoun(len(missing)), pronoun(len(missing)), pronoun(len(missing))))
	f.StatementID, f.File, f.Line = first.ID, first.File, first.StartLine
	return []Finding{f}
}

func pronoun(n int) string {
	if n == 1 {
		return "it"
	}
	return "them"
}

// compatibilityNotChecked suggests compatibility probes when static rules
// found a drop or rename that breaks running code, and the workload checks
// no application version (QUAL012).
func compatibilityNotChecked(wl *workload.Spec, static []sqlinspect.Finding) []Finding {
	if len(wl.Compatibility) > 0 {
		return nil
	}
	for _, sf := range static {
		if sf.Suppressed || (sf.RuleID != "PS109" && sf.RuleID != "PS110") {
			continue
		}
		f := newFinding("QUAL012", evidence.High, "derived from "+sf.RuleID+" and the workload definition",
			fmt.Sprintf("Statement %s drops or renames something that running application code may use (%s), but the workload declares no compatibility probes, so the experiment does not check whether the previous release keeps working. Add the previous release's queries under compatibility.", sf.StatementID, sf.RuleID))
		f.StatementID, f.File, f.Line, f.Table = sf.StatementID, sf.File, sf.Line, sf.Table
		return []Finding{f}
	}
	return nil
}

// FailingScenarioRate is the baseline error rate from which a scenario is
// considered broken rather than affected by the migration.
const FailingScenarioRate = 0.9

// failingScenarios reports scenarios that fail almost every operation in
// the baseline, before any migration ran: the workload itself is broken
// (QUAL013). The first error message says why.
func failingScenarios(res *experiment.Result) []Finding {
	type agg struct {
		ops, errors int
		sample      string
	}
	byID := map[string]*agg{}
	var order []string
	for _, ph := range res.Phases {
		if ph.Kind != experiment.Baseline {
			continue
		}
		for _, sc := range ph.Scenarios {
			a, ok := byID[sc.ID]
			if !ok {
				a = &agg{}
				byID[sc.ID] = a
				order = append(order, sc.ID)
			}
			a.ops += sc.Ops
			a.errors += sc.Errors
			if a.sample == "" {
				a.sample = sc.SampleError
			}
		}
	}
	var out []Finding
	for _, id := range order {
		a := byID[id]
		if a.ops < 10 || float64(a.errors) < FailingScenarioRate*float64(a.ops) {
			continue
		}
		msg := fmt.Sprintf("Scenario %s failed %d of %d operations in the baseline, before any migration ran, so its query is broken and the experiment measured nothing for it.", id, a.errors, a.ops)
		if a.sample != "" {
			msg += " First error: " + a.sample + "."
		}
		f := newFinding("QUAL013", evidence.High, "baseline error rate", msg)
		f.Scenario = id
		f.Observed, f.Threshold, f.Unit = ptr(float64(a.errors)/float64(a.ops)), ptr(FailingScenarioRate), "error rate"
		f.Remediation = "Run the scenario's SQL against the fixture by hand: check table and column names, parameter types (add explicit casts such as $1::bigint) and the privileges of the application role."
		out = append(out, f)
	}
	return out
}
