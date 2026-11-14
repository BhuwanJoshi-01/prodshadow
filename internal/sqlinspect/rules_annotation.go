package sqlinspect

import (
	"fmt"

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
