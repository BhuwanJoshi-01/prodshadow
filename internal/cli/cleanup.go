package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/isolation"
)

func runCleanup(ctx context.Context, env *Env, args []string) int {
	fs := newFlags(env, "cleanup", "[--provider docker|podman] [--run RUN_ID] [--dry-run]")
	provider := fs.String("provider", "docker", "container provider")
	binary := fs.String("binary", "", "path to the container CLI")
	runID := fs.String("run", "", "only remove resources of this run")
	dryRun := fs.Bool("dry-run", false, "list resources without removing them")
	if code := parseFlags(fs, args); code >= 0 {
		return code
	}
	p, err := isolation.New(*provider, *binary)
	if err != nil {
		fmt.Fprintf(env.Stderr, "prodshadow: %v\n", err)
		return ExitInvalid
	}
	ids, err := isolation.Orphans(ctx, p, *runID)
	if errors.Is(err, isolation.ErrNoCleanup) {
		fmt.Fprintln(env.Stdout, "nothing to clean up for this provider")
		return ExitOK
	}
	if err != nil {
		fmt.Fprintf(env.Stderr, "prodshadow: %v\n", err)
		return ExitInfra
	}
	if len(ids) == 0 {
		fmt.Fprintln(env.Stdout, "no ProdShadow servers found")
		return ExitOK
	}
	for _, id := range ids {
		fmt.Fprintf(env.Stdout, "%s %s\n", map[bool]string{true: "would remove", false: "removing"}[*dryRun], id)
	}
	if *dryRun {
		return ExitOK
	}
	if err := isolation.RemoveOrphans(ctx, p, ids); err != nil {
		fmt.Fprintf(env.Stderr, "prodshadow: %v\n", err)
		return ExitInfra
	}
	return ExitOK
}
