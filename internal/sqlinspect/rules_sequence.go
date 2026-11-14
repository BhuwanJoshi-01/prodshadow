package sqlinspect

import (
	"fmt"
	"strings"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/pglock"

	pg_query "github.com/pganalyze/pg_query_go/v6"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func init() {
	register(Rule{
		ID:       "PS156",
		Name:     "sequence-change-blocks-inserts",
		Summary:  "ALTER SEQUENCE takes SHARE ROW EXCLUSIVE, which waits for and blocks every transaction that calls nextval() on it",
		Severity: evidence.Warning,
		DocsURL:  "https://www.postgresql.org/docs/current/sql-altersequence.html",
		check:    checkSequenceChange,
	})
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

// checkSequenceChange flags ALTER SEQUENCE on an existing sequence. Inserts
// that draw from the sequence hold ROW EXCLUSIVE on it until they commit,
// and ALTER SEQUENCE's SHARE ROW EXCLUSIVE conflicts with it in both
// directions.
func checkSequenceChange(c *Context, s *Statement) []Finding {
	st := s.node.GetAlterSeqStmt()
	if st == nil {
		return nil
	}
	seq := relName(st.Sequence)
	if c.CreatedEarlier(s, seq) {
		return nil
	}
	users := "inserts that use its values"
	if cols := c.Options.Catalog.TablesUsingSequence(seq); len(cols) > 0 {
		users = "inserts into " + strings.Join(cols, ", ")
	}
	f := at(s, fmt.Sprintf("ALTER SEQUENCE %s takes SHARE ROW EXCLUSIVE on it. Every transaction that called nextval() on the sequence holds ROW EXCLUSIVE until it commits, so the change waits for in-flight %s and blocks new ones while it waits.", seq, users))
	f.Table, f.Lock = seq, pglock.ShareRowExclusive
	f.Remediation = "Run it with a short lock_timeout and retries, at a quiet time; to change a column's sequence, create a new sequence and switch the column default instead."
	return []Finding{f}
}
