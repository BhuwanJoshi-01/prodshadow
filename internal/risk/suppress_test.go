package risk

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeSup(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "suppressions.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadSuppressions(t *testing.T) {
	sups, err := LoadSuppressions(writeSup(t, `version: 1
suppressions:
  - rule: PS101
    file: migrations/0042.sql
    reason: "orders_archive has fewer than 100 rows"
    expires: 2027-01-31
    approved_by: dba-team
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(sups) != 1 || sups[0].ApprovedBy != "dba-team" {
		t.Fatalf("unexpected %+v", sups)
	}
}

func TestLoadSuppressionsRejectsWeakEntries(t *testing.T) {
	for name, body := range map[string]string{
		"short reason": "version: 1\nsuppressions:\n  - rule: PS101\n    reason: ok\n",
		"bad rule":     "version: 1\nsuppressions:\n  - rule: ps101\n    reason: a long enough reason\n",
		"bad date":     "version: 1\nsuppressions:\n  - rule: PS101\n    reason: a long enough reason\n    expires: soon\n",
		"version":      "version: 2\nsuppressions: []\n",
		"unknown key":  "version: 1\nsuppressions:\n  - rule: PS101\n    reason: a long enough reason\n    owner: me\n",
	} {
		if _, err := LoadSuppressions(writeSup(t, body)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestSuppressionScope(t *testing.T) {
	s := Suppression{Rule: "OBS005", Scenario: "report", Reason: "known slow report"}
	if !s.matches(Finding{RuleID: "OBS005", Scenario: "report"}, testNow) {
		t.Fatal("should match its scenario")
	}
	if s.matches(Finding{RuleID: "OBS005", Scenario: "checkout"}, testNow) {
		t.Fatal("must not match another scenario")
	}
}

var testNow = mustDate("2026-11-13")

func mustDate(s string) (t time.Time) {
	t, _ = time.Parse(time.DateOnly, s)
	return t
}
