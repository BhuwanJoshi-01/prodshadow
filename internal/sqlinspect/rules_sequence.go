package sqlinspect

import (
	"fmt"
	"strings"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"

	pg_query "github.com/pganalyze/pg_query_go/v6"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func init() {
	register(Rule{
		ID:       "PS130",
		Name:     "sequence-reset",
		Summary:  "Restarting a sequence at a fixed value can hand out ids that are already in use",
		Severity: evidence.Warning,
		DocsURL:  "https://www.postgresql.org/docs/current/sql-altersequence.html",
		check:    checkSequenceReset,
	})
}

// checkSequenceReset flags ALTER SEQUENCE ... RESTART, setval() with a
// constant value, and TRUNCATE ... RESTART IDENTITY. Resetting a sequence
// to a value computed from the table, e.g. setval(..., max(id)), is not
// flagged.
func checkSequenceReset(c *Context, s *Statement) []Finding {
	if s.node == nil {
		return nil
	}
	var what []string
	switch {
	case s.node.GetAlterSeqStmt() != nil:
		st := s.node.GetAlterSeqStmt()
		for _, o := range st.Options {
			if d := o.GetDefElem(); d != nil && d.Defname == "restart" && !c.CreatedEarlier(s, relName(st.Sequence)) {
				what = append(what, "ALTER SEQUENCE "+relName(st.Sequence)+" RESTART")
			}
		}
	case s.node.GetTruncateStmt() != nil:
		if s.node.GetTruncateStmt().RestartSeqs {
			what = append(what, "TRUNCATE ... RESTART IDENTITY")
		}
	default:
		walk(s.node, func(m protoreflect.Message) bool {
			fc, ok := m.Interface().(*pg_query.FuncCall)
			if !ok || lastName(qualifiedName(fc.Funcname)) != "setval" || len(fc.Args) < 2 {
				return true
			}
			if fc.Args[1].GetAConst() != nil {
				what = append(what, "setval() with a constant value")
			}
			return true
		})
	}
	var out []Finding
	for _, w := range what {
		f := at(s, fmt.Sprintf("%s moves the sequence to a fixed value. If rows with higher ids exist in production, inserts that rely on the sequence fail with duplicate key errors until it passes them.", w))
		f.Evidence, f.Confidence = evidence.Heuristic, evidence.Medium
		f.Remediation = "Compute the new value from the data instead, e.g. SELECT setval('orders_id_seq', (SELECT max(id) FROM orders)), or leave the sequence alone."
		out = append(out, f)
	}
	// One finding per statement is enough for several setval() calls.
	if len(out) > 1 && strings.HasPrefix(what[0], "setval") {
		out = out[:1]
	}
	return out
}
