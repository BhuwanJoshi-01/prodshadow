package sqlinspect

import (
	"strings"
	"testing"
)

func TestPS148InvalidAnnotation(t *testing.T) {
	stmt := "CREATE INDEX countries_name_idx ON countries (name);\n"
	cases := map[string]string{
		"-- prodshadow:ingore PS101 reason=\"small table\"\n":                    "unknown directive",
		"-- prodshadow:ignore PS101\n":                                           "requires a reason",
		"-- prodshadow:ignore reason=\"small table\"\n":                          "at least one rule ID",
		"-- prodshadow:ignore PS1O1 reason=\"small table\"\n":                    "invalid rule ID",
		"-- prodshadow:ignore PS101 reason=\"small table\" expires=31/01/2027\n": "YYYY-MM-DD",
	}
	for comment, want := range cases {
		got := findings(t, "PS148", Options{}, comment+stmt)
		if len(got) != 1 || !strings.Contains(got[0].Message, want) {
			t.Errorf("%q: want one finding containing %q, got %+v", comment, want, got)
		}
	}
	valid := "-- prodshadow:ignore PS101 reason=\"small table\" expires=2027-01-31\n" + stmt
	expectCount(t, findings(t, "PS148", Options{}, valid), 0)
	if got := findings(t, "PS101", Options{}, valid); len(got) != 1 || !got[0].Suppressed {
		t.Errorf("valid annotation should still suppress PS101: %+v", got)
	}
}

func TestPS149ExpiredAnnotation(t *testing.T) {
	stmt := "CREATE INDEX countries_name_idx ON countries (name);\n"
	// The test helper evaluates at 2026-11-13.
	expired := "-- prodshadow:ignore PS101, PS114 reason=\"small table\" expires=2026-11-12\n" + stmt
	got := findings(t, "PS149", Options{}, expired)
	if len(got) != 1 || !strings.Contains(got[0].Message, "PS101, PS114") || !strings.Contains(got[0].Message, "2026-11-12") {
		t.Fatalf("expired: %+v", got)
	}
	expectCount(t, findings(t, "PS149", Options{}, "-- prodshadow:ignore PS101 reason=\"small table\" expires=2026-11-13\n"+stmt), 0)
	expectCount(t, findings(t, "PS149", Options{}, "-- prodshadow:ignore PS101 reason=\"small table\"\n"+stmt), 0)
}

func TestPS150UnusedAnnotation(t *testing.T) {
	index := "CREATE INDEX countries_name_idx ON countries (name);\n"
	used := "-- prodshadow:ignore PS101 reason=\"small table\"\n" + index
	expectCount(t, findings(t, "PS150", Options{}, used), 0)
	unused := "-- prodshadow:ignore PS101, PS109 reason=\"small table\"\n" + index
	got := findings(t, "PS150", Options{}, unused)
	if len(got) != 1 || !strings.Contains(got[0].Message, "PS109") || strings.Contains(got[0].Message, "PS101") {
		t.Fatalf("unused: %+v", got)
	}
	// Observed rules are only evaluated later, so they are never unused here.
	expectCount(t, findings(t, "PS150", Options{}, "-- prodshadow:ignore OBS001 reason=\"accepted stall\"\n"+index), 0)
	// A suppressed finding still counts as used.
	expectCount(t, findings(t, "PS150", Options{}, "-- prodshadow:ignore PS101 reason=\"small table\"\nCREATE INDEX CONCURRENTLY countries_name_idx ON countries (name);\n"), 1)
	// The rule can be turned off like any other.
	expectCount(t, findings(t, "PS150", Options{Overrides: map[string]string{"PS150": "off"}}, unused), 0)
}
