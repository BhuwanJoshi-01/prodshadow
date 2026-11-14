package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/BhuwanJoshi-01/prodshadow/internal/report"
	"github.com/BhuwanJoshi-01/prodshadow/internal/risk"
	"github.com/BhuwanJoshi-01/prodshadow/internal/sqlinspect"
)

type ruleInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Severity string `json:"default_severity"`
	Summary  string `json:"summary"`
	DocsURL  string `json:"docs_url,omitempty"`
	Page     string `json:"page"`
	// NeedsSchema: the rule only reports anything with the fixture schema.
	NeedsSchema bool `json:"needs_schema,omitempty"`
}

func allRules() []ruleInfo {
	static := sqlinspect.Rules()
	out := make([]ruleInfo, 0, len(static)+len(risk.ObservedRules))
	for _, r := range static {
		out = append(out, ruleInfo{r.ID, r.Name, "static", string(r.Severity), r.Summary, r.DocsURL, report.RulePageURL(r.ID), r.NeedsSchema})
	}
	for _, r := range risk.ObservedRules {
		kind := "observed"
		if r.ID[:4] == "QUAL" {
			kind = "quality"
		}
		out = append(out, ruleInfo{ID: r.ID, Name: r.Name, Kind: kind, Severity: string(r.Severity), Summary: r.Summary, Page: report.RulePageURL(r.ID)})
	}
	return out
}

func runRules(_ context.Context, env *Env, args []string) int {
	fs := newFlags(env, "rules", "[--kind static|observed|quality] [--json] [RULE ...]")
	asJSON := fs.Bool("json", false, "print machine-readable JSON")
	kind := fs.String("kind", "", "only list rules of this kind: static, observed or quality")
	ids, code := parseWithArgs(fs, args)
	if code >= 0 {
		return code
	}
	switch *kind {
	case "", "static", "observed", "quality":
	default:
		fmt.Fprintf(env.Stderr, "prodshadow: invalid --kind %q\n", *kind)
		return ExitInvalid
	}
	rules, err := selectRules(allRules(), *kind, ids)
	if err != nil {
		fmt.Fprintf(env.Stderr, "prodshadow: %v\n", err)
		return ExitInvalid
	}
	if *asJSON {
		b, _ := json.MarshalIndent(rules, "", "  ")
		fmt.Fprintln(env.Stdout, string(b))
		return ExitOK
	}
	if len(ids) > 0 {
		for i, r := range rules {
			if i > 0 {
				fmt.Fprintln(env.Stdout)
			}
			printRule(env.Stdout, r)
		}
		return ExitOK
	}
	tw := tabwriter.NewWriter(env.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tKIND\tDEFAULT\tNAME\tSUMMARY")
	for _, r := range rules {
		summary := r.Summary
		if r.NeedsSchema {
			summary += " [needs schema]"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", r.ID, r.Kind, r.Severity, r.Name, summary)
	}
	if err := tw.Flush(); err != nil {
		return ExitInternal
	}
	return ExitOK
}

// selectRules filters rules by kind and, when ids are given, returns those
// rules in the order requested. IDs are matched case-insensitively and may
// also be given by rule name.
func selectRules(all []ruleInfo, kind string, ids []string) ([]ruleInfo, error) {
	var pool []ruleInfo
	for _, r := range all {
		if kind == "" || r.Kind == kind {
			pool = append(pool, r)
		}
	}
	if len(ids) == 0 {
		return pool, nil
	}
	var out []ruleInfo
	for _, id := range ids {
		found := false
		for _, r := range pool {
			if strings.EqualFold(r.ID, id) || r.Name == id {
				out = append(out, r)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown rule %q; run 'prodshadow rules' for the list", id)
		}
	}
	return out, nil
}

func printRule(w io.Writer, r ruleInfo) {
	fmt.Fprintf(w, "%s %s\n  %s\n\n  kind:     %s\n  default:  %s\n  details:  %s\n", r.ID, r.Name, r.Summary, r.Kind, r.Severity, r.Page)
	if r.NeedsSchema {
		fmt.Fprintf(w, "  needs:    the schema before the migration (inspect --schema, or fixture.schema)\n")
	}
	if r.DocsURL != "" {
		fmt.Fprintf(w, "  postgres: %s\n", r.DocsURL)
	}
}
