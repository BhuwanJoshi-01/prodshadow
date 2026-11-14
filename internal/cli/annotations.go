package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/BhuwanJoshi-01/prodshadow/internal/evidence"
	"github.com/BhuwanJoshi-01/prodshadow/internal/report"
	"github.com/BhuwanJoshi-01/prodshadow/internal/risk"
)

// inGitHubActions reports whether workflow commands will be interpreted.
func inGitHubActions(env *Env) bool {
	v, ok := env.Getenv("GITHUB_ACTIONS")
	return ok && v == "true"
}

// writeAnnotations prints GitHub Actions workflow commands so findings show
// up inline on the pull request. Informational findings are skipped.
func writeAnnotations(w io.Writer, findings []risk.Finding) {
	for _, f := range findings {
		if f.Suppressed {
			continue
		}
		var cmd string
		switch f.Severity {
		case evidence.Critical:
			cmd = "error"
		case evidence.Warning, evidence.Inconclusive:
			cmd = "warning"
		default:
			continue
		}
		props := []string{"title=" + escapeProperty(fmt.Sprintf("ProdShadow %s %s", f.RuleID, f.Rule))}
		if f.File != "" {
			props = append(props, "file="+escapeProperty(f.File))
			if f.Line > 0 {
				props = append(props, fmt.Sprintf("line=%d", f.Line))
			}
		}
		fmt.Fprintf(w, "::%s %s::%s\n", cmd, strings.Join(props, ","), escapeData(f.Message+" See "+report.RulePageURL(f.RuleID)))
	}
}

// escapeData and escapeProperty follow the workflow command encoding, so
// text from findings cannot inject further commands.
func escapeData(s string) string {
	return strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A").Replace(s)
}

func escapeProperty(s string) string {
	return strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A", ":", "%3A", ",", "%2C").Replace(s)
}
