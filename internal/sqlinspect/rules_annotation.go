package sqlinspect

import (
	"fmt"
	"strings"
	"time"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"
)

func init() {
	register(Rule{
		ID:       "PS148",
		Name:     "invalid-annotation",
		Summary:  "A prodshadow: comment directive is malformed and is ignored",
		Severity: evidence.Warning,
		DocsURL:  "https://github.com/BhuwanJoshi-01/Open-Source-1/blob/main/docs/rules.md#suppressions",
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
		DocsURL:  "https://github.com/BhuwanJoshi-01/Open-Source-1/blob/main/docs/rules.md#suppressions",
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
