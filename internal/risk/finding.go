package risk

import (
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/sqlinspect"
)

// Finding is a single, explainable result. Static findings from the SQL
// analyser and observed findings from the experiment share this shape.
type Finding struct {
	RuleID     string              `json:"rule_id"`
	Rule       string              `json:"rule"`
	Severity   evidence.Severity   `json:"severity"`
	Confidence evidence.Confidence `json:"confidence"`
	// ConfidenceReason records why the confidence level was chosen.
	ConfidenceReason string        `json:"confidence_reason,omitempty"`
	Evidence         evidence.Kind `json:"evidence_kind"`
	Message          string        `json:"message"`

	Scenario  string   `json:"scenario,omitempty"`
	Observed  *float64 `json:"observed,omitempty"`
	Threshold *float64 `json:"threshold,omitempty"`
	Unit      string   `json:"unit,omitempty"`

	StatementID string `json:"statement_id,omitempty"`
	File        string `json:"file,omitempty"`
	Line        int    `json:"line,omitempty"`
	Table       string `json:"table,omitempty"`
	Lock        string `json:"lock,omitempty"`

	Remediation string `json:"remediation,omitempty"`
	DocsURL     string `json:"docs_url,omitempty"`

	Suppressed        bool   `json:"suppressed,omitempty"`
	SuppressionReason string `json:"suppression_reason,omitempty"`
	SuppressionExpiry string `json:"suppression_expires,omitempty"`
}

// FromStatic converts static analyser findings.
func FromStatic(fs []sqlinspect.Finding) []Finding {
	out := make([]Finding, 0, len(fs))
	for _, f := range fs {
		lock := ""
		if f.Lock.Valid() && f.Lock != 0 {
			lock = f.Lock.String()
		}
		out = append(out, Finding{
			RuleID: f.RuleID, Rule: f.Rule, Severity: f.Severity, Confidence: f.Confidence,
			Evidence: f.Evidence, Message: f.Message, StatementID: f.StatementID, File: f.File,
			Line: f.Line, Table: f.Table, Lock: lock, Remediation: f.Remediation, DocsURL: f.DocsURL,
			Suppressed: f.Suppressed, SuppressionReason: f.SuppressionReason, SuppressionExpiry: f.SuppressionExpiry,
		})
	}
	return out
}

// ObservedRule documents a rule evaluated against experiment evidence.
type ObservedRule struct {
	ID       string
	Name     string
	Summary  string
	Severity evidence.Severity
}

// ObservedRules lists the experiment rules, for documentation and
// `prodshadow rules` output.
var ObservedRules = []ObservedRule{
	{"OBS001", "lock-wait-budget", "Workload sessions waited longer than policy.max_lock_wait_ms while the migration ran", evidence.Critical},
	{"OBS002", "migration-lock-timeout", "The migration could not acquire a lock within lock_timeout", evidence.Critical},
	{"OBS003", "migration-failed", "A migration statement failed in the disposable database", evidence.Critical},
	{"OBS004", "migration-statement-timeout", "A migration statement exceeded statement_timeout", evidence.Critical},
	{"OBS005", "p95-latency-regression", "A scenario's p95 latency rose beyond policy.max_p95_latency_ratio", evidence.Critical},
	{"OBS006", "p99-latency-regression", "A scenario's p99 latency rose beyond policy.max_p99_latency_ratio", evidence.Critical},
	{"OBS007", "error-rate-increase", "A scenario's error rate rose beyond policy.max_error_rate_increase_pct_points", evidence.Critical},
	{"OBS008", "throughput-drop", "A scenario's throughput fell beyond policy.max_throughput_drop_pct", evidence.Critical},
	{"OBS009", "migration-duration", "The migration ran longer than policy.max_migration_seconds", evidence.Critical},
	{"OBS010", "deadlocks", "Deadlocks occurred while the migration ran", evidence.Warning},
	{"OBS011", "sessions-queued-behind-migration", "Workload sessions queued behind the migration's lock request", evidence.Info},
	{"OBS012", "queued-session-budget", "More workload sessions queued behind the migration than policy.max_queued_sessions", evidence.Critical},
	{"QUAL001", "insufficient-samples", "Too few successful operations to compare latency", evidence.Inconclusive},
	{"QUAL002", "unstable-baseline", "Baseline latency varied too much between repeats to judge a regression", evidence.Inconclusive},
	{"QUAL003", "representativeness", "The fixture's relation to production data", evidence.Info},
	{"QUAL004", "few-repeats", "Fewer than three repeats; noise cannot be estimated", evidence.Info},
	{"QUAL005", "sampler-starved", "Telemetry sampling fell behind; the test host may be overloaded", evidence.Warning},
	{"QUAL006", "connect-failures", "Workload sessions failed to connect", evidence.Warning},
	{"QUAL007", "no-experiment", "No workload or fixture; only static analysis was performed", evidence.Inconclusive},
	{"QUAL008", "no-blocker-modelled", "No session holds a transaction open while the migration runs, so lock queues behind long transactions are not exercised", evidence.Info},
}
