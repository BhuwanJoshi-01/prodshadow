package sqlinspect

import (
	"slices"
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/pglock"
)

// Catalog describes the database a migration runs against, as far as a
// schema file shows it. Some statements lock relations they do not name:
// DROP INDEX locks the index's table, DDL on a partitioned table locks its
// partitions, and refreshing a materialized view reads the tables of its
// query. With a catalog the classifier can predict those locks too.
type Catalog struct {
	indexTable  map[string]string   // index -> its table
	partitions  map[string][]string // partitioned table -> direct partitions
	partitioned map[string]bool     // tables declared with PARTITION BY
	viewReads   map[string][]string // view or materialized view -> relations its query reads
	columns     map[string][]string // table -> column names in order
	sequences   map[string][]columnSequence
	foreignKeys []catalogFK
	primaryKeys map[string][]string // table -> primary key columns
	// primaryKeyName and keys name the primary key and unique
	// constraints of each table (keys: constraint -> columns).
	primaryKeyName map[string]string
	keys           map[string]map[string][]string
}

// columnSequence is a column filled from a sequence by default: serial,
// identity or DEFAULT nextval('...').
type columnSequence struct {
	column   string
	sequence string
}

// NewCatalog learns indexes, partitions and views from schema statements,
// typically the parsed fixture schema.
func NewCatalog(schema []*Statement) *Catalog {
	c := &Catalog{indexTable: map[string]string{}, partitions: map[string][]string{}, partitioned: map[string]bool{}, viewReads: map[string][]string{},
		columns: map[string][]string{}, sequences: map[string][]columnSequence{}, primaryKeys: map[string][]string{},
		primaryKeyName: map[string]string{}, keys: map[string]map[string][]string{}}
	for _, s := range schema {
		c.learn(s)
	}
	return c
}

// catalogKey normalises a relation name: unqualified and public-qualified
// names refer to the same relation under the default search_path.
func catalogKey(name string) string {
	return strings.ToLower(strings.TrimPrefix(name, "public."))
}

// TableOfIndex returns the table an index belongs to, or "".
func (c *Catalog) TableOfIndex(index string) string {
	if c == nil {
		return ""
	}
	return c.indexTable[catalogKey(index)]
}

// IsPartitioned reports whether the schema declares table with PARTITION BY.
func (c *Catalog) IsPartitioned(table string) bool {
	return c != nil && c.partitioned[catalogKey(table)]
}

// TablesUsingSequence returns the columns ("table.column") filled from a
// sequence by default, or nil.
func (c *Catalog) TablesUsingSequence(seq string) []string {
	if c == nil {
		return nil
	}
	var out []string
	for table, cols := range c.sequences {
		for _, cs := range cols {
			if catalogKey(cs.sequence) == catalogKey(seq) {
				out = append(out, table+"."+cs.column)
			}
		}
	}
	slices.Sort(out)
	return out
}

// Partitions returns every partition of a partitioned table, including
// partitions of partitions, or nil.
func (c *Catalog) Partitions(table string) []string {
	if c == nil {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	var visit func(string)
	visit = func(t string) {
		for _, p := range c.partitions[catalogKey(t)] {
			if !seen[catalogKey(p)] {
				seen[catalogKey(p)] = true
				out = append(out, p)
				visit(p)
			}
		}
	}
	visit(table)
	return out
}

// learn records what a statement adds to the schema.
func (c *Catalog) learn(s *Statement) {
	if s.node == nil {
		return
	}
	switch x := s.node.Node.(type) {
	case *pg_query.Node_IndexStmt:
		if x.IndexStmt.Idxname != "" {
			c.indexTable[catalogKey(x.IndexStmt.Idxname)] = relName(x.IndexStmt.Relation)
		}
	case *pg_query.Node_CreateStmt:
		table := relName(x.CreateStmt.Relation)
		c.learnTableConstraints(x.CreateStmt)
		for _, e := range x.CreateStmt.TableElts {
			if col := e.GetColumnDef(); col != nil {
				c.columns[catalogKey(table)] = append(c.columns[catalogKey(table)], col.Colname)
				c.learnColumnSequence(x.CreateStmt.Relation, col.Colname, col.TypeName, col.Constraints)
			}
		}
		for _, con := range createTableConstraints(x.CreateStmt) {
			if name := constraintIndexName(x.CreateStmt.Relation.GetRelname(), con); name != "" {
				c.indexTable[catalogKey(name)] = table
			}
		}
		if x.CreateStmt.Partspec != nil {
			c.partitioned[catalogKey(table)] = true
		}
		if x.CreateStmt.Partbound != nil {
			for _, parent := range x.CreateStmt.InhRelations {
				p := catalogKey(relName(parent.GetRangeVar()))
				c.partitions[p] = append(c.partitions[p], table)
			}
		}
	case *pg_query.Node_AlterTableStmt:
		table := relName(x.AlterTableStmt.Relation)
		for _, cmd := range x.AlterTableStmt.Cmds {
			at := cmd.GetAlterTableCmd()
			switch at.GetSubtype() {
			case pg_query.AlterTableType_AT_AddConstraint:
				if con := at.GetDef().GetConstraint(); con != nil {
					c.learnForeignKey(x.AlterTableStmt.Relation, "", con)
					c.learnKey(x.AlterTableStmt.Relation, "", con)
					if name := constraintIndexName(x.AlterTableStmt.Relation.GetRelname(), con); name != "" {
						c.indexTable[catalogKey(name)] = table
					}
				}
			case pg_query.AlterTableType_AT_AddColumn:
				if col := at.GetDef().GetColumnDef(); col != nil {
					c.columns[catalogKey(table)] = append(c.columns[catalogKey(table)], col.Colname)
					c.learnColumnSequence(x.AlterTableStmt.Relation, col.Colname, col.TypeName, col.Constraints)
				}
			case pg_query.AlterTableType_AT_ColumnDefault:
				if seq := nextvalSequence(at.GetDef()); seq != "" {
					c.setColumnSequence(table, at.Name, seq)
				}
			case pg_query.AlterTableType_AT_AddIdentity:
				c.setColumnSequence(table, at.Name, defaultSequenceName(x.AlterTableStmt.Relation, at.Name))
			case pg_query.AlterTableType_AT_AttachPartition:
				if part := relName(at.GetDef().GetPartitionCmd().GetName()); part != "" {
					c.partitions[catalogKey(table)] = append(c.partitions[catalogKey(table)], part)
				}
			}
		}
	case *pg_query.Node_CreateTableAsStmt:
		if x.CreateTableAsStmt.Objtype == pg_query.ObjectType_OBJECT_MATVIEW {
			c.viewReads[catalogKey(relName(x.CreateTableAsStmt.GetInto().GetRel()))] = lockedTables(s)
		}
	case *pg_query.Node_ViewStmt:
		view := relName(x.ViewStmt.View)
		var reads []string
		for _, t := range lockedTables(s) {
			if catalogKey(t) != catalogKey(view) {
				reads = append(reads, t)
			}
		}
		c.viewReads[catalogKey(view)] = reads
	}
}

// learnColumnSequence records the sequence a new column draws from: the
// implicit <table>_<column>_seq of serial and identity columns, or the one
// named in DEFAULT nextval('...').
func (c *Catalog) learnColumnSequence(rel *pg_query.RangeVar, column string, tn *pg_query.TypeName, cons []*pg_query.Node) {
	table := relName(rel)
	switch typeName(tn) {
	case "serial", "bigserial", "smallserial", "serial2", "serial4", "serial8":
		c.setColumnSequence(table, column, defaultSequenceName(rel, column))
		return
	}
	for _, n := range cons {
		con := n.GetConstraint()
		switch {
		case con == nil:
		case con.Contype == pg_query.ConstrType_CONSTR_IDENTITY:
			c.setColumnSequence(table, column, defaultSequenceName(rel, column))
		case con.Contype == pg_query.ConstrType_CONSTR_DEFAULT:
			if seq := nextvalSequence(con.RawExpr); seq != "" {
				c.setColumnSequence(table, column, seq)
			}
		}
	}
}

func (c *Catalog) setColumnSequence(table, column, seq string) {
	key := catalogKey(table)
	for i, cs := range c.sequences[key] {
		if cs.column == column {
			c.sequences[key][i].sequence = seq
			return
		}
	}
	c.sequences[key] = append(c.sequences[key], columnSequence{column: column, sequence: seq})
}

// defaultSequenceName is the name PostgreSQL gives the sequence of a serial
// or identity column, in the table's schema.
func defaultSequenceName(rel *pg_query.RangeVar, column string) string {
	name := rel.GetRelname() + "_" + column + "_seq"
	if rel.GetSchemaname() != "" {
		return rel.GetSchemaname() + "." + name
	}
	return name
}

// nextvalSequence returns the sequence of a nextval('...') expression, or "".
func nextvalSequence(n *pg_query.Node) string {
	fc := n.GetFuncCall()
	if fc == nil || len(fc.Args) == 0 {
		return ""
	}
	if names := stringItems(fc.Funcname); len(names) == 0 || strings.ToLower(names[len(names)-1]) != "nextval" {
		return ""
	}
	return regclassLiteral(fc.Args[0])
}

// insertSequences adds ROW EXCLUSIVE on the sequences an INSERT draws from:
// those of columns it leaves to their default, explicitly or by omission.
func (c *Catalog) insertSequences(s *Statement, ins *pg_query.InsertStmt) {
	table := relName(ins.Relation)
	seqs := c.sequences[catalogKey(table)]
	if len(seqs) == 0 {
		return
	}
	var cols []string
	for _, n := range ins.Cols {
		cols = append(cols, n.GetResTarget().GetName())
	}
	explicit := cols != nil
	if !explicit {
		cols = c.columns[catalogKey(table)]
	}
	rows := ins.GetSelectStmt().GetSelectStmt().GetValuesLists()
	for _, cs := range seqs {
		idx := slices.Index(cols, cs.column)
		usesDefault := false
		switch {
		case idx < 0:
			// Omitted from the column list, or a column the catalog does
			// not know the position of: the default applies.
			usesDefault = explicit || len(cols) == 0
		case ins.GetSelectStmt().GetSelectStmt() == nil:
			usesDefault = true // INSERT ... DEFAULT VALUES
		case len(rows) > 0:
			for _, r := range rows {
				items := r.GetList().GetItems()
				if idx >= len(items) || items[idx].GetSetToDefault() != nil {
					usesDefault = true
				}
			}
		case !explicit:
			// INSERT ... SELECT without a column list: the query's columns
			// fill the table's first columns; assume they cover it.
		}
		if usesDefault {
			s.addImpliedLock(cs.sequence, pglock.RowExclusive, "column "+strings.TrimPrefix(table, "public.")+"."+cs.column)
		}
	}
}

func lockedTables(s *Statement) []string {
	out := make([]string, 0, len(s.Locks))
	for _, l := range s.Locks {
		out = append(out, l.Table)
	}
	return out
}

// constraintIndexName returns the name of the index a PRIMARY KEY or
// UNIQUE constraint creates: its own name, or PostgreSQL's default
// <table>_pkey for an unnamed primary key. Unnamed unique constraints get
// generated names that are not predicted.
func constraintIndexName(table string, con *pg_query.Constraint) string {
	switch con.Contype {
	case pg_query.ConstrType_CONSTR_PRIMARY, pg_query.ConstrType_CONSTR_UNIQUE:
	default:
		return ""
	}
	if con.Indexname != "" {
		return con.Indexname
	}
	if con.Conname != "" {
		return con.Conname
	}
	if con.Contype == pg_query.ConstrType_CONSTR_PRIMARY {
		return table + "_pkey"
	}
	return ""
}

// Resolve adds the locks stmts take on relations they do not name, as far
// as the catalog knows them, and learns from each statement for the ones
// after it (an index created by the migration and dropped later in it).
// Added locks carry Via, which names the object that implies them.
func (c *Catalog) Resolve(stmts []*Statement) {
	if c == nil {
		return
	}
	for _, s := range stmts {
		c.resolve(s)
		if s.node != nil {
			// Before learning: a dropped foreign key is forgotten here.
			c.ddlForeignKeyLocks(s)
		}
		c.learn(s)
	}
	markCreated(stmts)
}

func (c *Catalog) resolve(s *Statement) {
	if s.node == nil {
		return
	}
	switch x := s.node.Node.(type) {
	case *pg_query.Node_DropStmt:
		if x.DropStmt.RemoveType != pg_query.ObjectType_OBJECT_INDEX {
			break
		}
		// DROP INDEX locks the index's table like the index itself.
		mode := pglock.AccessExclusive
		if x.DropStmt.Concurrent {
			mode = pglock.ShareUpdateExclusive
		}
		for _, idx := range dropObjectNames(x.DropStmt.Objects) {
			if t := c.TableOfIndex(idx); t != "" {
				s.addImpliedLock(t, mode, "index "+idx)
			}
		}
	case *pg_query.Node_ReindexStmt:
		if x.ReindexStmt.Kind == pg_query.ReindexObjectType_REINDEX_OBJECT_TABLE {
			c.partitionedMaintenance(s, relName(x.ReindexStmt.Relation))
			break
		}
		if x.ReindexStmt.Kind != pg_query.ReindexObjectType_REINDEX_OBJECT_INDEX {
			break
		}
		idx := relName(x.ReindexStmt.Relation)
		if t := c.TableOfIndex(idx); t != "" {
			mode := pglock.Share
			if s.Txn == TxnNotInBlock { // REINDEX INDEX CONCURRENTLY
				mode = pglock.ShareUpdateExclusive
			}
			s.addImpliedLock(t, mode, "index "+idx)
		}
	case *pg_query.Node_InsertStmt:
		c.insertSequences(s, x.InsertStmt)
		c.dmlForeignKeyLocks(s)
	case *pg_query.Node_UpdateStmt, *pg_query.Node_DeleteStmt:
		c.dmlForeignKeyLocks(s)
	case *pg_query.Node_ClusterStmt:
		if x.ClusterStmt.Relation != nil {
			c.partitionedMaintenance(s, relName(x.ClusterStmt.Relation))
		}
	case *pg_query.Node_RefreshMatViewStmt:
		view := relName(x.RefreshMatViewStmt.Relation)
		for _, t := range c.viewReads[catalogKey(view)] {
			s.addImpliedLock(t, pglock.AccessShare, "materialized view "+view)
		}
	case *pg_query.Node_IndexStmt:
		if x.IndexStmt.Relation.GetInh() {
			c.propagate(s, relName(x.IndexStmt.Relation))
		}
	case *pg_query.Node_AlterTableStmt:
		if !x.AlterTableStmt.Relation.GetInh() {
			return
		}
		for _, cmd := range x.AlterTableStmt.Cmds {
			switch cmd.GetAlterTableCmd().GetSubtype() {
			case pg_query.AlterTableType_AT_AttachPartition, pg_query.AlterTableType_AT_DetachPartition,
				pg_query.AlterTableType_AT_DetachPartitionFinalize:
				// These lock the parent and the one partition they name.
				return
			}
		}
		c.propagate(s, relName(x.AlterTableStmt.Relation))
	}
	switch s.Kind {
	case KindDropTable, KindTruncate, KindLockTable:
		for _, rv := range namedRelations(s.node) {
			if rv.GetInh() {
				c.propagate(s, relName(rv))
			}
		}
	}
}

// partitionedMaintenance handles REINDEX TABLE and CLUSTER on a
// partitioned table: PostgreSQL processes each partition in its own
// transaction, so the statement cannot run inside a transaction block, and
// each partition is locked in turn.
func (c *Catalog) partitionedMaintenance(s *Statement, table string) {
	if !c.IsPartitioned(table) {
		return
	}
	s.Txn = TxnNotInBlock
	c.propagate(s, table)
}

// propagate gives every partition of table the lock s takes on table:
// DDL on a partitioned table recurses to its partitions.
func (c *Catalog) propagate(s *Statement, table string) {
	var mode pglock.Mode
	for _, l := range s.Locks {
		if catalogKey(l.Table) == catalogKey(table) {
			mode = l.Mode
		}
	}
	for _, p := range c.Partitions(table) {
		s.addImpliedLock(p, mode, "partition of "+table)
	}
}

// namedRelations returns the relations a DROP TABLE, TRUNCATE or LOCK
// statement names.
func namedRelations(n *pg_query.Node) []*pg_query.RangeVar {
	var out []*pg_query.RangeVar
	switch {
	case n.GetTruncateStmt() != nil:
		for _, r := range n.GetTruncateStmt().Relations {
			out = append(out, r.GetRangeVar())
		}
	case n.GetLockStmt() != nil:
		for _, r := range n.GetLockStmt().Relations {
			out = append(out, r.GetRangeVar())
		}
	case n.GetDropStmt() != nil:
		// DROP TABLE has no ONLY; partitions are always dropped.
		for _, name := range dropObjectNames(n.GetDropStmt().Objects) {
			schema, rel, found := strings.Cut(name, ".")
			if !found {
				schema, rel = "", name
			}
			out = append(out, &pg_query.RangeVar{Schemaname: schema, Relname: rel, Inh: true})
		}
	}
	return out
}
