// Package report defines the versioned report contract and renders it as
// JSON, Markdown and HTML. The JSON report is the primary integration
// boundary; its schema lives in schemas/report.v1.schema.json and is
// versioned independently of the CLI.
package report

import (
	"time"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/config"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/experiment"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/migrate"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/pglock"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/postgres"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/risk"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/telemetry"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/version"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/workload"
)

// SchemaVersion follows semantic versioning: additive fields bump the
// minor version, removals or meaning changes bump the major version.
const SchemaVersion = "1.0.0"

// Run statuses.
const (
	StatusCompleted             = "completed"
	StatusCompletedWithFindings = "completed_with_findings"
	StatusInconclusive          = "inconclusive"
	StatusFailed                = "failed"
)

// Report is the complete, self-describing result of a run.
type Report struct {
	SchemaVersion string       `json:"schema_version"`
	RunID         string       `json:"run_id"`
	Project       string       `json:"project"`
	Status        string       `json:"status"`
	CIDecision    string       `json:"ci_decision"`
	Tool          version.Info `json:"tool"`
	GitSHA        string       `json:"git_sha,omitempty"`
	StartedAt     time.Time    `json:"started_at"`
	FinishedAt    time.Time    `json:"finished_at"`
	DurationSec   float64      `json:"duration_sec"`

	Engine             Engine             `json:"engine"`
	Representativeness Representativeness `json:"representativeness"`
	Fixture            Fixture            `json:"fixture"`
	Workload           Workload           `json:"workload"`
	Migration          Migration          `json:"migration"`

	Comparisons []risk.Comparison `json:"comparisons"`
	Phases      []Phase           `json:"phases"`
	Quality     risk.Quality      `json:"quality"`
	Findings    []risk.Finding    `json:"findings"`
	Decision    risk.Decision     `json:"decision"`

	Environment Environment `json:"environment"`
	// EffectiveConfig is the configuration after defaults and environment
	// overrides; it contains no credentials because the configuration
	// format has none.
	EffectiveConfig *config.Config `json:"effective_config,omitempty"`
	Errors          []string       `json:"errors,omitempty"`
}

// Engine describes the disposable server.
type Engine struct {
	PostgresMajor string            `json:"postgres_major"`
	ServerVersion string            `json:"server_version,omitempty"`
	Runtime       string            `json:"runtime"`
	Image         string            `json:"image,omitempty"`
	Network       string            `json:"network"`
	MemoryMB      int               `json:"memory_mb"`
	CPUs          float64           `json:"cpus"`
	DiskMB        int               `json:"ephemeral_disk_mb"`
	SettingsHash  string            `json:"settings_hash,omitempty"`
	Settings      map[string]string `json:"settings,omitempty"`
	ParserGrammar string            `json:"parser_grammar"`
}

// Representativeness states what the fixture claims to model.
type Representativeness struct {
	Level       string   `json:"level"`
	Limitations []string `json:"limitations"`
}

// Fixture describes the seeded data.
type Fixture struct {
	Hash          string               `json:"hash"`
	Tables        []postgres.TableSize `json:"tables,omitempty"`
	GeneratedRows int64                `json:"generated_rows,omitempty"`
	SeedSeconds   float64              `json:"seed_seconds,omitempty"`
}

// Workload describes the traffic model.
type Workload struct {
	Name           string                  `json:"name,omitempty"`
	Hash           string                  `json:"hash,omitempty"`
	Mode           string                  `json:"mode"`
	Concurrency    int                     `json:"concurrency"`
	ArrivalRate    float64                 `json:"arrival_rate,omitempty"`
	WarmupSec      float64                 `json:"warmup_sec"`
	MeasurementSec float64                 `json:"measurement_sec"`
	Repeats        int                     `json:"repeats"`
	Seed           int64                   `json:"seed"`
	Scenarios      []ScenarioDescription   `json:"scenarios"`
	Background     []workload.ScenarioInfo `json:"background,omitempty"`
}

// ScenarioDescription identifies a scenario without exposing its SQL unless
// raw query text persistence is enabled.
type ScenarioDescription struct {
	ID         string   `json:"id"`
	Mode       string   `json:"mode"`
	Weight     float64  `json:"weight"`
	TemplateID string   `json:"template_id"`
	SQL        []string `json:"sql,omitempty"`
	HoldMS     int      `json:"hold_ms,omitempty"`
}

// Migration describes the migration under test.
type Migration struct {
	Files              []string    `json:"files"`
	LockTimeoutMS      int         `json:"lock_timeout_ms"`
	StatementTimeoutMS int         `json:"statement_timeout_ms"`
	Transaction        string      `json:"transaction"`
	RunAs              string      `json:"run_as"`
	Statements         []Statement `json:"statements"`
}

// Statement is one analysed migration statement.
type Statement struct {
	ID          string      `json:"id"`
	File        string      `json:"file"`
	StartLine   int         `json:"start_line"`
	EndLine     int         `json:"end_line"`
	Kind        string      `json:"kind"`
	Tag         string      `json:"tag"`
	Locks       []TableLock `json:"locks,omitempty"`
	Transaction string      `json:"transaction_requirement"`
	Parsed      bool        `json:"parsed"`
	ParseError  string      `json:"parse_error,omitempty"`
	Fingerprint string      `json:"fingerprint,omitempty"`
	SQL         string      `json:"sql,omitempty"`
}

// TableLock is a predicted lock.
type TableLock struct {
	Table  string `json:"table"`
	Mode   string `json:"mode"`
	Impact string `json:"impact"`
}

func lockOf(table string, m pglock.Mode) TableLock {
	return TableLock{Table: table, Mode: m.String(), Impact: string(m.Impact())}
}

// Phase is one baseline or candidate phase.
type Phase struct {
	Kind            string                   `json:"kind"`
	Repeat          int                      `json:"repeat"`
	MeasuredSec     float64                  `json:"measured_sec"`
	Sessions        int                      `json:"sessions"`
	Scenarios       []workload.ScenarioStats `json:"scenarios"`
	DuringMigration []workload.ScenarioStats `json:"during_migration,omitempty"`
	Timeline        []workload.Bucket        `json:"timeline"`
	Locks           telemetry.Summary        `json:"locks"`
	Counters        telemetry.Counters       `json:"counters"`
	Migration       *migrate.Outcome         `json:"migration,omitempty"`
}

func phaseOf(p experiment.PhaseResult) Phase {
	return Phase{Kind: string(p.Kind), Repeat: p.Repeat, MeasuredSec: p.MeasuredSec, Sessions: p.Sessions,
		Scenarios: p.Scenarios, DuringMigration: p.DuringMigration, Timeline: p.Timeline,
		Locks: p.Locks, Counters: p.Counters, Migration: p.Migration}
}

// Environment records where and how the run happened.
type Environment struct {
	OS           string   `json:"os"`
	Arch         string   `json:"arch"`
	HostCPUs     int      `json:"host_cpus"`
	EnvOverrides []string `json:"env_overrides,omitempty"`
	CI           string   `json:"ci,omitempty"`
}
