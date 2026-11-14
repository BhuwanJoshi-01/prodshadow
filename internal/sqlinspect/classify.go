package sqlinspect

import (
	"fmt"
	"strings"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/pglock"

	pg_query "github.com/pganalyze/pg_query_go/v6"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// classify derives Kind, Tag, locks and the transaction requirement from
// the parse tree. Lock levels follow the PostgreSQL manual; where a level
// depends on information not present in the SQL text (for example the
// current column type), the stronger level is assumed.
func classify(s *Statement) {
	s.Txn = TxnAny
	if s.node == nil {
		s.Kind, s.Tag = KindUnknown, "UNKNOWN"
		return
	}
	switch x := s.node.Node.(type) {
	case *pg_query.Node_IndexStmt:
		classifyCreateIndex(s, x.IndexStmt)
	case *pg_query.Node_AlterTableStmt:
		classifyAlterTable(s, x.AlterTableStmt)
	case *pg_query.Node_RenameStmt:
		classifyRename(s, x.RenameStmt)
	case *pg_query.Node_AlterObjectSchemaStmt:
		s.Kind, s.Tag = KindAlterTable, "ALTER ... SET SCHEMA"
		s.addLock(relName(x.AlterObjectSchemaStmt.Relation), pglock.AccessExclusive)
	case *pg_query.Node_DropStmt:
		classifyDrop(s, x.DropStmt)
	case *pg_query.Node_CreateStmt:
		classifyCreateTable(s, x.CreateStmt)
	case *pg_query.Node_InsertStmt:
		s.Kind, s.Tag = KindInsert, "INSERT"
		s.addLock(relName(x.InsertStmt.Relation), pglock.RowExclusive)
		addReadLocks(s, x.InsertStmt, pglock.AccessShare)
	case *pg_query.Node_UpdateStmt:
		s.Kind, s.Tag = KindUpdate, "UPDATE"
		s.addLock(relName(x.UpdateStmt.Relation), pglock.RowExclusive)
		addReadLocks(s, x.UpdateStmt, pglock.AccessShare)
	case *pg_query.Node_DeleteStmt:
		s.Kind, s.Tag = KindDelete, "DELETE"
		s.addLock(relName(x.DeleteStmt.Relation), pglock.RowExclusive)
		addReadLocks(s, x.DeleteStmt, pglock.AccessShare)
	case *pg_query.Node_SelectStmt:
		s.Kind, s.Tag = KindSelect, "SELECT"
		mode := pglock.AccessShare
		if len(x.SelectStmt.LockingClause) > 0 {
			s.Tag, mode = "SELECT ... FOR "+lockingStrength(x.SelectStmt.LockingClause), pglock.RowShare
		}
		addReadLocks(s, x.SelectStmt, mode)
	case *pg_query.Node_TruncateStmt:
		s.Kind, s.Tag = KindTruncate, "TRUNCATE"
		for _, r := range x.TruncateStmt.Relations {
			s.addLock(relName(r.GetRangeVar()), pglock.AccessExclusive)
		}
	case *pg_query.Node_VacuumStmt:
		classifyVacuum(s, x.VacuumStmt)
	case *pg_query.Node_ClusterStmt:
		s.Kind, s.Tag = KindCluster, "CLUSTER"
		if x.ClusterStmt.Relation == nil {
			// Reclusters every previously clustered table, one
			// transaction per table.
			s.Txn = TxnNotInBlock
		}
		s.addLock(relName(x.ClusterStmt.Relation), pglock.AccessExclusive)
	case *pg_query.Node_ReindexStmt:
		classifyReindex(s, x.ReindexStmt)
	case *pg_query.Node_RefreshMatViewStmt:
		s.Kind, s.Tag = KindRefreshMatView, "REFRESH MATERIALIZED VIEW"
		mode := pglock.AccessExclusive
		if x.RefreshMatViewStmt.Concurrent {
			s.Tag += " CONCURRENTLY"
			mode = pglock.Exclusive
		}
		s.addLock(relName(x.RefreshMatViewStmt.Relation), mode)
	case *pg_query.Node_LockStmt:
		s.Kind, s.Tag = KindLockTable, "LOCK"
		mode := pglock.Mode(x.LockStmt.Mode)
		if !mode.Valid() || mode == pglock.None {
			mode = pglock.AccessExclusive
		}
		for _, r := range x.LockStmt.Relations {
			s.addLock(relName(r.GetRangeVar()), mode)
		}
	case *pg_query.Node_CreateTrigStmt:
		s.Kind, s.Tag = KindCreateTrigger, "CREATE TRIGGER"
		s.addLock(relName(x.CreateTrigStmt.Relation), pglock.ShareRowExclusive)
	case *pg_query.Node_TransactionStmt:
		s.Kind, s.Txn = KindTransaction, TxnControl
		s.Tag = transactionTag(x.TransactionStmt.Kind)
	case *pg_query.Node_VariableSetStmt:
		s.Kind, s.Tag = KindSet, "SET"
		if name := x.VariableSetStmt.Name; name != "" {
			s.Subcommands = []string{strings.ToLower(name)}
		}
	case *pg_query.Node_DoStmt:
		s.Kind, s.Tag = KindProcedural, "DO"
	case *pg_query.Node_CallStmt:
		s.Kind, s.Tag = KindProcedural, "CALL"
	case *pg_query.Node_CreateExtensionStmt:
		s.Kind, s.Tag = KindCreateExtension, "CREATE EXTENSION"
		s.Subcommands = []string{x.CreateExtensionStmt.Extname}
	case *pg_query.Node_CopyStmt:
		s.Kind, s.Tag = KindCopy, "COPY"
		mode := pglock.AccessShare
		if x.CopyStmt.IsFrom {
			mode = pglock.RowExclusive
		}
		s.addLock(relName(x.CopyStmt.Relation), mode)
	case *pg_query.Node_CreateTableAsStmt:
		s.Kind, s.Tag = KindOtherDDL, "CREATE TABLE AS"
		if x.CreateTableAsStmt.Objtype == pg_query.ObjectType_OBJECT_MATVIEW {
			s.Tag = "CREATE MATERIALIZED VIEW"
		}
		// Parse analysis of the query locks every table it reads, even
		// WITH NO DATA.
		addReadLocks(s, x.CreateTableAsStmt.Query, pglock.AccessShare)
	case *pg_query.Node_ViewStmt:
		s.Kind, s.Tag = KindOtherDDL, "CREATE VIEW"
		addReadLocks(s, x.ViewStmt.Query, pglock.AccessShare)
		if x.ViewStmt.Replace {
			// Replacing an existing view takes ACCESS EXCLUSIVE on it,
			// which blocks every query that uses the view.
			s.Tag = "CREATE OR REPLACE VIEW"
			s.addLock(relName(x.ViewStmt.View), pglock.AccessExclusive)
		}
	case *pg_query.Node_CommentStmt:
		s.Kind, s.Tag = KindComment, "COMMENT"
		// Commenting on a relation or a column takes SHARE UPDATE
		// EXCLUSIVE on the relation: it does not block queries, but it
		// queues behind a pending ACCESS EXCLUSIVE request and conflicts
		// with VACUUM, ANALYZE and concurrent index builds.
		s.addLock(commentRelation(x.CommentStmt))
	case *pg_query.Node_AlterSystemStmt:
		s.Kind, s.Tag, s.Txn = KindOtherDDL, "ALTER SYSTEM", TxnNotInBlock
	case *pg_query.Node_CreatedbStmt:
		s.Kind, s.Tag, s.Txn = KindOtherDDL, "CREATE DATABASE", TxnNotInBlock
	case *pg_query.Node_DropdbStmt:
		s.Kind, s.Tag, s.Txn = KindOtherDDL, "DROP DATABASE", TxnNotInBlock
	case *pg_query.Node_CreateTableSpaceStmt:
		s.Kind, s.Tag, s.Txn = KindOtherDDL, "CREATE TABLESPACE", TxnNotInBlock
	case *pg_query.Node_DropTableSpaceStmt:
		s.Kind, s.Tag, s.Txn = KindOtherDDL, "DROP TABLESPACE", TxnNotInBlock
	default:
		s.Kind = KindOtherDDL
		s.Tag = strings.ToUpper(strings.TrimSuffix(strings.TrimPrefix(nodeTypeName(s.node), "Node_"), "Stmt"))
	}
}

func classifyCreateIndex(s *Statement, st *pg_query.IndexStmt) {
	s.Kind, s.Tag = KindCreateIndex, "CREATE INDEX"
	if st.Unique {
		s.Tag = "CREATE UNIQUE INDEX"
	}
	if st.Concurrent {
		s.Tag += " CONCURRENTLY"
		s.Txn = TxnNotInBlock
		s.addLock(relName(st.Relation), pglock.ShareUpdateExclusive)
		return
	}
	s.addLock(relName(st.Relation), pglock.Share)
}

func classifyRename(s *Statement, st *pg_query.RenameStmt) {
	s.Kind, s.Tag = KindRename, "ALTER ... RENAME"
	mode := pglock.AccessExclusive
	switch st.RenameType {
	case pg_query.ObjectType_OBJECT_INDEX:
		// Renaming an index takes SHARE UPDATE EXCLUSIVE since PG 12.
		mode = pglock.ShareUpdateExclusive
		s.Subcommands = []string{"rename_index"}
	case pg_query.ObjectType_OBJECT_COLUMN:
		s.Subcommands = []string{"rename_column"}
	case pg_query.ObjectType_OBJECT_TABLE:
		s.Subcommands = []string{"rename_table"}
	case pg_query.ObjectType_OBJECT_TABCONSTRAINT:
		s.Subcommands = []string{"rename_constraint"}
	default:
		s.Subcommands = []string{"rename_" + strings.ToLower(strings.TrimPrefix(st.RenameType.String(), "OBJECT_"))}
	}
	s.addLock(relName(st.Relation), mode)
}

func classifyDrop(s *Statement, st *pg_query.DropStmt) {
	names := dropObjectNames(st.Objects)
	switch st.RemoveType {
	case pg_query.ObjectType_OBJECT_TABLE:
		s.Kind, s.Tag = KindDropTable, "DROP TABLE"
		for _, n := range names {
			s.addLock(n, pglock.AccessExclusive)
		}
	case pg_query.ObjectType_OBJECT_INDEX:
		s.Kind, s.Tag = KindDropIndex, "DROP INDEX"
		mode := pglock.AccessExclusive
		if st.Concurrent {
			s.Tag += " CONCURRENTLY"
			s.Txn = TxnNotInBlock
			mode = pglock.ShareUpdateExclusive
		}
		// The lock is taken on the index's table, which is not named in
		// the statement; it is recorded against the index.
		for _, n := range names {
			s.addLock(n, mode)
		}
	case pg_query.ObjectType_OBJECT_VIEW, pg_query.ObjectType_OBJECT_MATVIEW, pg_query.ObjectType_OBJECT_SEQUENCE:
		s.Kind = KindDropOther
		s.Tag = "DROP " + strings.ReplaceAll(strings.TrimPrefix(st.RemoveType.String(), "OBJECT_"), "MATVIEW", "MATERIALIZED VIEW")
		for _, n := range names {
			s.addLock(n, pglock.AccessExclusive)
		}
	default:
		s.Kind = KindDropOther
		s.Tag = "DROP " + strings.TrimPrefix(st.RemoveType.String(), "OBJECT_")
	}
}

// commentRelation returns the relation a COMMENT ON statement locks and the
// lock mode, or "" for objects that are not relations or parts of one.
func commentRelation(st *pg_query.CommentStmt) (string, pglock.Mode) {
	items := stringItems(st.GetObject().GetList().GetItems())
	switch st.Objtype {
	case pg_query.ObjectType_OBJECT_TABLE, pg_query.ObjectType_OBJECT_VIEW, pg_query.ObjectType_OBJECT_MATVIEW,
		pg_query.ObjectType_OBJECT_FOREIGN_TABLE, pg_query.ObjectType_OBJECT_SEQUENCE, pg_query.ObjectType_OBJECT_INDEX:
		return strings.Join(items, "."), pglock.ShareUpdateExclusive
	case pg_query.ObjectType_OBJECT_COLUMN:
		// The last name is the column; the rest name its relation.
		if len(items) > 1 {
			return strings.Join(items[:len(items)-1], "."), pglock.ShareUpdateExclusive
		}
	case pg_query.ObjectType_OBJECT_TABCONSTRAINT, pg_query.ObjectType_OBJECT_TRIGGER,
		pg_query.ObjectType_OBJECT_RULE, pg_query.ObjectType_OBJECT_POLICY:
		// The lock is taken on the constraint, trigger, rule or policy
		// itself; its relation (all but the last name) is only read.
		if len(items) > 1 {
			return strings.Join(items[:len(items)-1], "."), pglock.AccessShare
		}
	}
	return "", pglock.None
}

// dropObjectNames renders DropStmt objects, which are lists of String nodes.
func dropObjectNames(objs []*pg_query.Node) []string {
	var out []string
	for _, o := range objs {
		if l := o.GetList(); l != nil {
			out = append(out, strings.Join(stringItems(l.Items), "."))
		}
	}
	return out
}

func stringItems(items []*pg_query.Node) []string {
	var out []string
	for _, it := range items {
		if s := it.GetString_(); s != nil {
			out = append(out, s.Sval)
		}
	}
	return out
}

func classifyCreateTable(s *Statement, st *pg_query.CreateStmt) {
	s.Kind, s.Tag = KindCreateTable, "CREATE TABLE"
	// Foreign keys take SHARE ROW EXCLUSIVE on the referenced table.
	for _, c := range createTableConstraints(st) {
		if c.Contype == pg_query.ConstrType_CONSTR_FOREIGN {
			s.addLock(relName(c.Pktable), pglock.ShareRowExclusive)
		}
	}
	// CREATE TABLE ... PARTITION OF takes ACCESS EXCLUSIVE on the parent,
	// because the partition descriptor changes. Plain INHERITS takes SHARE
	// UPDATE EXCLUSIVE, which is self-conflicting so that two children
	// cannot update the parent's catalog entry at once.
	parentMode := pglock.ShareUpdateExclusive
	if st.Partbound != nil {
		s.Subcommands = append(s.Subcommands, "partition_of")
		parentMode = pglock.AccessExclusive
	}
	for _, parent := range st.InhRelations {
		s.addLock(relName(parent.GetRangeVar()), parentMode)
	}
	// LIKE reads the source table's definition under ACCESS SHARE.
	for _, e := range st.TableElts {
		if like := e.GetTableLikeClause(); like != nil {
			s.addLock(relName(like.Relation), pglock.AccessShare)
		}
	}
}

func createTableConstraints(st *pg_query.CreateStmt) []*pg_query.Constraint {
	var out []*pg_query.Constraint
	for _, c := range st.Constraints {
		if con := c.GetConstraint(); con != nil {
			out = append(out, con)
		}
	}
	for _, e := range st.TableElts {
		if con := e.GetConstraint(); con != nil {
			out = append(out, con)
		}
		if col := e.GetColumnDef(); col != nil {
			for _, c := range col.Constraints {
				if con := c.GetConstraint(); con != nil {
					out = append(out, con)
				}
			}
		}
	}
	return out
}

func classifyVacuum(s *Statement, st *pg_query.VacuumStmt) {
	full := false
	for _, o := range st.Options {
		if d := o.GetDefElem(); d != nil && strings.EqualFold(d.Defname, "full") {
			full = defElemTrue(d)
		}
	}
	mode := pglock.ShareUpdateExclusive
	if st.IsVacuumcmd {
		s.Kind, s.Tag, s.Txn = KindVacuum, "VACUUM", TxnNotInBlock
		if full {
			s.Tag, mode = "VACUUM FULL", pglock.AccessExclusive
		}
	} else {
		s.Kind, s.Tag = KindVacuum, "ANALYZE"
	}
	for _, r := range st.Rels {
		if vr := r.GetVacuumRelation(); vr != nil {
			s.addLock(relName(vr.Relation), mode)
		}
	}
}

func classifyReindex(s *Statement, st *pg_query.ReindexStmt) {
	s.Kind, s.Tag = KindReindex, "REINDEX"
	concurrent := false
	for _, p := range st.Params {
		if d := p.GetDefElem(); d != nil && strings.EqualFold(d.Defname, "concurrently") {
			concurrent = defElemTrue(d)
		}
	}
	switch st.Kind {
	case pg_query.ReindexObjectType_REINDEX_OBJECT_SCHEMA, pg_query.ReindexObjectType_REINDEX_OBJECT_SYSTEM, pg_query.ReindexObjectType_REINDEX_OBJECT_DATABASE:
		// These process one table at a time, each in its own transaction,
		// so they cannot run inside a transaction block. The tables are
		// not named in the statement, so no locks are recorded.
		s.Tag += " " + strings.TrimPrefix(st.Kind.String(), "REINDEX_OBJECT_")
		if concurrent {
			s.Tag += " CONCURRENTLY"
		}
		s.Txn = TxnNotInBlock
		return
	}
	if concurrent {
		s.Tag += " CONCURRENTLY"
		s.Txn = TxnNotInBlock
		s.addLock(relName(st.Relation), pglock.ShareUpdateExclusive)
		return
	}
	if st.Kind == pg_query.ReindexObjectType_REINDEX_OBJECT_INDEX {
		s.addLock(relName(st.Relation), pglock.AccessExclusive)
		return
	}
	s.addLock(relName(st.Relation), pglock.Share)
}

// defElemTrue interprets a boolean option such as FULL or FULL true.
func defElemTrue(d *pg_query.DefElem) bool {
	if d.Arg == nil {
		return true
	}
	if b := d.Arg.GetBoolean(); b != nil {
		return b.Boolval
	}
	if s := d.Arg.GetString_(); s != nil {
		v := strings.ToLower(s.Sval)
		return v == "true" || v == "on" || v == "1"
	}
	if i := d.Arg.GetInteger(); i != nil {
		return i.Ival != 0
	}
	// Unreserved words such as on and off parse as type names in
	// option lists.
	if tn := d.Arg.GetTypeName(); tn != nil {
		v := typeName(tn)
		return v == "true" || v == "on" || v == "yes" || v == "1"
	}
	return true
}

func transactionTag(k pg_query.TransactionStmtKind) string {
	switch k {
	case pg_query.TransactionStmtKind_TRANS_STMT_BEGIN, pg_query.TransactionStmtKind_TRANS_STMT_START:
		return "BEGIN"
	case pg_query.TransactionStmtKind_TRANS_STMT_COMMIT:
		return "COMMIT"
	case pg_query.TransactionStmtKind_TRANS_STMT_ROLLBACK:
		return "ROLLBACK"
	case pg_query.TransactionStmtKind_TRANS_STMT_SAVEPOINT:
		return "SAVEPOINT"
	case pg_query.TransactionStmtKind_TRANS_STMT_RELEASE:
		return "RELEASE"
	case pg_query.TransactionStmtKind_TRANS_STMT_ROLLBACK_TO:
		return "ROLLBACK TO"
	default:
		return strings.TrimPrefix(k.String(), "TRANS_STMT_")
	}
}

// nodeTypeName returns the Go type name of the node's oneof value.
func nodeTypeName(n *pg_query.Node) string {
	if n == nil || n.Node == nil {
		return ""
	}
	return strings.TrimPrefix(fmt.Sprintf("%T", n.Node), "*pg_query.")
}

// addReadLocks records mode on every relation that root reads, skipping the
// names of common table expressions. Relations that already have a lock
// keep the stronger of the two.
func addReadLocks(s *Statement, root proto.Message, mode pglock.Mode) {
	ctes := map[string]bool{}
	walk(root, func(m protoreflect.Message) bool {
		if cte, ok := m.Interface().(*pg_query.CommonTableExpr); ok {
			ctes[cte.Ctename] = true
		}
		return true
	})
	walk(root, func(m protoreflect.Message) bool {
		rv, ok := m.Interface().(*pg_query.RangeVar)
		if !ok {
			return true
		}
		if rv.Schemaname == "" && ctes[rv.Relname] {
			return true
		}
		s.addLock(relName(rv), mode)
		return true
	})
}

// lockingStrength names the row lock of a SELECT's first locking clause.
func lockingStrength(clauses []*pg_query.Node) string {
	lc := clauses[0].GetLockingClause()
	if lc == nil {
		return "UPDATE"
	}
	switch lc.Strength {
	case pg_query.LockClauseStrength_LCS_FORKEYSHARE:
		return "KEY SHARE"
	case pg_query.LockClauseStrength_LCS_FORSHARE:
		return "SHARE"
	case pg_query.LockClauseStrength_LCS_FORNOKEYUPDATE:
		return "NO KEY UPDATE"
	default:
		return "UPDATE"
	}
}
