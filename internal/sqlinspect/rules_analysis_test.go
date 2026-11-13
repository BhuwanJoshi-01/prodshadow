package sqlinspect

import (
	"testing"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"
)

func TestPS116Unparsed(t *testing.T) {
	stmts, err := ParseSources([]Source{{Path: "m.sql", Content: "ALTER TABLE orders ADD COLUMN c int; ALTER TABLE orders FROBNICATE;"}})
	if err != nil {
		t.Fatal(err)
	}
	if stmts[1].Parsed || stmts[1].Kind != KindUnknown {
		t.Fatalf("second statement should fail to parse: %+v", stmts[1])
	}
	var got []Finding
	for _, f := range Analyze(stmts, Options{}) {
		if f.RuleID == "PS116" {
			got = append(got, f)
		}
	}
	expectCount(t, got, 1)
	if got[0].Severity != evidence.Inconclusive || got[0].StatementID != "1.2" {
		t.Fatalf("unexpected finding %+v", got[0])
	}
}

func TestPS117Procedural(t *testing.T) {
	expectCount(t, findings(t, "PS117", Options{}, "DO $$ BEGIN PERFORM 1; END $$;"), 1)
	expectCount(t, findings(t, "PS117", Options{}, "CALL refresh_stats();"), 1)
}

func TestRuleCatalogIsComplete(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range Rules() {
		if seen[r.ID] {
			t.Errorf("duplicate rule ID %s", r.ID)
		}
		seen[r.ID] = true
		if r.Name == "" || r.Summary == "" || !r.Severity.Valid() {
			t.Errorf("rule %s is missing metadata", r.ID)
		}
		if (r.check == nil) == (r.checkAll == nil) {
			t.Errorf("rule %s must define exactly one check function", r.ID)
		}
	}
	if _, ok := RuleByID("PS101"); !ok {
		t.Error("RuleByID failed for PS101")
	}
	if _, ok := RuleByID("PS999"); ok {
		t.Error("RuleByID found a rule that does not exist")
	}
}

func TestAnalyzeOrdersFindingsByStatement(t *testing.T) {
	stmts := parse(t, "UPDATE orders SET a = 1;\nCREATE INDEX i ON orders (a);")
	fs := Analyze(stmts, Options{Transaction: TxnModeNone})
	for i := 1; i < len(fs); i++ {
		if fs[i-1].Line > fs[i].Line && fs[i].Line != 0 {
			t.Fatalf("findings out of order: %+v", fs)
		}
	}
}
