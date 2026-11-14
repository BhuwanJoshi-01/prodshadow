package sqlinspect

import "strings"

// Created records which statement of a migration creates each relation, so
// locks on relations the migration itself created can be told apart from
// locks on relations the application already uses.
type Created struct {
	byName   map[string]int
	position map[*Statement]int
	stmts    []*Statement
}

// NewCreated indexes the relations created by stmts. Only the first
// statement creating a name counts.
func NewCreated(stmts []*Statement) *Created {
	c := &Created{byName: map[string]int{}, position: map[*Statement]int{}, stmts: stmts}
	for i, s := range stmts {
		c.position[s] = i
		if name := createdRelation(s); name != "" {
			if _, dup := c.byName[name]; !dup {
				c.byName[name] = i
			}
		}
	}
	return c
}

// By returns the statement that created table before s, or nil when table
// existed before the migration (or is created later).
func (c *Created) By(s *Statement, table string) *Statement {
	i, ok := c.byName[table]
	if !ok {
		// Unqualified and public-qualified names refer to the same table
		// under the default search_path.
		if alt, found := strings.CutPrefix(table, "public."); found {
			i, ok = c.byName[alt]
		} else {
			i, ok = c.byName["public."+table]
		}
	}
	pos, known := c.position[s]
	if !ok || !known || i >= pos {
		return nil
	}
	return c.stmts[i]
}

// ExistingLock returns the strongest lock s takes on relations that existed
// before the migration: the lock that can make application sessions wait.
func (c *Created) ExistingLock(s *Statement) TableLock {
	var best TableLock
	for _, l := range s.Locks {
		if c.By(s, l.Table) == nil && l.Mode > best.Mode {
			best = l
		}
	}
	return best
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
