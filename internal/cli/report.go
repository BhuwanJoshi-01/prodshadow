package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"

	"github.com/BhuwanJoshi-01/prodshadow/internal/report"
)

func runReport(_ context.Context, env *Env, args []string) int {
	fs := newFlags(env, "report", "--input report.json [--format markdown|html|json|sarif|junit|text] [--output FILE]")
	input := fs.String("input", "prodshadow-results/report.json", "JSON report to render")
	format := fs.String("format", "markdown", "output format: markdown, html, json, sarif, junit or text (the summary verify prints)")
	output := fs.String("output", "", "write to FILE instead of stdout")
	if code := parseFlags(fs, args); code >= 0 {
		return code
	}
	rep, err := report.ReadFile(env.abs(*input))
	if err != nil {
		fmt.Fprintf(env.Stderr, "prodshadow: %v\n", err)
		return ExitInvalid
	}
	var out []byte
	switch *format {
	case "markdown":
		out = []byte(rep.Markdown())
	case "text":
		var buf bytes.Buffer
		writeSummary(&buf, rep, nil)
		out = buf.Bytes()
	case "html":
		h, err := rep.HTML()
		if err != nil {
			fmt.Fprintf(env.Stderr, "prodshadow: %v\n", err)
			return ExitInternal
		}
		out = []byte(h)
	case "junit":
		if out, err = rep.JUnit(); err != nil {
			fmt.Fprintf(env.Stderr, "prodshadow: %v\n", err)
			return ExitInternal
		}
	case "sarif":
		if out, err = rep.SARIF(); err != nil {
			fmt.Fprintf(env.Stderr, "prodshadow: %v\n", err)
			return ExitInternal
		}
	case "json":
		if out, err = rep.JSON(); err != nil {
			fmt.Fprintf(env.Stderr, "prodshadow: %v\n", err)
			return ExitInternal
		}
	default:
		fmt.Fprintf(env.Stderr, "prodshadow: invalid --format %q\n", *format)
		return ExitInvalid
	}
	if *output == "" {
		if _, err := env.Stdout.Write(out); err != nil {
			return ExitInternal
		}
		return ExitOK
	}
	if err := os.WriteFile(env.abs(*output), out, 0o644); err != nil {
		fmt.Fprintf(env.Stderr, "prodshadow: %v\n", err)
		return ExitInfra
	}
	return ExitOK
}
