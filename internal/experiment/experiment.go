package experiment

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/config"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/fixture"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/isolation"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/migrate"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/postgres"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/security"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/sqlinspect"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/telemetry"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/workload"
)

// TemplateDatabase holds the seeded fixture that every phase copies.
const TemplateDatabase = "ps_template"

// Plan is everything needed to run an experiment.
type Plan struct {
	Config    *config.Config
	RunID     string
	Provider  isolation.Provider
	Redactor  *security.Redactor
	Migration []*sqlinspect.Statement
	Workload  *workload.Spec
	Schema    *sqlinspect.Source
	Seed      *sqlinspect.Source
	Generator *fixture.GeneratorFile
	// Keep leaves the server running after the run for debugging.
	Keep bool
	// Progress receives human-readable progress lines.
	Progress func(format string, args ...any)
}

// PhaseResult is the outcome of one baseline or candidate phase.
type PhaseResult struct {
	Step
	StartedAt       time.Time                `json:"started_at"`
	MeasuredSec     float64                  `json:"measured_sec"`
	Workload        *workload.Result         `json:"-"`
	Scenarios       []workload.ScenarioStats `json:"scenarios"`
	PreMigration    []workload.ScenarioStats `json:"pre_migration,omitempty"`
	DuringMigration []workload.ScenarioStats `json:"during_migration,omitempty"`
	Timeline        []workload.Bucket        `json:"timeline"`
	Locks           telemetry.Summary        `json:"locks"`
	LoggedWaits     telemetry.LogSummary     `json:"logged_waits"`
	Counters        telemetry.Counters       `json:"counters"`
	Migration       *migrate.Outcome         `json:"migration,omitempty"`
	Sessions        int                      `json:"sessions"`
	ConnectFailures int                      `json:"connect_failures"`
	Mode            string                   `json:"mode"`
}

// Result is the outcome of the whole experiment.
type Result struct {
	RunID        string               `json:"run_id"`
	Started      time.Time            `json:"started"`
	Finished     time.Time            `json:"finished"`
	Server       postgres.ServerInfo  `json:"server"`
	Tables       []postgres.TableSize `json:"tables"`
	FixtureRows  []fixture.LoadResult `json:"fixture_rows,omitempty"`
	SeedSeconds  float64              `json:"seed_seconds"`
	Phases       []PhaseResult        `json:"phases"`
	CleanupError string               `json:"cleanup_error,omitempty"`
	KeptServer   string               `json:"kept_server,omitempty"`
}

// ErrNoWorkload is returned when the plan has no workload; the caller
// reports the run as inconclusive.
var ErrNoWorkload = errors.New("no workload configured")

// Run executes the experiment. The disposable server is always destroyed
// unless plan.Keep is set; a cleanup failure is returned as an error
// together with the (otherwise complete) result.
func Run(ctx context.Context, plan Plan) (res *Result, err error) {
	if plan.Workload == nil {
		return nil, ErrNoWorkload
	}
	if plan.Progress == nil {
		plan.Progress = func(string, ...any) {}
	}
	cfg := plan.Config
	res = &Result{RunID: plan.RunID, Started: time.Now()}

	plan.Progress("starting PostgreSQL %s (%s, provider %s, network %s)", cfg.Postgres.Version, cfg.Postgres.Image, plan.Provider.Name(), cfg.Postgres.Isolation.Network)
	srv, err := plan.Provider.Start(ctx, isolation.Spec{
		RunID:          plan.RunID,
		PostgresMajor:  cfg.Postgres.Version,
		Image:          cfg.Postgres.Image,
		Network:        cfg.Postgres.Isolation.Network,
		MemoryMB:       cfg.Postgres.Isolation.MemoryMB,
		CPUs:           cfg.Postgres.Isolation.CPUs,
		DiskMB:         cfg.Postgres.Isolation.EphemeralDiskMB,
		PidsLimit:      cfg.Postgres.Isolation.PidsLimit,
		StartupTimeout: time.Duration(cfg.Postgres.Isolation.StartupTimeoutSeconds) * time.Second,
		Settings:       cfg.Postgres.Settings,
		Binary:         cfg.Postgres.Isolation.Binary,
		Redactor:       plan.Redactor,
	})
	if err != nil {
		return nil, fmt.Errorf("start disposable server: %w", err)
	}
	defer func() {
		if plan.Keep {
			res.KeptServer = srv.ID()
			plan.Progress("keeping server %s at %s for debugging", srv.ID(), srv.Address())
			return
		}
		dctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if derr := plan.Provider.Destroy(dctx, srv); derr != nil {
			res.CleanupError = plan.Redactor.String(derr.Error())
			err = errors.Join(err, fmt.Errorf("cleanup failed: %w", derr))
		}
	}()

	if err := srv.Bootstrap(ctx, plan.Redactor); err != nil {
		return res, err
	}
	seedStart := time.Now()
	if err := seedTemplate(ctx, plan, srv, res); err != nil {
		return res, fmt.Errorf("seed fixture: %w", err)
	}
	res.SeedSeconds = time.Since(seedStart).Seconds()
	plan.Progress("fixture seeded in %.1fs", res.SeedSeconds)

	for _, step := range Schedule(cfg.Workload.Repeats) {
		plan.Progress("repeat %d/%d: %s phase", step.Repeat+1, cfg.Workload.Repeats, step.Kind)
		pr, err := runPhase(ctx, plan, srv, step)
		if err != nil {
			return res, fmt.Errorf("%s phase (repeat %d): %w", step.Kind, step.Repeat+1, err)
		}
		res.Phases = append(res.Phases, *pr)
	}
	res.Finished = time.Now()
	return res, nil
}
