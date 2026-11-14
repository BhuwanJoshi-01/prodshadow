// Package workload defines and runs the concurrent workload that is applied
// to the baseline and candidate databases.
//
// A workload has weighted scenarios, executed by a pool of client sessions,
// and optional background sessions. Background sessions model the
// long-running or idle-in-transaction work that, in most real migration
// outages, holds a lock the migration has to wait for; they are what makes
// lock-queue amplification reproducible.
package workload

import (
	"bytes"
	"fmt"
	"os"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/gen"

	"go.yaml.in/yaml/v3"
)

// Spec is the root of a workload file.
type Spec struct {
	Version int    `yaml:"version" json:"version"`
	Name    string `yaml:"name" json:"name"`
	// ThinkTimeMS pauses each closed-loop worker between operations.
	ThinkTimeMS int          `yaml:"think_time_ms" json:"think_time_ms,omitempty"`
	Scenarios   []Scenario   `yaml:"scenarios" json:"scenarios"`
	Background  []Background `yaml:"background" json:"background,omitempty"`
	// Compatibility lists queries of application versions that are still
	// deployed while the migration runs. Each probe is run once before and
	// once after the migration in candidate phases.
	Compatibility []AppVersion `yaml:"compatibility" json:"compatibility,omitempty"`
}

// AppVersion is one deployed application release and the queries it runs.
type AppVersion struct {
	Version string  `yaml:"version" json:"version"`
	Probes  []Probe `yaml:"probes" json:"probes"`
}

// Probe is a single query an application version depends on. It runs in a
// transaction that is rolled back, so writes are allowed and have no
// lasting effect.
type Probe struct {
	ID         string     `yaml:"id" json:"id"`
	SQL        string     `yaml:"sql" json:"-"`
	Parameters []gen.Spec `yaml:"parameters" json:"parameters,omitempty"`
}

// Scenario is one weighted operation of the foreground mix.
type Scenario struct {
	ID     string  `yaml:"id" json:"id"`
	Weight float64 `yaml:"weight" json:"weight"`
	// Mode is "read" or "write"; it groups results in reports.
	Mode string `yaml:"mode" json:"mode"`
	// SQL is a single statement; mutually exclusive with Transaction.
	SQL         string     `yaml:"sql" json:"-"`
	Transaction *TxnSpec   `yaml:"transaction" json:"transaction,omitempty"`
	Parameters  []gen.Spec `yaml:"parameters" json:"parameters,omitempty"`
	// TimeoutMS is the session statement_timeout for this scenario.
	TimeoutMS int `yaml:"timeout_ms" json:"timeout_ms,omitempty"`
}

// TxnSpec runs statements in one transaction and optionally keeps it open.
type TxnSpec struct {
	Statements []string `yaml:"statements" json:"-"`
	// HoldMS keeps the transaction open (idle in transaction) after the
	// statements, holding their locks, before committing.
	HoldMS int `yaml:"hold_ms" json:"hold_ms,omitempty"`
}

// Background describes sessions that run alongside the weighted mix.
type Background struct {
	ID       string `yaml:"id" json:"id"`
	Sessions int    `yaml:"sessions" json:"sessions"`
	// StartAfterSeconds is relative to the start of the measurement window.
	StartAfterSeconds float64    `yaml:"start_after_seconds" json:"start_after_seconds"`
	Transaction       TxnSpec    `yaml:"transaction" json:"transaction"`
	Parameters        []gen.Spec `yaml:"parameters" json:"parameters,omitempty"`
	// Repeat starts a new transaction after each one completes.
	Repeat bool `yaml:"repeat" json:"repeat"`
}

// MaxFileSize bounds workload files.
const MaxFileSize = 1 << 20

// Load reads and validates a workload file.
func Load(path string) (*Spec, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if fi.Size() > MaxFileSize {
		return nil, fmt.Errorf("%s is larger than %d bytes", path, MaxFileSize)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Parse decodes and validates a workload document. Unknown keys are errors
// because a misspelled key in a workload silently changes the experiment.
func Parse(data []byte) (*Spec, error) {
	var s Spec
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("parse workload: %w", err)
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return &s, nil
}
