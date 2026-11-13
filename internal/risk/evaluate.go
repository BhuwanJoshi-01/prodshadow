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
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/sqlinspect"
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
