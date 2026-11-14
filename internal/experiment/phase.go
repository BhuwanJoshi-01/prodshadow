package experiment

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/isolation"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/migrate"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/postgres"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/sqlinspect"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/telemetry"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/workload"

	"github.com/jackc/pgx/v5"
)

// statsFlushDelay lets backends publish their statistics before counters
// are read after a phase.
const statsFlushDelay = 1100 * time.Millisecond

// runPhase creates a fresh copy of the template, runs the workload (and for
// candidates the migration), and drops the copy.
func runPhase(ctx context.Context, plan Plan, srv *isolation.Server, step Step) (*PhaseResult, error) {
	cfg := plan.Config
	db := cfg.Postgres.Database
	app, _ := srv.App()

	admin, err := srv.Connect(ctx, srv.Admin(), isolation.ConnOptions{ApplicationName: "prodshadow/admin"})
	if err != nil {
		return nil, err
	}
	defer isolation.Close(admin)
	if err := postgres.DropDatabase(ctx, admin, db); err != nil {
		return nil, err
	}
	if err := postgres.CreateDatabase(ctx, admin, db, app.User, TemplateDatabase); err != nil {
		return nil, err
	}
	defer func() {
		dctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = postgres.DropDatabase(dctx, admin, db)
	}()

	monitor, err := srv.Connect(ctx, srv.Admin(), isolation.ConnOptions{Database: db, ApplicationName: "prodshadow/telemetry"})
	if err != nil {
		return nil, err
	}
	defer isolation.Close(monitor)
	if err := postgres.Checkpoint(ctx, monitor); err != nil {
		return nil, fmt.Errorf("checkpoint: %w", err)
	}
	before, err := telemetry.Snapshot(ctx, monitor)
	if err != nil {
		return nil, err
	}

	var probesBefore []workload.ProbeResult
	if step.Kind == Candidate && hasProbes(plan) {
		if probesBefore, err = runProbes(ctx, plan, srv, db, step); err != nil {
			return nil, err
		}
	}

	pr := &PhaseResult{Step: step, StartedAt: time.Now()}
	interval := time.Duration(cfg.Telemetry.SampleIntervalMS) * time.Millisecond
	sampler := telemetry.NewSampler(monitor, interval)

	var (
		wg           sync.WaitGroup
		samples      []telemetry.Sample
		failed       int
		migrationOut *migrate.Outcome
		migrationErr error
		measureStart time.Time
	)
	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()
	// The sampler starts with the measurement window and stops as soon as
	// the workload returns.
	samplerCtx, samplerStop := context.WithCancel(runCtx)
	defer samplerStop()
	var extend chan struct{}
	if step.Kind == Candidate {
		extend = make(chan struct{})
	}

	opts := workload.Options{
		Concurrency: cfg.Workload.Concurrency,
		ArrivalRate: cfg.Workload.ArrivalRate,
		Warmup:      time.Duration(cfg.Workload.WarmupSeconds * float64(time.Second)),
		Measurement: time.Duration(cfg.Workload.MeasurementSeconds * float64(time.Second)),
		Seed:        cfg.Workload.Seed,
		// Both phases of a repeat share a stream, so they issue the same
		// parameter sequences.
		Stream: fmt.Sprintf("repeat-%d", step.Repeat),
		Connect: func(ctx context.Context, appName string) (*pgx.Conn, error) {
			return srv.Connect(ctx, app, isolation.ConnOptions{Database: db, ApplicationName: appName})
		},
		MeasureStarted: func(at time.Time) {
			measureStart = at
			wg.Add(1)
			go func() {
				defer wg.Done()
				samples, failed = sampler.Run(samplerCtx)
			}()
			if step.Kind != Candidate {
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer close(extend)
				migrationOut, migrationErr = runMigration(runCtx, plan, srv, db, at)
			}()
		},
	}
	if step.Kind == Candidate {
		opts.Extend = extend
		opts.Tail = 2 * time.Second
		perStmt := time.Duration(cfg.Migration.StatementTimeoutMS) * time.Millisecond
		opts.MaxExtension = perStmt*time.Duration(max(1, len(plan.Migration))) + 10*time.Second
	}

	res, err := workload.Run(runCtx, plan.Workload, opts)
	samplerStop()
	if err != nil {
		cancelRun()
		wg.Wait()
		return nil, err
	}
	wg.Wait()
	if migrationErr != nil {
		return nil, fmt.Errorf("migration session: %w", migrationErr)
	}
	if probesBefore != nil {
		after, err := runProbes(ctx, plan, srv, db, step)
		if err != nil {
			return nil, err
		}
		pr.Compatibility = pairProbes(plan, probesBefore, after)
	}

	time.Sleep(statsFlushDelay)
	after, err := telemetry.Snapshot(ctx, monitor)
	if err != nil {
		return nil, err
	}
	if pr.Counters, err = telemetry.Delta(ctx, monitor, before, after); err != nil {
		return nil, err
	}

	pr.Workload = res
	pr.Mode, pr.Sessions, pr.ConnectFailures = res.Mode, res.Sessions, res.ConnectFailures
	pr.MeasuredSec = res.MeasuredDuration().Seconds()
	pr.Scenarios = workload.Summarize(res, workload.Window{})
	pr.Timeline = workload.Timeline(res, time.Second)
	pr.Locks = telemetry.Summarize(samples, failed, interval, measureStart, time.Second)
	// The server logs every lock wait longer than deadlock_timeout; this
	// corroborates the sampler without its gaps.
	if logs, err := plan.Provider.LogsSince(ctx, srv, measureStart.Add(-time.Second)); err == nil {
		waits := telemetry.ParseLockWaits(logs, measureStart, time.Now())
		pr.LoggedWaits = telemetry.SummarizeLogWaits(waits, telemetry.ApplicationsByPID(samples))
	}
	if migrationOut != nil {
		redactOutcome(plan, migrationOut)
		pr.Migration = migrationOut
		if pre, during, ok := migrationWindows(migrationOut, res.MeasuredDuration()); ok {
			if pre.To > 0 {
				pr.PreMigration = workload.Summarize(res, pre)
			}
			pr.DuringMigration = workload.Summarize(res, during)
		}
	}
	return pr, nil
}

// runMigration waits for migration.start_after_seconds into the
// measurement window, then executes the migration in its own session.
func runMigration(ctx context.Context, plan Plan, srv *isolation.Server, db string, measureStart time.Time) (*migrate.Outcome, error) {
	cfg := plan.Config
	startAt := measureStart.Add(time.Duration(cfg.Migration.StartAfterSeconds * float64(time.Second)))
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(time.Until(startAt)):
	}
	cred, _ := srv.App()
	if cfg.Migration.RunAs == "superuser" {
		cred = srv.Admin()
	}
	conn, err := srv.Connect(ctx, cred, isolation.ConnOptions{Database: db, ApplicationName: migrate.AppName})
	if err != nil {
		return nil, err
	}
	defer isolation.Close(conn)
	return migrate.Execute(ctx, conn, plan.Migration, migrate.Options{
		LockTimeout:      time.Duration(cfg.Migration.LockTimeoutMS) * time.Millisecond,
		StatementTimeout: time.Duration(cfg.Migration.StatementTimeoutMS) * time.Millisecond,
		Transaction:      sqlinspect.TransactionMode(cfg.Migration.Transaction),
		Reference:        measureStart,
	})
}

func hasProbes(plan Plan) bool {
	return plan.Workload != nil && len(plan.Workload.Compatibility) > 0
}

// runProbes runs the compatibility probes as the application role. Before
// and after the migration the same parameter values are drawn.
func runProbes(ctx context.Context, plan Plan, srv *isolation.Server, db string, step Step) ([]workload.ProbeResult, error) {
	app, _ := srv.App()
	conn, err := srv.Connect(ctx, app, isolation.ConnOptions{Database: db, ApplicationName: "prodshadow/compatibility"})
	if err != nil {
		return nil, err
	}
	defer isolation.Close(conn)
	res, err := workload.RunProbes(ctx, conn, plan.Workload, plan.Config.Workload.Seed, fmt.Sprintf("repeat-%d", step.Repeat))
	if err != nil {
		return nil, fmt.Errorf("compatibility probes: %w", err)
	}
	return res, nil
}

// pairProbes joins the before and after results of each probe by version
// and ID, and redacts server messages. A probe without an after result was
// not measured and is left out rather than reported as broken.
func pairProbes(plan Plan, before, after []workload.ProbeResult) []Compatibility {
	type key struct{ version, id string }
	afterByKey := make(map[key]workload.ProbeResult, len(after))
	for _, a := range after {
		afterByKey[key{a.Version, a.ID}] = a
	}
	out := make([]Compatibility, 0, len(before))
	for _, b := range before {
		a, ok := afterByKey[key{b.Version, b.ID}]
		if !ok {
			continue
		}
		out = append(out, Compatibility{
			Version: b.Version, Probe: b.ID,
			BeforeOK: b.OK, BeforeSQLState: b.SQLState, BeforeError: plan.Redactor.String(b.Error),
			AfterOK: a.OK, AfterSQLState: a.SQLState, AfterError: plan.Redactor.String(a.Error),
		})
	}
	return out
}

// redactOutcome removes secrets from server error messages.
func redactOutcome(plan Plan, out *migrate.Outcome) {
	out.Error = plan.Redactor.String(out.Error)
	for i := range out.Statements {
		out.Statements[i].Error = plan.Redactor.String(out.Statements[i].Error)
	}
}
