package sqlinspect

import (
	"slices"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

// learnRename keeps the catalog in step with ALTER ... RENAME, so the
// statements after a rename are resolved under the new names.
func (c *Catalog) learnRename(st *pg_query.RenameStmt) {
	switch st.RenameType {
	case pg_query.ObjectType_OBJECT_TABLE, pg_query.ObjectType_OBJECT_VIEW, pg_query.ObjectType_OBJECT_MATVIEW,
		pg_query.ObjectType_OBJECT_FOREIGN_TABLE, pg_query.ObjectType_OBJECT_SEQUENCE:
		old := relName(st.Relation)
		renamed := st.Newname
		if st.Relation.GetSchemaname() != "" {
			renamed = st.Relation.Schemaname + "." + st.Newname
		}
		c.renameRelation(old, renamed)
	case pg_query.ObjectType_OBJECT_INDEX:
		if t, ok := c.indexTable[catalogKey(relName(st.Relation))]; ok {
			delete(c.indexTable, catalogKey(relName(st.Relation)))
			c.indexTable[catalogKey(st.Newname)] = t
		}
	case pg_query.ObjectType_OBJECT_COLUMN:
		c.renameColumn(relName(st.Relation), st.Subname, st.Newname)
	}
}

// renameKey moves a map entry from one relation name to another.
func renameKey[V any](m map[string]V, from, to string) {
	if v, ok := m[catalogKey(from)]; ok {
		delete(m, catalogKey(from))
		m[catalogKey(to)] = v
	}
}

// renameIn replaces a relation name in a list of names.
func renameIn(names []string, from, to string) {
	for i, n := range names {
		if catalogKey(n) == catalogKey(from) {
			names[i] = to
		}
	}
}

func (c *Catalog) renameRelation(from, to string) {
	renameKey(c.partitions, from, to)
	renameKey(c.partitioned, from, to)
	renameKey(c.defaultPartition, from, to)
	renameKey(c.viewReads, from, to)
	renameKey(c.matviews, from, to)
	renameKey(c.plainUnique, from, to)
	renameKey(c.viewColumns, from, to)
	renameKey(c.viewOutputs, from, to)
	renameKey(c.columns, from, to)
	renameKey(c.sequences, from, to)
	renameKey(c.triggers, from, to)
	renameKey(c.rules, from, to)
	renameKey(c.primaryKeys, from, to)
	renameKey(c.primaryKeyName, from, to)
	renameKey(c.keys, from, to)
	for idx, t := range c.indexTable {
		if catalogKey(t) == catalogKey(from) {
			c.indexTable[idx] = to
		}
	}
	for p, parts := range c.partitions {
		renameIn(parts, from, to)
		if catalogKey(c.defaultPartition[p]) == catalogKey(from) {
			c.defaultPartition[p] = to
		}
	}
	for _, reads := range c.viewReads {
		renameIn(reads, from, to)
	}
	for _, cols := range c.viewColumns {
		for _, views := range cols {
			renameIn(views, from, to)
		}
	}
	for i := range c.foreignKeys {
		if catalogKey(c.foreignKeys[i].table) == catalogKey(from) {
			c.foreignKeys[i].table = to
		}
		if catalogKey(c.foreignKeys[i].refTable) == catalogKey(from) {
			c.foreignKeys[i].refTable = to
		}
	}
	for _, seqs := range c.sequences {
		for i := range seqs {
			if catalogKey(seqs[i].sequence) == catalogKey(from) {
				seqs[i].sequence = to
			}
		}
	}
}

func (c *Catalog) renameColumn(table, from, to string) {
	key := catalogKey(table)
	renameIn(c.columns[key], from, to)
	renameIn(c.primaryKeys[key], from, to)
	for _, cols := range c.keys[key] {
		renameIn(cols, from, to)
	}
	for i, cs := range c.sequences[key] {
		if cs.column == from {
			c.sequences[key][i].column = to
		}
	}
	if views, ok := c.viewColumns[key][from]; ok {
		delete(c.viewColumns[key], from)
		c.viewColumns[key][to] = views
	}
	for i := range c.foreignKeys {
		fk := &c.foreignKeys[i]
		if catalogKey(fk.table) == key {
			renameIn(fk.columns, from, to)
		}
		if catalogKey(fk.refTable) == key {
			renameIn(fk.refColumns, from, to)
		}
	}
}

// forgetRelation removes a dropped table (or view, sequence, ...) from the
// catalog: its foreign keys in both directions, its partitions' link to it,
// its indexes, columns, triggers and rules, and its place in other views'
// and partitioned tables' lists. Later statements of the migration then no
// longer see it.
func (c *Catalog) forgetRelation(name string) {
	key := catalogKey(name)
	for _, m := range []map[string][]string{c.partitions, c.viewReads, c.viewOutputs, c.columns, c.primaryKeys} {
		delete(m, key)
	}
	delete(c.partitioned, key)
	delete(c.defaultPartition, key)
	delete(c.matviews, key)
	delete(c.plainUnique, key)
	delete(c.viewColumns, key)
	delete(c.sequences, key)
	delete(c.triggers, key)
	delete(c.rules, key)
	delete(c.primaryKeyName, key)
	delete(c.keys, key)
	for idx, t := range c.indexTable {
		if catalogKey(t) == key {
			delete(c.indexTable, idx)
		}
	}
	for p, parts := range c.partitions {
		c.partitions[p] = slices.DeleteFunc(parts, func(t string) bool { return catalogKey(t) == key })
		if catalogKey(c.defaultPartition[p]) == key {
			delete(c.defaultPartition, p)
		}
	}
	c.foreignKeys = slices.DeleteFunc(c.foreignKeys, func(fk catalogFK) bool {
		return catalogKey(fk.table) == key || catalogKey(fk.refTable) == key
	})
}

// dropColumn removes a dropped column: from the column order, from the
// columns filled from sequences (a column default goes with its column),
// and the foreign keys that include it (PostgreSQL drops them too).
func (c *Catalog) dropColumn(table, column string) {
	key := catalogKey(table)
	c.columns[key] = slices.DeleteFunc(c.columns[key], func(col string) bool { return col == column })
	c.sequences[key] = slices.DeleteFunc(c.sequences[key], func(cs columnSequence) bool { return cs.column == column })
	c.foreignKeys = slices.DeleteFunc(c.foreignKeys, func(fk catalogFK) bool {
		return catalogKey(fk.table) == key && slices.Contains(fk.columns, column)
	})
	delete(c.viewColumns[key], column)
}
