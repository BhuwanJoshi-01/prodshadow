package sqlinspect

import (
	"errors"
	"fmt"

	pg_query "github.com/pganalyze/pg_query_go/v6"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// ParseOne parses and classifies a single statement, such as a workload
// query. It fails if sql does not contain exactly one statement.
func ParseOne(sql string) (*Statement, error) {
	res, err := pg_query.Parse(sql)
	if err != nil {
		return nil, err
	}
	if len(res.Stmts) != 1 {
		return nil, fmt.Errorf("expected exactly one statement, found %d", len(res.Stmts))
	}
	st := &Statement{SQL: sql, Parsed: true, node: res.Stmts[0].Stmt}
	classify(st)
	return st, nil
}

// MaxParam returns the highest positional parameter ($n) referenced by the
// statement, or 0 when it uses none.
func (s *Statement) MaxParam() int {
	if s.node == nil {
		return 0
	}
	max := 0
	walk(s.node, func(m protoreflect.Message) bool {
		if p, ok := m.Interface().(*pg_query.ParamRef); ok && int(p.Number) > max {
			max = int(p.Number)
		}
		return true
	})
	return max
}

// ErrNotDML is returned for workload statements that are not plain queries.
var ErrNotDML = errors.New("only SELECT, INSERT, UPDATE, DELETE and MERGE are allowed in workloads")

// RequireDML checks that s is a statement suitable for a workload scenario.
func (s *Statement) RequireDML() error {
	switch s.Kind {
	case KindSelect, KindInsert, KindUpdate, KindDelete, KindMerge:
		if s.node.GetSelectStmt().GetIntoClause() != nil {
			return fmt.Errorf("%w: SELECT INTO creates a table", ErrNotDML)
		}
		return nil
	default:
		return fmt.Errorf("%w (got %s)", ErrNotDML, s.Tag)
	}
}
