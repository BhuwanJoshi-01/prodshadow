package sqlinspect

import (
	"fmt"
	"strings"

	"github.com/BhuwanJoshi-01/prodshadow/internal/evidence"
)

func init() {
	register(Rule{
		ID:       "PS128",
		Name:     "enum-value-used-before-commit",
		Summary:  "A value added with ALTER TYPE ... ADD VALUE cannot be used until the transaction commits",
		Severity: evidence.Critical,
		DocsURL:  "https://www.postgresql.org/docs/current/sql-altertype.html",
		check:    checkEnumValueUse,
	})
}

// checkEnumValueUse flags a statement that uses an enum value added earlier
// in the same transaction. PostgreSQL rejects it with "unsafe use of new
// value", unless the enum type itself was created in that transaction.
func checkEnumValueUse(c *Context, s *Statement) []Finding {
	if s.node == nil || s.node.GetAlterEnumStmt() == nil {
		return nil
	}
	st := s.node.GetAlterEnumStmt()
	if st.NewVal == "" || st.OldVal != "" { // RENAME VALUE
		return nil
	}
	g := c.Group(s)
	if !g.InTransaction() {
		return nil
	}
	typ := qualifiedName(st.TypeName)
	literal := "'" + strings.ReplaceAll(st.NewVal, "'", "''") + "'"
	after := false
	for _, o := range g.Statements {
		if o == s {
			after = true
			continue
		}
		if !after {
			if createsEnum(o, typ) {
				return nil
			}
			continue
		}
		if !strings.Contains(o.SQL, literal) {
			continue
		}
		f := at(o, fmt.Sprintf("Statement %s uses %s, which statement %s adds to enum %s in the same transaction; PostgreSQL rejects this with \"unsafe use of new value\" until the transaction commits.", o.ID, literal, s.ID, typ))
		f.Evidence, f.Confidence = evidence.Static, evidence.Medium
		f.Remediation = "Add the enum value in its own transaction (a separate migration file, or migration.transaction: per-file) and use it in a later one."
		return []Finding{f}
	}
	return nil
}

// createsEnum reports whether s is CREATE TYPE typ AS ENUM.
func createsEnum(s *Statement, typ string) bool {
	if s.node == nil || s.node.GetCreateEnumStmt() == nil {
		return false
	}
	return lastName(qualifiedName(s.node.GetCreateEnumStmt().TypeName)) == lastName(typ)
}
