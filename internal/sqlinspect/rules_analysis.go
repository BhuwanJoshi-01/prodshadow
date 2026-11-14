package sqlinspect

import (
	"fmt"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"
)

func init() {
	register(Rule{
		ID:       "PS116",
		Name:     "statement-not-analysed",
		Summary:  "The bundled parser could not parse the statement; its locks were not predicted",
		Severity: evidence.Inconclusive,
		check:    checkUnparsed,
	})
	register(Rule{
		ID:       "PS117",
		Name:     "procedural-code",
		Summary:  "DO blocks and procedure calls cannot be analysed statically",
		Severity: evidence.Info,
		check:    checkProcedural,
	})
}

func checkUnparsed(c *Context, s *Statement) []Finding {
	if s.Parsed {
		return nil
	}
	f := at(s, fmt.Sprintf("The statement could not be parsed with the %s grammar (%s). It is still executed in the disposable database, but its locks were not predicted.", ParserGrammar, s.ParseError))
	f.Evidence = evidence.Quality
	f.Remediation = "If the syntax is valid for your server version, set policy.rules.PS116 to \"info\" and rely on observed results."
	return []Finding{f}
}

func checkProcedural(c *Context, s *Statement) []Finding {
	if s.Kind != KindProcedural || (s.bodyAnalysed && !s.dynamicSQL) {
		return nil
	}
	msg := fmt.Sprintf("%s runs procedural code whose locks cannot be predicted statically; only the observed experiment shows its effect.", s.Tag)
	if s.dynamicSQL {
		msg = "DO runs dynamic SQL (EXECUTE) whose locks cannot be predicted statically; the locks of its static statements are listed, but only the observed experiment shows the rest."
	}
	f := at(s, msg)
	f.Evidence = evidence.Quality
	return []Finding{f}
}
