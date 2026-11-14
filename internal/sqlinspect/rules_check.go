package sqlinspect

import (
	"fmt"
	"slices"
	"strings"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"

	pg_query "github.com/pganalyze/pg_query_go/v6"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func init() {
	register(Rule{
		ID:       "PS151",
		Name:     "non-immutable-check",
		Summary:  "A CHECK constraint depends on the time, randomness or the session, which PostgreSQL does not re-evaluate",
		Severity: evidence.Warning,
		DocsURL:  "https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-CHECK-CONSTRAINTS",
		check:    checkNonImmutableCheck,
	})
}

// changingFunctions return different results over time or between
// sessions; PostgreSQL assumes CHECK expressions do not.
var changingFunctions = []string{
	"now", "transaction_timestamp", "statement_timestamp", "clock_timestamp", "timeofday",
	"random", "gen_random_uuid", "current_setting", "pg_backend_pid", "inet_client_addr",
}

// checkNonImmutableCheck flags CHECK constraints that call functions whose
// result changes, or use CURRENT_DATE, CURRENT_TIMESTAMP, CURRENT_USER and
// similar. Rows are only checked when written, and a dump may fail to
// restore once old rows no longer pass.
func checkNonImmutableCheck(c *Context, s *Statement) []Finding {
	if s.node == nil {
		return nil
	}
	var out []Finding
	report := func(table string, con *pg_query.Constraint) {
		if con == nil || con.Contype != pg_query.ConstrType_CONSTR_CHECK || con.RawExpr == nil {
			return
		}
		uses := changingParts(con.RawExpr)
		if len(uses) == 0 {
			return
		}
		f := at(s, fmt.Sprintf("CHECK constraint %s on %s uses %s, whose value changes over time or between sessions. PostgreSQL only checks a row when it is written, so rows that passed can fail later, and restoring a dump of the table can then fail.", nameOr(con.Conname, "(unnamed)"), table, strings.Join(uses, ", ")))
		f.Table = table
		f.Evidence, f.Confidence = evidence.Static, evidence.High
		f.Remediation = "Enforce time- or session-dependent rules in a trigger or in the application, and keep CHECK constraints to immutable expressions."
		out = append(out, f)
	}
	switch {
	case s.node.GetCreateStmt() != nil:
		st := s.node.GetCreateStmt()
		for _, con := range createTableConstraints(st) {
			report(relName(st.Relation), con)
		}
	case s.Kind == KindAlterTable:
		table, cons := addedConstraints(c, s)
		if table == "" && s.node.GetAlterTableStmt() != nil {
			// New tables are skipped by addedConstraints; the hazard does
			// not depend on existing traffic, so check them too.
			table = relName(s.node.GetAlterTableStmt().Relation)
			for _, n := range s.node.GetAlterTableStmt().Cmds {
				if cmd := n.GetAlterTableCmd(); cmd != nil && cmd.Subtype == pg_query.AlterTableType_AT_AddConstraint {
					cons = append(cons, cmd.Def.GetConstraint())
				}
			}
		}
		for _, con := range cons {
			report(table, con)
		}
	}
	return out
}

// changingParts lists the time-, randomness- or session-dependent parts of
// expr in order of appearance.
func changingParts(expr *pg_query.Node) []string {
	var out []string
	walk(expr, func(m protoreflect.Message) bool {
		switch n := m.Interface().(type) {
		case *pg_query.FuncCall:
			if name := lastName(qualifiedName(n.Funcname)); slices.Contains(changingFunctions, name) && !slices.Contains(out, name+"()") {
				out = append(out, name+"()")
			}
		case *pg_query.SQLValueFunction:
			name := strings.TrimSuffix(strings.TrimPrefix(n.Op.String(), "SVFOP_"), "_N")
			if !slices.Contains(out, name) {
				out = append(out, name)
			}
		}
		return true
	})
	return out
}
