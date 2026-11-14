package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/BhuwanJoshi-01/prodshadow/internal/report"
	"github.com/BhuwanJoshi-01/prodshadow/internal/sqlinspect"
	"github.com/BhuwanJoshi-01/prodshadow/internal/version"
)

func runVersion(_ context.Context, env *Env, args []string) int {
	fs := newFlags(env, "version", "[--json | --short]")
	asJSON := fs.Bool("json", false, "print machine-readable JSON")
	short := fs.Bool("short", false, "print only the version number")
	if code := parseFlags(fs, args); code >= 0 {
		return code
	}
	info := version.Get()
	if *short {
		fmt.Fprintln(env.Stdout, info.Version)
		return ExitOK
	}
	if *asJSON {
		out := struct {
			version.Info
			ReportSchema string `json:"report_schema"`
			Parser       string `json:"parser_grammar"`
		}{info, report.SchemaVersion, sqlinspect.ParserGrammar}
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Fprintln(env.Stdout, string(b))
		return ExitOK
	}
	fmt.Fprintln(env.Stdout, info.String())
	fmt.Fprintf(env.Stdout, "report schema %s, parser %s\n", report.SchemaVersion, sqlinspect.ParserGrammar)
	return ExitOK
}
