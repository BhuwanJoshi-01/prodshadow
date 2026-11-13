package sqlinspect

import "testing"

func parse(t *testing.T, files ...string) []*Statement {
	t.Helper()
	srcs := make([]Source, 0, len(files))
	for i, f := range files {
		srcs = append(srcs, Source{Path: string(rune('a'+i)) + ".sql", Content: f})
	}
	stmts, err := ParseSources(srcs)
	if err != nil {
		t.Fatal(err)
	}
	return stmts
}

func TestPlanAutoWrapsPlainFiles(t *testing.T) {
	groups := Plan(parse(t, "ALTER TABLE t ADD c int; UPDATE t SET c = 1;"), TxnModeAuto)
	if len(groups) != 1 || !groups[0].Wrapped || len(groups[0].Statements) != 2 {
		t.Fatalf("unexpected plan %+v", groups)
	}
}

func TestPlanAutoDoesNotWrapConcurrentIndex(t *testing.T) {
	groups := Plan(parse(t, "CREATE INDEX CONCURRENTLY i ON t (a); ANALYZE t;"), TxnModeAuto)
	if len(groups) != 2 || groups[0].InTransaction() || groups[1].InTransaction() {
		t.Fatalf("unexpected plan %+v", groups)
	}
}

func TestPlanFollowsExplicitTransactions(t *testing.T) {
	groups := Plan(parse(t, "SET lock_timeout = '1s'; BEGIN; ALTER TABLE t ADD c int; UPDATE t SET c = 1; COMMIT; VACUUM t;"), TxnModeAuto)
	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(groups))
	}
	if groups[0].InTransaction() || !groups[1].Explicit || len(groups[1].Statements) != 4 || groups[2].InTransaction() {
		t.Fatalf("unexpected plan %+v", groups)
	}
}

func TestPlanPerFileAndNone(t *testing.T) {
	stmts := parse(t, "ALTER TABLE t ADD c int;", "UPDATE t SET c = 1; DELETE FROM t WHERE c = 2;")
	per := Plan(stmts, TxnModePerFile)
	if len(per) != 2 || !per[0].Wrapped || !per[1].Wrapped {
		t.Fatalf("per-file plan %+v", per)
	}
	none := Plan(stmts, TxnModeNone)
	if len(none) != 3 {
		t.Fatalf("none plan should autocommit each statement, got %d groups", len(none))
	}
}

func TestPlanUnclosedBegin(t *testing.T) {
	groups := Plan(parse(t, "BEGIN; ALTER TABLE t ADD c int;"), TxnModeNone)
	if len(groups) != 1 || !groups[0].Explicit {
		t.Fatalf("unexpected plan %+v", groups)
	}
}
