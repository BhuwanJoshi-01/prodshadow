package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/BhuwanJoshi-01/prodshadow/internal/report"
)

func runDiff(_ context.Context, env *Env, args []string) int {
	fs := newFlags(env, "diff", "--base report.json --head report.json [--json]")
	base := fs.String("base", "", "report of the original migration")
	head := fs.String("head", "", "report of the alternative")
	asJSON := fs.Bool("json", false, "print machine-readable JSON")
	if code := parseFlags(fs, args); code >= 0 {
		return code
	}
	if *base == "" || *head == "" {
		fmt.Fprintln(env.Stderr, "prodshadow: --base and --head are required")
		return ExitInvalid
	}
	b, err := report.ReadFile(env.abs(*base))
	if err != nil {
		fmt.Fprintf(env.Stderr, "prodshadow: %v\n", err)
		return ExitInvalid
	}
	h, err := report.ReadFile(env.abs(*head))
	if err != nil {
		fmt.Fprintf(env.Stderr, "prodshadow: %v\n", err)
		return ExitInvalid
	}
	d := report.Compare(b, h)
	if *asJSON {
		out, _ := json.MarshalIndent(d, "", "  ")
		fmt.Fprintln(env.Stdout, string(out))
		return ExitOK
	}
	fmt.Fprint(env.Stdout, d.Markdown())
	return ExitOK
}
