package sqlinspect

import (
	"strings"
	"testing"
	"time"
)

func TestParseAnnotations(t *testing.T) {
	anns := parseAnnotations([]string{
		"plain comment",
		`prodshadow:ignore PS101 PS102 reason="tiny table; \"lookup\" data" expires=2027-01-31`,
		"prodshadow:ignore PS103",
		"prodshadow:skip PS101 reason=x",
		"prodshadow:ignore bad reason=x",
		"prodshadow:ignore PS104 reason=ok expires=tomorrow",
	})
	if len(anns) != 5 {
		t.Fatalf("expected 5 annotations, got %d", len(anns))
	}
	a := anns[0]
	if a.Problem != "" || len(a.Rules) != 2 || a.Reason != `tiny table; "lookup" data` || a.Expires != "2027-01-31" {
		t.Fatalf("unexpected first annotation %+v", a)
	}
	for i, want := range []string{"reason", "unknown directive", "invalid rule", "YYYY-MM-DD"} {
		if got := anns[i+1].Problem; got == "" || !strings.Contains(got, want) {
			t.Errorf("annotation %d problem = %q, want mention of %q", i+1, got, want)
		}
	}
}

func TestParseAnnotationsFromBlockComment(t *testing.T) {
	anns := parseAnnotations([]string{"note\n * prodshadow:ignore PS110 reason=\"column unused since v42\""})
	if len(anns) != 1 || anns[0].Problem != "" || anns[0].Rules[0] != "PS110" {
		t.Fatalf("got %+v", anns)
	}
}

func TestSuppressesHonoursExpiry(t *testing.T) {
	s := &Statement{Annotations: parseAnnotations([]string{`prodshadow:ignore PS101 reason=ok expires=2026-12-31`})}
	day := func(d string) time.Time { tm, _ := time.Parse(time.DateOnly, d); return tm.Add(12 * time.Hour) }
	if _, ok := s.Suppresses("PS101", day("2026-12-31")); !ok {
		t.Fatal("annotation should apply on its expiry date")
	}
	if _, ok := s.Suppresses("PS101", day("2027-01-01")); ok {
		t.Fatal("annotation should not apply after expiry")
	}
	if _, ok := s.Suppresses("PS102", day("2026-01-01")); ok {
		t.Fatal("annotation should only apply to listed rules")
	}
}

func TestInvalidAnnotationNeverSuppresses(t *testing.T) {
	s := &Statement{Annotations: parseAnnotations([]string{"prodshadow:ignore PS101"})}
	if _, ok := s.Suppresses("PS101", time.Now()); ok {
		t.Fatal("annotation without reason must not suppress")
	}
}
