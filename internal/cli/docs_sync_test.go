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
		row := "| `" + r.ID + "` | " + r.Name + " | " + r.Severity + " |"
		if !strings.Contains(string(doc), row) {
			t.Errorf("docs/rules.md lacks a row starting with %q", row)
		}
	}
}
