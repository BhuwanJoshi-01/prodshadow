package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/risk"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/sqlinspect"
)

type ruleInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Severity string `json:"default_severity"`
	Summary  string `json:"summary"`
	DocsURL  string `json:"docs_url,omitempty"`
}

func allRules() []ruleInfo {
	static := sqlinspect.Rules()
	out := make([]ruleInfo, 0, len(static)+len(risk.ObservedRules))
	for _, r := range static {
		out = append(out, ruleInfo{r.ID, r.Name, "static", string(r.Severity), r.Summary, r.DocsURL})
	}
	for _, r := range risk.ObservedRules {
		kind := "observed"
		if r.ID[:4] == "QUAL" {
			kind = "quality"
		}
		out = append(out, ruleInfo{r.ID, r.Name, kind, string(r.Severity), r.Summary, ""})
	}
	return out
}

func runRules(_ context.Context, env *Env, args []string) int {
	fs := newFlags(env, "rules", "[--json]")
	asJSON := fs.Bool("json", false, "print machine-readable JSON")
	if code := parseFlags(fs, args); code >= 0 {
		return code
	}
	rules := allRules()
	if *asJSON {
		b, _ := json.MarshalIndent(rules, "", "  ")
		fmt.Fprintln(env.Stdout, string(b))
		return ExitOK
	}
	tw := tabwriter.NewWriter(env.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tKIND\tDEFAULT\tNAME\tSUMMARY")
	for _, r := range rules {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", r.ID, r.Kind, r.Severity, r.Name, r.Summary)
	}
	if err := tw.Flush(); err != nil {
		return ExitInternal
	}
	return ExitOK
}
