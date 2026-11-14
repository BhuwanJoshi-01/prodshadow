package workload

import (
	"fmt"
	"time"

	"github.com/BhuwanJoshi-01/prodshadow/internal/gen"
	"github.com/BhuwanJoshi-01/prodshadow/internal/sqlinspect"
)

// DefaultTimeout is the statement_timeout of workload sessions when a
// scenario sets none. It keeps a stuck worker from outliving the run.
const DefaultTimeout = 30 * time.Second

// ScenarioInfo identifies a scenario in results; Op.Scenario indexes into
// Result.Scenarios.
type ScenarioInfo struct {
	ID         string  `json:"id"`
	Mode       string  `json:"mode"`
	Weight     float64 `json:"weight,omitempty"`
	Background bool    `json:"background,omitempty"`
}

type stmt struct {
	sql    string
	params int
}

type compiled struct {
	info    ScenarioInfo
	stmts   []stmt
	txn     bool
	hold    time.Duration
	timeout time.Duration
	think   time.Duration
	params  []gen.Spec
	cum     float64
	// background only
	sessions   int
	startAfter time.Duration
	repeat     bool
}

// compile prepares scenarios for execution. Foreground scenarios come first
// so that their indices match weighted selection.
func compile(s *Spec) ([]*compiled, []*compiled, error) {
	var fg, bg []*compiled
	total := 0.0
	for _, sc := range s.Scenarios {
		total += sc.Weight
	}
	cum := 0.0
	for _, sc := range s.Scenarios {
		c := &compiled{info: ScenarioInfo{ID: sc.ID, Mode: sc.Mode, Weight: sc.Weight}, params: sc.Parameters, timeout: DefaultTimeout,
			think: time.Duration(s.ThinkTimeMS) * time.Millisecond}
		if sc.TimeoutMS > 0 {
			c.timeout = time.Duration(sc.TimeoutMS) * time.Millisecond
		}
		if sc.ThinkTimeMS != nil {
			c.think = time.Duration(*sc.ThinkTimeMS) * time.Millisecond
		}
		raw := []string{sc.SQL}
		if sc.Transaction != nil {
			raw, c.txn, c.hold = sc.Transaction.Statements, true, time.Duration(sc.Transaction.HoldMS)*time.Millisecond
		}
		var err error
		if c.stmts, err = compileStatements(raw); err != nil {
			return nil, nil, fmt.Errorf("scenario %s: %w", sc.ID, err)
		}
		cum += sc.Weight / total
		c.cum = cum
		fg = append(fg, c)
	}
	if len(fg) > 0 {
		fg[len(fg)-1].cum = 1
	}
	for _, b := range s.Background {
		c := &compiled{
			info:       ScenarioInfo{ID: b.ID, Mode: "background", Background: true},
			params:     b.Parameters,
			txn:        true,
			hold:       time.Duration(b.Transaction.HoldMS) * time.Millisecond,
			timeout:    DefaultTimeout,
			sessions:   b.Sessions,
			startAfter: time.Duration(b.StartAfterSeconds * float64(time.Second)),
			repeat:     b.Repeat,
		}
		var err error
		if c.stmts, err = compileStatements(b.Transaction.Statements); err != nil {
			return nil, nil, fmt.Errorf("background %s: %w", b.ID, err)
		}
		bg = append(bg, c)
	}
	return fg, bg, nil
}

func compileStatements(raw []string) ([]stmt, error) {
	out := make([]stmt, len(raw))
	for i, sql := range raw {
		st, err := sqlinspect.ParseOne(sql)
		if err != nil {
			return nil, err
		}
		out[i] = stmt{sql: sql, params: st.MaxParam()}
	}
	return out, nil
}

// pick selects a foreground scenario index for a uniform draw x in [0,1).
func pick(fg []*compiled, x float64) int {
	for i, c := range fg {
		if x < c.cum {
			return i
		}
	}
	return len(fg) - 1
}
