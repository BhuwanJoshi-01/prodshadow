package workload

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/BhuwanJoshi-01/prodshadow/internal/gen"

	"github.com/jackc/pgx/v5"
)

// Connector opens a database session for a workload client.
type Connector func(ctx context.Context, appName string) (*pgx.Conn, error)

// session is one client connection with its own parameter generators.
type session struct {
	name    string
	connect Connector
	conn    *pgx.Conn
	timeout time.Duration
	gens    map[*compiled][]gen.Generator
	rng     map[*compiled][]*rand.Rand
	seed    int64
	stream  string
}

func newSession(name string, connect Connector, seed int64, stream string) *session {
	return &session{name: name, connect: connect, seed: seed, stream: stream,
		gens: map[*compiled][]gen.Generator{}, rng: map[*compiled][]*rand.Rand{}}
}

// ensureConn connects or reconnects the session.
func (s *session) ensureConn(ctx context.Context) error {
	if s.conn != nil && !s.conn.IsClosed() {
		return nil
	}
	conn, err := s.connect(ctx, "prodshadow/"+s.name)
	if err != nil {
		return err
	}
	s.conn, s.timeout = conn, 0
	return nil
}

func (s *session) close() {
	if s.conn != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.conn.Close(ctx)
	}
}

// args draws the next parameter values for c.
func (s *session) args(c *compiled) ([]any, error) {
	gens, ok := s.gens[c]
	if !ok {
		gens = make([]gen.Generator, len(c.params))
		rngs := make([]*rand.Rand, len(c.params))
		for i, p := range c.params {
			g, err := gen.New(p)
			if err != nil {
				return nil, err
			}
			gens[i] = g
			rngs[i] = gen.Stream(s.seed, fmt.Sprintf("%s/%s/%s/p%d", s.stream, s.name, c.info.ID, i))
		}
		s.gens[c], s.rng[c] = gens, rngs
	}
	out := make([]any, len(gens))
	for i, g := range gens {
		out[i] = g.Next(s.rng[c][i])
	}
	return out, nil
}

// execute runs one scenario and returns the time spent excluding any
// deliberate hold, so held transactions do not inflate latency.
func (s *session) execute(ctx context.Context, c *compiled, args []any) (time.Duration, error) {
	if err := s.ensureConn(ctx); err != nil {
		return 0, err
	}
	if s.timeout != c.timeout {
		if _, err := s.conn.Exec(ctx, fmt.Sprintf("SET statement_timeout = %d", c.timeout.Milliseconds())); err != nil {
			return 0, err
		}
		s.timeout = c.timeout
	}
	begin := time.Now()
	if !c.txn {
		_, err := s.conn.Exec(ctx, c.stmts[0].sql, args[:c.stmts[0].params]...)
		return time.Since(begin), err
	}
	tx, err := s.conn.Begin(ctx)
	if err != nil {
		return time.Since(begin), err
	}
	rollback := func() {
		rctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = tx.Rollback(rctx)
	}
	for _, st := range c.stmts {
		if _, err := tx.Exec(ctx, st.sql, args[:st.params]...); err != nil {
			rollback()
			return time.Since(begin), err
		}
	}
	worked := time.Since(begin)
	if c.hold > 0 {
		select {
		case <-ctx.Done():
			rollback()
			return worked, ctx.Err()
		case <-time.After(c.hold):
		}
	}
	commitStart := time.Now()
	err = tx.Commit(ctx)
	return worked + time.Since(commitStart), err
}
