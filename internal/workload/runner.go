package workload

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/BhuwanJoshi-01/prodshadow/internal/gen"

	"github.com/jackc/pgx/v5/pgconn"
)

// ErrInterrupted marks operations that were still running when the run was
// stopped. They are not errors, but their latency is a lower bound.
const ErrInterrupted ErrClass = 100

// Options control one workload run.
type Options struct {
	Concurrency int
	// ArrivalRate > 0 selects open-loop scheduling (operations per second).
	ArrivalRate float64
	Warmup      time.Duration
	Measurement time.Duration
	Seed        int64
	// Stream namespaces random streams. Baseline and candidate runs of the
	// same repeat use the same Stream so they issue the same parameters.
	Stream  string
	Connect Connector
	// MeasureStarted is invoked once when the measurement window begins.
	MeasureStarted func(at time.Time)
	// Extend, when non-nil, keeps the run going after Measurement until it
	// is closed (for example until the migration finishes), plus Tail, but
	// never longer than MaxExtension.
	Extend       <-chan struct{}
	Tail         time.Duration
	MaxExtension time.Duration
	// Grace bounds how long in-flight operations may finish after the end.
	Grace time.Duration
}

// Result holds every operation of a run.
type Result struct {
	Scenarios       []ScenarioInfo `json:"scenarios"`
	Ops             []Op           `json:"-"`
	MeasureStart    time.Time      `json:"measure_start"`
	MeasureEnd      time.Time      `json:"measure_end"`
	Mode            string         `json:"mode"`
	Sessions        int            `json:"sessions"`
	ConnectFailures int            `json:"connect_failures"`
	// SampleErrors keeps the first error message of each scenario that
	// failed, so a broken query can be diagnosed from the report.
	SampleErrors map[string]string `json:"-"`
}

// MeasuredDuration is the length of the measurement window.
func (r *Result) MeasuredDuration() time.Duration { return r.MeasureEnd.Sub(r.MeasureStart) }

// Run executes the workload. It returns an error only for setup problems;
// operation failures are recorded in the result.
func Run(ctx context.Context, spec *Spec, opts Options) (*Result, error) {
	if opts.Concurrency < 1 {
		return nil, errors.New("workload: concurrency must be at least 1")
	}
	if opts.Measurement <= 0 {
		return nil, errors.New("workload: measurement must be positive")
	}
	if opts.Connect == nil {
		return nil, errors.New("workload: no connector")
	}
	if opts.Grace <= 0 {
		opts.Grace = 5 * time.Second
	}
	fg, bg, err := compile(spec)
	if err != nil {
		return nil, err
	}
	r := &runner{spec: spec, opts: opts, fg: fg, bg: bg}
	return r.run(ctx)
}

type runner struct {
	spec *Spec
	opts Options
	fg   []*compiled
	bg   []*compiled

	measureStart time.Time
	mu           sync.Mutex
	ops          []Op
	connFailures int
	sampleErrs   map[string]string
}

// index returns the result index of c. Validation caps the number of
// scenarios at MaxScenarios, well inside uint16.
func (r *runner) index(c *compiled) uint16 {
	for i, f := range r.fg {
		if f == c {
			return uint16(i) //nolint:gosec // bounded by MaxScenarios
		}
	}
	for i, b := range r.bg {
		if b == c {
			return uint16(len(r.fg) + i) //nolint:gosec // bounded by MaxScenarios
		}
	}
	return math.MaxUint16
}

func (r *runner) run(parent context.Context) (*Result, error) {
	start := time.Now()
	r.measureStart = start.Add(r.opts.Warmup)

	// stop ends issuing new work; kill interrupts in-flight work.
	stopCtx, stop := context.WithCancel(parent)
	killCtx, kill := context.WithCancel(parent)
	defer stop()
	defer kill()

	var wg sync.WaitGroup
	mode := "closed-loop"
	if r.opts.ArrivalRate > 0 {
		mode = "open-loop"
		r.startOpenLoop(stopCtx, killCtx, &wg)
	} else {
		for i := 0; i < r.opts.Concurrency; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				r.closedLoopWorker(stopCtx, killCtx, i)
			}(i)
		}
	}
	for _, c := range r.bg {
		for i := 0; i < c.sessions; i++ {
			wg.Add(1)
			go func(c *compiled, i int) {
				defer wg.Done()
				r.backgroundSession(stopCtx, c, i)
			}(c, i)
		}
	}

	if !sleepUntil(parent, r.measureStart) {
		stop()
		kill()
		wg.Wait()
		return nil, parent.Err()
	}
	if r.opts.MeasureStarted != nil {
		r.opts.MeasureStarted(r.measureStart)
	}
	end := r.waitForEnd(parent)
	stop()

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(r.opts.Grace):
		kill()
		<-done
	}
	if parent.Err() != nil {
		return nil, parent.Err()
	}

	scenarios := make([]ScenarioInfo, 0, len(r.fg)+len(r.bg))
	for _, c := range append(append([]*compiled{}, r.fg...), r.bg...) {
		scenarios = append(scenarios, c.info)
	}
	sessions := r.opts.Concurrency
	for _, c := range r.bg {
		sessions += c.sessions
	}
	return &Result{
		Scenarios: scenarios, Ops: r.ops, MeasureStart: r.measureStart, MeasureEnd: end,
		Mode: mode, Sessions: sessions, ConnectFailures: r.connFailures, SampleErrors: r.sampleErrs,
	}, nil
}

// waitForEnd blocks until the measurement window (plus any extension) ends.
// The tail is measured from the moment Extend closed, so a migration that
// finished early does not lengthen the window.
func (r *runner) waitForEnd(ctx context.Context) time.Time {
	end := r.measureStart.Add(r.opts.Measurement)
	var closedAt atomic.Int64
	if r.opts.Extend != nil {
		go func() {
			select {
			case <-r.opts.Extend:
				closedAt.Store(time.Now().UnixNano())
			case <-ctx.Done():
			}
		}()
	}
	sleepUntil(ctx, end)
	if r.opts.Extend == nil {
		return end
	}
	limit := time.NewTimer(r.opts.MaxExtension)
	defer limit.Stop()
	select {
	case <-r.opts.Extend:
	case <-limit.C:
		return time.Now()
	case <-ctx.Done():
		return time.Now()
	}
	// The watcher may not have stored the time yet; fall back to now.
	at := time.Now()
	for i := 0; i < 100 && closedAt.Load() == 0; i++ {
		time.Sleep(time.Millisecond)
	}
	if ns := closedAt.Load(); ns != 0 {
		at = time.Unix(0, ns)
	}
	after := at.Add(r.opts.Tail)
	if !after.After(end) {
		return end
	}
	sleepUntil(ctx, after)
	return after
}

func sleepUntil(ctx context.Context, t time.Time) bool {
	d := time.Until(t)
	if d <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func (r *runner) record(local []Op) {
	r.mu.Lock()
	r.ops = append(r.ops, local...)
	r.mu.Unlock()
}

func (r *runner) connectFailed() {
	r.mu.Lock()
	r.connFailures++
	r.mu.Unlock()
}

// op builds a record for an operation that started at begin.
func (r *runner) op(c *compiled, begin time.Time, latency time.Duration, err error, stopped bool) Op {
	class := Classify(err)
	if err != nil && stopped {
		class = ErrInterrupted
	}
	if err != nil && class != ErrInterrupted {
		r.keepSampleError(c.info.ID, err)
	}
	return Op{Scenario: r.index(c), Err: class, Start: int64(begin.Sub(r.measureStart)), Latency: int64(latency)}
}

func (r *runner) closedLoopWorker(stopCtx, killCtx context.Context, i int) {
	s := newSession(fmt.Sprintf("worker/%d", i), r.opts.Connect, r.opts.Seed, r.opts.Stream)
	defer s.close()
	choice := gen.Stream(r.opts.Seed, fmt.Sprintf("%s/worker/%d/choice", r.opts.Stream, i))
	var local []Op
	defer func() { r.record(local) }()

	for stopCtx.Err() == nil {
		if err := s.ensureConn(stopCtx); err != nil {
			if stopCtx.Err() == nil {
				r.connectFailed()
				sleepUntil(stopCtx, time.Now().Add(250*time.Millisecond))
			}
			continue
		}
		c := r.fg[pick(r.fg, choice.Float64())]
		args, err := s.args(c)
		if err != nil {
			return
		}
		begin := time.Now()
		lat, err := s.execute(killCtx, c, args)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || isConnErr(err) {
			lat = time.Since(begin)
		}
		local = append(local, r.op(c, begin, lat, err, killCtx.Err() != nil))
		if c.think > 0 {
			sleepUntil(stopCtx, time.Now().Add(c.think))
		}
	}
}

func isConnErr(err error) bool { return err != nil && Classify(err) == ErrConnection }

type job struct {
	c        *compiled
	args     []any
	intended time.Time
}

// startOpenLoop dispatches Poisson arrivals to a fixed pool of sessions.
// Latency is measured from the intended start time, so queueing caused by a
// blocked database is included (avoiding coordinated omission).
func (r *runner) startOpenLoop(stopCtx, killCtx context.Context, wg *sync.WaitGroup) {
	jobs := make(chan job, r.opts.Concurrency)
	for i := 0; i < r.opts.Concurrency; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s := newSession(fmt.Sprintf("worker/%d", i), r.opts.Connect, r.opts.Seed, r.opts.Stream)
			defer s.close()
			var local []Op
			defer func() { r.record(local) }()
			for j := range jobs {
				lat, err := s.execute(killCtx, j.c, j.args)
				if err != nil && s.conn == nil {
					r.connectFailed()
				}
				// Latency includes queueing since the intended start but
				// not a scenario's deliberate hold.
				total := time.Since(j.intended)
				if err == nil {
					total -= j.c.hold
				}
				if total < lat {
					total = lat
				}
				local = append(local, r.op(j.c, j.intended, total, err, killCtx.Err() != nil))
			}
		}(i)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(jobs)
		dispatcher := newSession("dispatcher", r.opts.Connect, r.opts.Seed, r.opts.Stream)
		arrivals := gen.Stream(r.opts.Seed, r.opts.Stream+"/arrivals")
		choice := gen.Stream(r.opts.Seed, r.opts.Stream+"/choice")
		var dropped []Op
		defer func() { r.record(dropped) }()
		next := time.Now()
		for {
			next = next.Add(time.Duration(arrivals.ExpFloat64() / r.opts.ArrivalRate * float64(time.Second)))
			if !sleepUntil(stopCtx, next) {
				return
			}
			c := r.fg[pick(r.fg, choice.Float64())]
			args, err := dispatcher.args(c)
			if err != nil {
				return
			}
			select {
			case jobs <- job{c: c, args: args, intended: next}:
			default:
				dropped = append(dropped, Op{Scenario: r.index(c), Err: ErrDropped, Start: int64(next.Sub(r.measureStart))})
			}
		}
	}()
}

// backgroundSession runs a blocker transaction, optionally repeatedly. It
// stops with the run rather than after the grace period: a held blocker is
// rolled back as soon as the measurement ends.
func (r *runner) backgroundSession(stopCtx context.Context, c *compiled, i int) {
	if !sleepUntil(stopCtx, r.measureStart.Add(c.startAfter)) {
		return
	}
	s := newSession(fmt.Sprintf("background/%s/%d", c.info.ID, i), r.opts.Connect, r.opts.Seed, r.opts.Stream)
	defer s.close()
	var local []Op
	defer func() { r.record(local) }()
	for stopCtx.Err() == nil {
		args, err := s.args(c)
		if err != nil {
			return
		}
		begin := time.Now()
		lat, err := s.execute(stopCtx, c, args)
		local = append(local, r.op(c, begin, lat, err, stopCtx.Err() != nil))
		if !c.repeat {
			return
		}
	}
}

// MaxSampleErrorLength bounds a kept error message.
const MaxSampleErrorLength = 300

// keepSampleError records the first error of a scenario, with its SQLSTATE
// when the server reported one.
func (r *runner) keepSampleError(scenario string, err error) {
	msg := err.Error()
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		msg = pgErr.Code + ": " + pgErr.Message
	}
	if len(msg) > MaxSampleErrorLength {
		msg = msg[:MaxSampleErrorLength] + "…"
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sampleErrs == nil {
		r.sampleErrs = map[string]string{}
	}
	if _, ok := r.sampleErrs[scenario]; !ok {
		r.sampleErrs[scenario] = msg
	}
}
