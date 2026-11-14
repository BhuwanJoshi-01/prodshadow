package sqlinspect

import (
	"fmt"
	"strings"
	"time"

	"github.com/BhuwanJoshi-01/prodshadow/internal/evidence"
)

func init() {
	register(Rule{
		ID:       "PS148",
		Name:     "invalid-annotation",
		Summary:  "A prodshadow: comment directive is malformed and is ignored",
		Severity: evidence.Warning,
		DocsURL:  "https://github.com/BhuwanJoshi-01/prodshadow/blob/main/docs/rules.md#suppressions",
		check:    checkInvalidAnnotation,
	})
}

// checkInvalidAnnotation reports directives that were ignored, so a
// suppression with a typo or without a reason does not silently fail to
// apply.
func checkInvalidAnnotation(_ *Context, s *Statement) []Finding {
	var out []Finding
	for _, a := range s.Annotations {
		if a.Problem == "" {
			continue
		}
		f := at(s, fmt.Sprintf("The %s%s directive above this statement is ignored: %s.", AnnotationPrefix, a.Directive, a.Problem))
		f.Evidence, f.Confidence = evidence.Static, evidence.High
		f.Remediation = `Use -- prodshadow:ignore RULE[, RULE...] reason="why this is acceptable" [expires=YYYY-MM-DD].`
		out = append(out, f)
	}
	return out
}

func init() {
	register(Rule{
		ID:       "PS149",
		Name:     "expired-annotation",
		Summary:  "An inline suppression has passed its expiry date and no longer applies",
		Severity: evidence.Info,
		DocsURL:  "https://github.com/BhuwanJoshi-01/prodshadow/blob/main/docs/rules.md#suppressions",
		check:    checkExpiredAnnotation,
	})
}

// checkExpiredAnnotation reports ignore directives past their expiry, so
// the findings that reappear come with an explanation.
func checkExpiredAnnotation(c *Context, s *Statement) []Finding {
	now := c.Options.Now
	if now.IsZero() {
		now = time.Now()
	}
	var out []Finding
	for _, a := range s.Annotations {
		if a.Problem != "" || a.Directive != "ignore" || a.Expires == "" {
			continue
		}
		exp, err := time.Parse(time.DateOnly, a.Expires)
		if err != nil || now.Before(exp.AddDate(0, 0, 1)) {
			continue
		}
		f := at(s, fmt.Sprintf("The suppression of %s above this statement expired on %s (reason: %q); its findings are reported again.", strings.Join(a.Rules, ", "), a.Expires, a.Reason))
		f.Evidence, f.Confidence = evidence.Static, evidence.High
		f.Remediation = "Fix the statement, renew the suppression with a new expiry date after another review, or remove it."
		out = append(out, f)
	}
	return out
}

func init() {
	register(Rule{
		ID:            "PS150",
		Name:          "unused-annotation",
		Summary:       "An inline suppression names a static rule that reports nothing for its statement",
		Severity:      evidence.Info,
		DocsURL:       "https://github.com/BhuwanJoshi-01/prodshadow/blob/main/docs/rules.md#suppressions",
		checkFindings: unusedAnnotations,
	})
}

// unusedAnnotations reports valid, unexpired ignore directives naming a
// static (PS) rule that produced no finding, suppressed or not, for the
// statement. Observed and quality rules are not checked: they are only
// evaluated after the experiment.
func unusedAnnotations(c *Context, found []Finding) []Finding {
	now := c.Options.Now
	if now.IsZero() {
		now = time.Now()
	}
	reported := map[string]bool{}
	for _, f := range found {
		reported[f.StatementID+"|"+f.RuleID] = true
	}
	var out []Finding
	for _, s := range c.Statements {
		for _, a := range s.Annotations {
			if a.Problem != "" || a.Directive != "ignore" {
				continue
			}
			if a.Expires != "" {
				if exp, err := time.Parse(time.DateOnly, a.Expires); err == nil && !now.Before(exp.AddDate(0, 0, 1)) {
					continue // PS149 reports it
				}
			}
			var unused []string
			for _, r := range a.Rules {
				if _, known := RuleByID(r); known && strings.HasPrefix(r, "PS") && !reported[s.ID+"|"+r] {
					unused = append(unused, r)
				}
			}
			if len(unused) == 0 {
				continue
			}
			f := at(s, fmt.Sprintf("The suppression of %s above this statement has nothing to suppress: the rule reports nothing here. Remove it, so it does not hide a future finding.", strings.Join(unused, ", ")))
			f.Evidence, f.Confidence = evidence.Static, evidence.High
			f.Remediation = "Delete the rule ID from the directive, or the whole directive if nothing is left."
			out = append(out, f)
		}
	}
	return out
}
