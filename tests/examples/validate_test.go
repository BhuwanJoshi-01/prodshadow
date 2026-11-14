// Package examples checks that every example project validates and that its
// static analysis runs, without starting any database.
package examples

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/BhuwanJoshi-01/prodshadow/internal/cli"
)

func TestExamplesValidate(t *testing.T) {
	dirs, err := filepath.Glob("../../examples/*/prodshadow.yaml")
	if err != nil || len(dirs) < 5 {
		t.Fatalf("expected at least five examples, found %d (%v)", len(dirs), err)
	}
	for _, cfg := range dirs {
		dir := filepath.Dir(cfg)
		t.Run(filepath.Base(dir), func(t *testing.T) {
			for _, args := range [][]string{
				{"validate", "--config", "prodshadow.yaml"},
				{"inspect", "--config", "prodshadow.yaml", "--format", "json"},
			} {
				var out, errOut bytes.Buffer
				code := cli.Main(context.Background(), args, &cli.Env{Stdout: &out, Stderr: &errOut, Dir: dir, Getenv: os.LookupEnv})
				if code != cli.ExitOK {
					t.Fatalf("%v exited %d\n%s%s", args, code, out.String(), errOut.String())
				}
			}
			if _, err := os.Stat(filepath.Join(dir, "README.md")); err != nil {
				t.Error("every example needs a README explaining the expected result")
			}
		})
	}
}
