package report

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestJUnit(t *testing.T) {
	r := Build(sampleInput(t))
	raw, err := r.JUnit()
	if err != nil {
		t.Fatal(err)
	}
	var doc junitSuites
	if err := xml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}
	s := doc.Suites[0]
	if s.Name != "prodshadow/orders-api" || s.Tests != len(r.Findings)+1 {
		t.Fatalf("unexpected suite %+v", s)
	}
	// The decision plus each blocking rule fails.
	if s.Failures != 1+len(r.Decision.FailedRules) {
		t.Fatalf("failures = %d, blocking rules %v", s.Failures, r.Decision.FailedRules)
	}
	failed := map[string]bool{}
	for _, c := range s.Cases {
		if c.Failure != nil {
			failed[strings.Fields(c.Name)[0]] = true
		}
	}
	if !failed["OBS002"] || failed["PS114"] {
		t.Fatalf("wrong failing cases: %v", failed)
	}
	if strings.Contains(string(raw), secret) {
		t.Fatal("secret leaked into JUnit output")
	}
}
