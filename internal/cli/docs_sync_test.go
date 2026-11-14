package cli

import (
	"os"
	"strings"
	"testing"
)

// TestEveryRuleIsDocumented keeps docs/rules.md in sync with the rule
// registries.
func TestEveryRuleIsDocumented(t *testing.T) {
	doc, err := os.ReadFile("../../docs/rules.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range allRules() {
		row := "| [`" + r.ID + "`](rules/" + r.ID + ".md) | " + r.Name + " | " + r.Severity + " |"
		if !strings.Contains(string(doc), row) {
			t.Errorf("docs/rules.md lacks a row starting with %q", row)
		}
		page, err := os.ReadFile("../../docs/rules/" + r.ID + ".md")
		if err != nil {
			t.Errorf("rule %s has no page: %v", r.ID, err)
			continue
		}
		if !strings.HasPrefix(string(page), "# "+r.ID+" · "+r.Name+"\n") {
			t.Errorf("docs/rules/%s.md must start with \"# %s · %s\"", r.ID, r.ID, r.Name)
		}
	}
}
