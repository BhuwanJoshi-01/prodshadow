package sqlinspect

import (
	"encoding/json"
	"slices"
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

// Trigger event bits as stored in CreateTrigStmt.Events (PostgreSQL's
// TRIGGER_TYPE_* values).
const (
	triggerInsert   = 1 << 2
	triggerDelete   = 1 << 3
	triggerUpdate   = 1 << 4
	triggerTruncate = 1 << 5
)

// catalogTrigger is a trigger known to the catalog.
type catalogTrigger struct {
	name     string
	function string
	events   int32
}

// learnFunction records the tables a PL/pgSQL or SQL function's statements
// lock, as far as they are static SQL. Dynamic SQL (EXECUTE) is not seen.
func (c *Catalog) learnFunction(s *Statement, st *pg_query.CreateFunctionStmt) {
	names := stringItems(st.Funcname)
	if len(names) == 0 {
		return
	}
	var language string
	var body []string
	for _, o := range st.Options {
		d := o.GetDefElem()
		switch d.GetDefname() {
		case "language":
			language = strings.ToLower(d.GetArg().GetString_().GetSval())
		case "as":
			body = stringItems(d.GetArg().GetList().GetItems())
		}
	}
	var queries []string
	switch language {
	case "plpgsql":
		queries = plpgsqlQueries(s.SQL)
	case "sql":
		if len(body) > 0 {
			queries = []string{body[0]}
		}
	}
	var locks []TableLock
	if st.SqlBody != nil {
		// BEGIN ATOMIC ... END: the statements are already parsed.
		walkStatementsOf(st.SqlBody, func(n *pg_query.Node) {
			fs := &Statement{node: n, Parsed: true}
			classify(fs)
			locks = append(locks, fs.Locks...)
		})
	}
	for _, q := range queries {
		stmts, err := ParseSources([]Source{{Path: "function", Content: q}})
		if err != nil {
			continue
		}
		for _, fs := range stmts {
			locks = append(locks, fs.Locks...)
		}
	}
	c.functionLocks[catalogKey(strings.Join(names, "."))] = locks
}

// walkStatementsOf calls fn for each statement of a BEGIN ATOMIC body.
func walkStatementsOf(n *pg_query.Node, fn func(*pg_query.Node)) {
	for _, item := range n.GetList().GetItems() {
		if l := item.GetList(); l != nil {
			for _, s := range l.Items {
				fn(s)
			}
			continue
		}
		fn(item)
	}
}

// plpgsqlQueries returns the static SQL statements of a PL/pgSQL function
// (CREATE FUNCTION text): the queries of plain statements and PERFORM.
func plpgsqlQueries(createFunction string) []string {
	q, _ := plpgsqlStatements(createFunction)
	return q.static
}

// plpgsqlBody is the SQL a PL/pgSQL body runs.
type plpgsqlBody struct {
	static  []string
	dynamic bool // uses EXECUTE
}

// plpgsqlStatements parses a CREATE FUNCTION text with a PL/pgSQL body;
// ok is false when the body does not parse.
func plpgsqlStatements(createFunction string) (plpgsqlBody, bool) {
	raw, err := pg_query.ParsePlPgSqlToJSON(createFunction)
	if err != nil {
		return plpgsqlBody{}, false
	}
	var tree any
	if json.Unmarshal([]byte(raw), &tree) != nil {
		return plpgsqlBody{}, false
	}
	var out []string
	dynamic := strings.Contains(raw, "PLpgSQL_stmt_dynexecute") || strings.Contains(raw, "PLpgSQL_stmt_dynfors")
	var visit func(v any, statement bool)
	visit = func(v any, statement bool) {
		switch x := v.(type) {
		case map[string]any:
			for k, child := range x {
				switch k {
				case "PLpgSQL_stmt_execsql":
					visit(child, true)
				case "PLpgSQL_stmt_perform":
					if m, ok := child.(map[string]any); ok {
						if q := exprQuery(m["expr"]); q != "" {
							out = append(out, q)
						}
					}
				case "PLpgSQL_expr":
					q := exprQuery(map[string]any{"PLpgSQL_expr": child})
					switch {
					case q == "":
					case statement:
						out = append(out, q)
					default:
						// Conditions, assignments and RETURN values are
						// evaluated as SELECT <expression>, which locks the
						// tables their subqueries read.
						if _, rhs, ok := strings.Cut(q, ":="); ok {
							q = rhs
						}
						out = append(out, "SELECT "+q)
					}
				default:
					visit(child, statement && k == "sqlstmt")
				}
			}
		case []any:
			for _, child := range x {
				visit(child, false)
			}
		}
	}
	visit(tree, false)
	return plpgsqlBody{static: out, dynamic: dynamic}, true
}

func exprQuery(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	e, ok := m["PLpgSQL_expr"].(map[string]any)
	if !ok {
		return ""
	}
	q, _ := e["query"].(string)
	return q
}

// learnTrigger records a trigger and the events that fire it.
func (c *Catalog) learnTrigger(st *pg_query.CreateTrigStmt) {
	fn := stringItems(st.Funcname)
	if len(fn) == 0 {
		return
	}
	table := catalogKey(relName(st.Relation))
	c.triggers[table] = append(c.triggers[table], catalogTrigger{name: st.Trigname, function: catalogKey(strings.Join(fn, ".")), events: st.Events})
}

// triggerLocks adds the locks of the triggers a DML statement fires:
// whatever their functions' static SQL locks, marked "via trigger <name>".
// Row-level triggers run only for affected rows, so these are locks the
// statement may take. Triggers are followed only one level deep: a trigger's own writes may fire further triggers,
// which are not followed.
func (c *Catalog) triggerLocks(s *Statement) {
	var table string
	var events int32
	switch x := s.node.Node.(type) {
	case *pg_query.Node_InsertStmt:
		table, events = relName(x.InsertStmt.Relation), triggerInsert
	case *pg_query.Node_UpdateStmt:
		table, events = relName(x.UpdateStmt.Relation), triggerUpdate
	case *pg_query.Node_DeleteStmt:
		table, events = relName(x.DeleteStmt.Relation), triggerDelete
	case *pg_query.Node_TruncateStmt:
		for _, r := range x.TruncateStmt.Relations {
			c.fireTriggers(s, relName(r.GetRangeVar()), triggerTruncate)
		}
		return
	case *pg_query.Node_MergeStmt:
		table = relName(x.MergeStmt.Relation)
		for _, w := range x.MergeStmt.MergeWhenClauses {
			switch w.GetMergeWhenClause().GetCommandType() {
			case pg_query.CmdType_CMD_INSERT:
				events |= triggerInsert
			case pg_query.CmdType_CMD_UPDATE:
				events |= triggerUpdate
			case pg_query.CmdType_CMD_DELETE:
				events |= triggerDelete
			}
		}
	default:
		return
	}
	c.fireTriggers(s, table, events)
}

func (c *Catalog) fireTriggers(s *Statement, table string, events int32) {
	for _, tr := range c.triggers[catalogKey(table)] {
		if tr.events&events == 0 {
			continue
		}
		for _, l := range c.functionLocks[tr.function] {
			s.addImpliedLock(l.Table, l.Mode, "trigger "+tr.name)
		}
	}
}

// forgetTrigger removes a dropped trigger.
func (c *Catalog) forgetTrigger(table, name string) {
	key := catalogKey(table)
	c.triggers[key] = slices.DeleteFunc(c.triggers[key], func(t catalogTrigger) bool { return t.name == name })
}

// classifyDo predicts the locks of a PL/pgSQL DO block from the static SQL
// in its body: the statements it runs directly, including inside IF
// branches and loops, and PERFORM. Dynamic SQL (EXECUTE) cannot be seen;
// the statement records whether it uses any.
func classifyDo(s *Statement, st *pg_query.DoStmt) {
	var body, language string
	for _, a := range st.Args {
		d := a.GetDefElem()
		switch d.GetDefname() {
		case "as":
			body = d.GetArg().GetString_().GetSval()
		case "language":
			language = strings.ToLower(d.GetArg().GetString_().GetSval())
		}
	}
	if body == "" || (language != "" && language != "plpgsql") {
		return
	}
	tag := "$prodshadow_do$"
	for strings.Contains(body, tag) {
		tag = strings.TrimSuffix(tag, "$") + "_$"
	}
	fn := "CREATE FUNCTION prodshadow_do() RETURNS void LANGUAGE plpgsql AS " + tag + body + tag
	queries, ok := plpgsqlStatements(fn)
	if !ok {
		return
	}
	s.bodyAnalysed = true
	s.dynamicSQL = queries.dynamic
	for _, q := range queries.static {
		inner, err := ParseSources([]Source{{Path: "do", Content: q}})
		if err != nil {
			continue
		}
		for _, is := range inner {
			for _, l := range is.Locks {
				s.addLock(l.Table, l.Mode)
			}
		}
	}
}
