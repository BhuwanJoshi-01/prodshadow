package evidence

import "testing"

func TestSeverityRankOrder(t *testing.T) {
	if !(Info.Rank() < Warning.Rank() && Warning.Rank() < Critical.Rank() && Critical.Rank() < Inconclusive.Rank()) {
		t.Fatal("severity ranks are out of order")
	}
	if Severity("bogus").Valid() {
		t.Fatal("unknown severity reported as valid")
	}
}

func TestParseSeverity(t *testing.T) {
	if s, err := ParseSeverity("warning"); err != nil || s != Warning {
		t.Fatalf("got %v %v", s, err)
	}
	if _, err := ParseSeverity("fatal"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestConfidenceLower(t *testing.T) {
	if High.Lower(Low) != Low || Low.Lower(High) != Low || Medium.Lower(High) != Medium {
		t.Fatal("Lower picked the wrong confidence")
	}
}
