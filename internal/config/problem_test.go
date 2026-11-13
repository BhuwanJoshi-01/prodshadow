package config

import (
	"strings"
	"testing"
)

func TestProblemsHasErrors(t *testing.T) {
	var ps Problems
	if ps.HasErrors() {
		t.Fatal("empty problems should not have errors")
	}
	ps.warnf("a", "warn")
	if ps.HasErrors() {
		t.Fatal("warnings alone should not count as errors")
	}
	ps.errorf("b", "bad %d", 1)
	if !ps.HasErrors() {
		t.Fatal("expected errors")
	}
	if len(ps.Errors()) != 1 || len(ps.Warnings()) != 1 {
		t.Fatalf("unexpected split: %v", ps)
	}
}

func TestProblemsSortedErrorsFirst(t *testing.T) {
	ps := Problems{
		{Level: LevelWarning, Path: "a"},
		{Level: LevelError, Path: "z"},
		{Level: LevelError, Path: "b"},
	}
	got := ps.Sorted()
	if got[0].Path != "b" || got[1].Path != "z" || got[2].Path != "a" {
		t.Fatalf("unexpected order: %v", got)
	}
}

func TestProblemStringIncludesLine(t *testing.T) {
	p := Problem{Level: LevelError, Path: "postgres.version", Message: "unsupported", Line: 7}
	if got := p.String(); !strings.Contains(got, "line 7") {
		t.Fatalf("missing line in %q", got)
	}
}

func TestStrictErrorsPromotesWarnings(t *testing.T) {
	ps := Problems{{Level: LevelWarning, Path: "x"}}
	if !ps.StrictErrors().HasErrors() {
		t.Fatal("strict mode should promote warnings")
	}
	if ps.HasErrors() {
		t.Fatal("StrictErrors must not mutate the receiver")
	}
}
