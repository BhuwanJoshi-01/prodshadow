// Package cli implements the prodshadow command line interface.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/risk"
)

// Exit codes. They are part of the public contract; see docs/cli.md.
const (
	ExitOK           = risk.ExitPass
	ExitInternal     = risk.ExitInternal
	ExitBlock        = risk.ExitBlock
	ExitInvalid      = risk.ExitInvalid
	ExitInfra        = risk.ExitInfra
	ExitInconclusive = risk.ExitInconclusive
)

// Env abstracts the process environment for testing.
type Env struct {
	Stdout io.Writer
	Stderr io.Writer
	Getenv func(string) (string, bool)
	Dir    string
}

type command struct {
	name    string
	summary string
	run     func(ctx context.Context, env *Env, args []string) int
}

func commands() []command {
	return []command{
		{"init", "Create a starter prodshadow.yaml, fixture, workload and migration", runInit},
		{"validate", "Validate the configuration and every file it references", runValidate},
		{"inspect", "Statically analyse migration files (no database needed)", runInspect},
		{"plan", "Show what verify would run, without starting anything", runPlan},
		{"verify", "Run the paired baseline/candidate experiment in a disposable database", runVerify},
		{"report", "Re-render a JSON report as Markdown, HTML or JSON", runReport},
		{"doctor", "Check that this machine can run experiments", runDoctor},
		{"cleanup", "Remove disposable servers left behind by interrupted runs", runCleanup},
		{"rules", "List static and observed rules", runRules},
		{"schema", "Print a JSON Schema (config, workload or report)", runSchema},
		{"version", "Print version information", runVersion},
	}
}

// Main runs the CLI and returns the process exit code.
func Main(ctx context.Context, args []string, env *Env) int {
	if env.Getenv == nil {
		env.Getenv = os.LookupEnv
	}
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		usage(env.Stdout)
		if len(args) == 0 {
			return ExitInvalid
		}
		return ExitOK
	}
	if args[0] == "--version" {
		return runVersion(ctx, env, nil)
	}
	for _, c := range commands() {
		if c.name == args[0] {
			return c.run(ctx, env, args[1:])
		}
	}
	fmt.Fprintf(env.Stderr, "prodshadow: unknown command %q\n\n", args[0])
	usage(env.Stderr)
	return ExitInvalid
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "ProdShadow rehearses PostgreSQL migrations under concurrent load in disposable databases.")
	fmt.Fprintln(w, "\nUsage:\n  prodshadow <command> [flags]\n\nCommands:")
	for _, c := range commands() {
		fmt.Fprintf(w, "  %-10s %s\n", c.name, c.summary)
	}
	fmt.Fprintln(w, "\nRun 'prodshadow <command> -h' for command flags.")
	fmt.Fprintln(w, "\nExit codes: 0 pass, 2 blocking findings, 3 invalid input, 4 infrastructure failure, 5 inconclusive.")
}

// newFlags creates a flag set that writes usage to stderr and never exits.
func newFlags(env *Env, name, synopsis string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	fs.Usage = func() {
		fmt.Fprintf(env.Stderr, "Usage: prodshadow %s %s\n\nFlags:\n", name, synopsis)
		fs.PrintDefaults()
	}
	return fs
}

// parseFlags parses args and maps errors to an exit code. It returns -1 to
// mean "continue".
func parseFlags(fs *flag.FlagSet, args []string) int {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return ExitOK
		}
		return ExitInvalid
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(fs.Output(), "unexpected arguments: %s\n", strings.Join(fs.Args(), " "))
		return ExitInvalid
	}
	return -1
}

// multiFlag collects repeated string flags.
type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }

func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}
