//go:build integration

// Package integration runs the example projects end to end against real
// PostgreSQL containers and checks the decisions their READMEs document.
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/cli"
)

type expectation struct {
	example   string
	migration string // optional override, relative to the example
	exit      int
	mustHave  []string // rule IDs that must be reported (unsuppressed)
	mustNot   []string // rule IDs that must not be reported
}

var expectations = []expectation{
	{example: "safe-add-column", exit: cli.ExitOK, mustNot: []string{"OBS001", "OBS002", "OBS005", "OBS007"}},
	{example: "long-running-transaction", exit: cli.ExitBlock, mustHave: []string{"OBS002", "OBS011"}},
	{example: "blocking-index", exit: cli.ExitBlock, mustHave: []string{"OBS001", "PS101"}},
	{example: "blocking-index", migration: "alternatives/0012_index_orders_by_status_concurrently.sql", mustHave: []string{"PS121"}, mustNot: []string{"OBS001", "PS101"}, exit: -1},
	{example: "backfill-heavy", exit: cli.ExitBlock, mustHave: []string{"OBS001", "PS111"}},
	{example: "rolling-deploy", exit: cli.ExitBlock, mustHave: []string{"OBS007", "OBS013", "PS109"}},
	{example: "partition-maintenance", exit: cli.ExitBlock, mustHave: []string{"OBS002", "PS141"}},
	{example: "partition-maintenance", migration: "alternatives/0031_events_2027.sql", mustNot: []string{"OBS002", "PS141", "PS123"}, exit: cli.ExitOK},
	{example: "matview-refresh", exit: cli.ExitBlock, mustHave: []string{"OBS001", "OBS011", "PS113"}},
	// The concurrent refresh blocks no reader, but competes for CPU; an
	// occasional contention stall can still exceed the budget, so only the
	// absence of lock queueing is asserted.
	{example: "matview-refresh", migration: "alternatives/0044_refresh_daily_revenue.sql", mustNot: []string{"OBS011", "PS113"}, exit: -1},
}

type reportFile struct {
	Status   string `json:"status"`
	Findings []struct {
		RuleID     string `json:"rule_id"`
		Suppressed bool   `json:"suppressed"`
	} `json:"findings"`
}

func TestExamples(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range expectations {
		name := e.example
		if e.migration != "" {
			name += "/" + filepath.Base(e.migration)
		}
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(root, "examples", e.example)
			out := filepath.Join(root, "tests", "integration", "testdata", "out", filepath.FromSlash(name))
			args := []string{"verify", "--quiet", "--config", "prodshadow.yaml", "--output", out, "--format", "json,markdown"}
			if e.migration != "" {
				args = append(args, "--migration", e.migration)
			}
			var stdout, stderr bytes.Buffer
			code := cli.Main(context.Background(), args, &cli.Env{Stdout: &stdout, Stderr: &stderr, Dir: dir, Getenv: os.LookupEnv})
			t.Log(stdout.String())
			if e.exit >= 0 && code != e.exit {
				t.Fatalf("exit code %d, want %d\nstderr: %s", code, e.exit, stderr.String())
			}
			if code == cli.ExitInfra || code == cli.ExitInvalid {
				t.Fatalf("run failed with exit %d: %s", code, stderr.String())
			}
			raw, err := os.ReadFile(filepath.Join(out, "report.json"))
			if err != nil {
				t.Fatal(err)
			}
			var rep reportFile
			if err := json.Unmarshal(raw, &rep); err != nil {
				t.Fatal(err)
			}
			got := map[string]bool{}
			for _, f := range rep.Findings {
				if !f.Suppressed {
					got[f.RuleID] = true
				}
			}
			for _, id := range e.mustHave {
				if !got[id] {
					t.Errorf("expected finding %s; got %v", id, keys(got))
				}
			}
			for _, id := range e.mustNot {
				if got[id] {
					t.Errorf("unexpected finding %s", id)
				}
			}
		})
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
