package sqlinspect

import (
	"regexp"
	"strings"
	"time"
)

// AnnotationPrefix starts an inline directive in a SQL comment, e.g.
//
//	-- prodshadow:ignore PS101 reason="table has 200 rows" expires=2027-03-01
const AnnotationPrefix = "prodshadow:"

// Annotation is an inline directive attached to the statement that follows
// it. Suppressions live next to the SQL they excuse so that version control
// keeps the audit trail.
type Annotation struct {
	Directive string   `json:"directive"`
	Rules     []string `json:"rules,omitempty"`
	Reason    string   `json:"reason,omitempty"`
	Expires   string   `json:"expires,omitempty"`
	// Problem explains why the annotation is invalid and will be ignored.
	Problem string `json:"problem,omitempty"`
}

var (
	annotationRule = regexp.MustCompile(`^[A-Z]{2,5}[0-9]{3}$`)
	annotationKV   = regexp.MustCompile(`(\w+)=("(?:[^"\\]|\\.)*"|\S+)`)
)

// parseAnnotations extracts directives from leading comments.
func parseAnnotations(comments []string) []Annotation {
	var out []Annotation
	for _, c := range comments {
		for _, line := range strings.Split(c, "\n") {
			line = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "*"))
			if !strings.HasPrefix(line, AnnotationPrefix) {
				continue
			}
			out = append(out, parseAnnotation(strings.TrimPrefix(line, AnnotationPrefix)))
		}
	}
	return out
}

func parseAnnotation(body string) Annotation {
	directive, rest, _ := strings.Cut(strings.TrimSpace(body), " ")
	a := Annotation{Directive: directive}
	if directive != "ignore" {
		a.Problem = "unknown directive " + `"` + directive + `"`
		return a
	}

	// Rule IDs come before key=value pairs.
	for _, f := range strings.Fields(rest) {
		if strings.Contains(f, "=") {
			break
		}
		a.Rules = append(a.Rules, strings.TrimSuffix(f, ","))
	}
	for _, m := range annotationKV.FindAllStringSubmatch(rest, -1) {
		val := m[2]
		if strings.HasPrefix(val, `"`) {
			val = strings.ReplaceAll(strings.Trim(val, `"`), `\"`, `"`)
		}
		switch m[1] {
		case "reason":
			a.Reason = val
		case "expires":
			a.Expires = val
		}
	}

	switch {
	case len(a.Rules) == 0:
		a.Problem = "ignore requires at least one rule ID"
	case strings.TrimSpace(a.Reason) == "":
		a.Problem = "ignore requires a reason=\"...\""
	}
	for _, r := range a.Rules {
		if !annotationRule.MatchString(r) {
			a.Problem = "invalid rule ID " + `"` + r + `"`
		}
	}
	if a.Expires != "" {
		if _, err := time.Parse(time.DateOnly, a.Expires); err != nil {
			a.Problem = "expires must be a date in YYYY-MM-DD form"
		}
	}
	return a
}

// Suppresses reports whether a valid, unexpired annotation suppresses rule
// at time now. It returns the annotation that applied.
func (s *Statement) Suppresses(rule string, now time.Time) (Annotation, bool) {
	for _, a := range s.Annotations {
		if a.Problem != "" || a.Directive != "ignore" {
			continue
		}
		if a.Expires != "" {
			exp, _ := time.Parse(time.DateOnly, a.Expires)
			if !now.Before(exp.AddDate(0, 0, 1)) {
				continue
			}
		}
		for _, r := range a.Rules {
			if r == rule {
				return a, true
			}
		}
	}
	return Annotation{}, false
}
