package sqlinspect

import (
	"sort"
	"strings"
	"time"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"
)

// Rule is a static hazard check. Rules are deliberately conservative and
// few: they explain lock behaviour that the experiment then measures. Teams
// are encouraged to keep using a dedicated migration linter alongside.
type Rule struct {
	ID       string
	Name     string
	Summary  string
	Severity evidence.Severity
	DocsURL  string

	// check runs once per statement; checkAll runs once per migration.
	check    func(c *Context, s *Statement) []Finding
	checkAll func(c *Context) []Finding
}

// Options tune rule evaluation.
type Options struct {
	// Transaction selects how files are wrapped in transactions.
	Transaction TransactionMode
	// Overrides maps rule IDs to a severity or "off".
	Overrides map[string]string
	// Now is used to evaluate annotation expiry. Zero means time.Now().
	Now time.Time
}

// Context gives rules access to the whole migration.
type Context struct {
	Statements []*Statement
	Groups     []Group
	Options    Options

	position map[*Statement]int
	groupOf  map[*Statement]int
	created  map[string]int
}

func newContext(stmts []*Statement, opts Options) *Context {
	if opts.Transaction == "" {
		opts.Transaction = TxnModeAuto
	}
	c := &Context{
		Statements: stmts,
		Groups:     Plan(stmts, opts.Transaction),
		Options:    opts,
		position:   map[*Statement]int{},
		groupOf:    map[*Statement]int{},
		created:    map[string]int{},
	}
	for i, s := range stmts {
		c.position[s] = i
		if name := createdRelation(s); name != "" {
			if _, dup := c.created[name]; !dup {
				c.created[name] = i
			}
		}
	}
	for gi, g := range c.Groups {
		for _, s := range g.Statements {
			c.groupOf[s] = gi
		}
	}
	return c
}

// createdRelation returns the relation a CREATE TABLE, CREATE TABLE AS,
// CREATE MATERIALIZED VIEW or CREATE SEQUENCE statement creates.
func createdRelation(s *Statement) string {
	if s.node == nil {
		return ""
	}
	switch {
	case s.node.GetCreateStmt() != nil:
		return relName(s.node.GetCreateStmt().Relation)
	case s.node.GetCreateTableAsStmt() != nil:
		return relName(s.node.GetCreateTableAsStmt().GetInto().GetRel())
	case s.node.GetCreateSeqStmt() != nil:
		return relName(s.node.GetCreateSeqStmt().Sequence)
	}
	return ""
}

// CreatedEarlier reports whether table was created by a statement that
// precedes s in this migration. Such tables are new and unused, so locking
// them does not affect application traffic.
func (c *Context) CreatedEarlier(s *Statement, table string) bool {
	i, ok := c.created[table]
	if !ok {
		// Unqualified and public-qualified names refer to the same table
		// under the default search_path.
		if alt, found := strings.CutPrefix(table, "public."); found {
			i, ok = c.created[alt]
		} else {
			i, ok = c.created["public."+table]
		}
	}
	return ok && i < c.position[s]
}

// Group returns the execution group containing s.
func (c *Context) Group(s *Statement) Group { return c.Groups[c.groupOf[s]] }

// registry lists all rules in ID order. Rules are added in rules_*.go.
var registry []Rule

func register(r Rule) {
	registry = append(registry, r)
	sort.Slice(registry, func(i, j int) bool { return registry[i].ID < registry[j].ID })
}

// Rules returns a copy of the registered rules.
func Rules() []Rule { return append([]Rule(nil), registry...) }

// RuleByID looks up a rule.
func RuleByID(id string) (Rule, bool) {
	for _, r := range registry {
		if r.ID == id {
			return r, true
		}
	}
	return Rule{}, false
}

// Analyze evaluates every rule, applies severity overrides and inline
// suppressions, and returns findings ordered by statement position.
func Analyze(stmts []*Statement, opts Options) []Finding {
	c := newContext(stmts, opts)
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}

	var out []Finding
	for _, r := range registry {
		setting := opts.Overrides[r.ID]
		if setting == "off" {
			continue
		}
		var found []Finding
		if r.check != nil {
			for _, s := range stmts {
				found = append(found, r.check(c, s)...)
			}
		}
		if r.checkAll != nil {
			found = append(found, r.checkAll(c)...)
		}
		for _, f := range found {
			f.RuleID, f.Rule, f.DocsURL = r.ID, r.Name, firstNonEmpty(f.DocsURL, r.DocsURL)
			if f.Severity == "" {
				f.Severity = r.Severity
			}
			if f.Confidence == "" {
				f.Confidence = evidence.High
			}
			if f.Evidence == "" {
				f.Evidence = evidence.Static
			}
			if sev, err := evidence.ParseSeverity(setting); err == nil {
				f.Severity = sev
			}
			if s := c.statementByID(f.StatementID); s != nil {
				if a, ok := s.Suppresses(r.ID, now); ok {
					f.Suppressed, f.SuppressionReason, f.SuppressionExpiry = true, a.Reason, a.Expires
				}
			}
			out = append(out, f)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		pi, pj := c.positionOf(out[i].StatementID), c.positionOf(out[j].StatementID)
		if pi != pj {
			return pi < pj
		}
		return out[i].RuleID < out[j].RuleID
	})
	return out
}

func (c *Context) statementByID(id string) *Statement {
	for _, s := range c.Statements {
		if s.ID == id {
			return s
		}
	}
	return nil
}

func (c *Context) positionOf(id string) int {
	if s := c.statementByID(id); s != nil {
		return c.position[s]
	}
	return len(c.Statements)
}

// at builds a finding located at statement s.
func at(s *Statement, msg string) Finding {
	return Finding{StatementID: s.ID, File: s.File, Line: s.StartLine, Message: msg}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
