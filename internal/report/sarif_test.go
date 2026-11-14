package report

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSARIFOutput(t *testing.T) {
	r := Build(sampleInput(t))
	raw, err := r.SARIF()
	if err != nil {
		t.Fatal(err)
	}
	var log struct {
		Version string `json:"version"`
		Runs    []struct {
			Tool struct {
				Driver struct {
					Name  string `json:"name"`
					Rules []struct {
						ID string `json:"id"`
					} `json:"rules"`
				} `json:"driver"`
			} `json:"tool"`
			Results []struct {
				RuleID    string `json:"ruleId"`
				Level     string `json:"level"`
				Locations []struct {
					Physical struct {
						Artifact struct {
							URI string `json:"uri"`
						} `json:"artifactLocation"`
						Region struct {
							StartLine int `json:"startLine"`
						} `json:"region"`
					} `json:"physicalLocation"`
				} `json:"locations"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(raw, &log); err != nil {
		t.Fatal(err)
	}
	if log.Version != "2.1.0" || log.Runs[0].Tool.Driver.Name != "ProdShadow" {
		t.Fatalf("unexpected header %+v", log)
	}
	levels := map[string]string{}
	for _, res := range log.Runs[0].Results {
		levels[res.RuleID] = res.Level
		loc := res.Locations[0].Physical
		if loc.Artifact.URI != "migrations/0042_add_note.sql" || loc.Region.StartLine < 1 {
			t.Errorf("%s has location %+v", res.RuleID, loc)
		}
	}
	if levels["OBS002"] != "error" || levels["PS114"] != "note" {
		t.Fatalf("levels = %v", levels)
	}
	rules := map[string]bool{}
	for _, r := range log.Runs[0].Tool.Driver.Rules {
		rules[r.ID] = true
	}
	for id := range levels {
		if !rules[id] {
			t.Errorf("rule %s used but not described", id)
		}
	}
	if strings.Contains(string(raw), secret) {
		t.Fatal("secret leaked into SARIF")
	}
}

func TestSARIFSkipsFindingsWithoutAnyFile(t *testing.T) {
	raw, err := SARIF(nil, "dev", "")
	if err != nil || !strings.Contains(string(raw), `"results": []`) {
		t.Fatalf("empty SARIF: %s %v", raw, err)
	}
}

func TestSARIFRulesLinkTheirPages(t *testing.T) {
	for id, r := range ruleCatalog() {
		if want := RulePageURL(id); r.HelpURI != want {
			t.Errorf("%s: helpUri %q, want %q", id, r.HelpURI, want)
		}
		if !strings.HasSuffix(r.HelpURI, "/docs/rules/"+id+".md") {
			t.Errorf("%s: helpUri %q does not name the rule page", id, r.HelpURI)
		}
	}
}
