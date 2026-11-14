package isolation

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrNoCleanup is returned for providers that keep no resources between
// processes (the local provider removes its directory on exit).
var ErrNoCleanup = errors.New("provider has no persistent resources to clean up")

// Orphans lists container IDs labelled as ProdShadow resources. An empty
// runID lists resources of every run.
func Orphans(ctx context.Context, p Provider, runID string) ([]string, error) {
	cp, ok := p.(*containerProvider)
	if !ok {
		return nil, ErrNoCleanup
	}
	return cp.ListOrphans(ctx, runID)
}

// RemoveOrphans force-removes the given labelled containers. IDs that do
// not carry the ProdShadow label are refused, so this can never remove
// unrelated containers.
func RemoveOrphans(ctx context.Context, p Provider, ids []string) error {
	cp, ok := p.(*containerProvider)
	if !ok {
		return ErrNoCleanup
	}
	labelled, err := cp.ListOrphans(ctx, "")
	if err != nil {
		return err
	}
	allowed := map[string]bool{}
	for _, id := range labelled {
		allowed[id] = true
	}
	var errs []error
	for _, id := range ids {
		if !allowed[id] {
			errs = append(errs, fmt.Errorf("%s is not a ProdShadow container; refusing to remove it", id))
			continue
		}
		if _, stderr, err := cp.run(ctx, cp.binary, "rm", "--force", "--volumes", id); err != nil {
			errs = append(errs, commandError(cp.binary, "rm", stderr, err))
		}
	}
	return errors.Join(errs...)
}

// ImagePresent reports whether a container image is available locally. A
// missing image is pulled on first use, which needs registry access.
func ImagePresent(ctx context.Context, p Provider, image string) (bool, error) {
	cp, ok := p.(*containerProvider)
	if !ok {
		return false, ErrNoCleanup
	}
	_, stderr, err := cp.run(ctx, cp.binary, "image", "inspect", "--format", "{{.Id}}", image)
	if err == nil {
		return true, nil
	}
	if len(stderr) > 0 {
		return false, nil
	}
	return false, commandError(cp.binary, "image inspect", stderr, err)
}

// EngineResources reports the memory (in bytes) and CPUs the container
// engine can give containers. On Docker Desktop and Podman machines this is
// the virtual machine's size, not the host's.
func EngineResources(ctx context.Context, p Provider) (memBytes int64, cpus int, err error) {
	cp, ok := p.(*containerProvider)
	if !ok {
		return 0, 0, ErrNoCleanup
	}
	format := "{{.MemTotal}} {{.NCPU}}"
	if cp.name == "podman" {
		format = "{{.Host.MemTotal}} {{.Host.CPUs}}"
	}
	out, stderr, err := cp.run(ctx, cp.binary, "info", "--format", format)
	if err != nil {
		return 0, 0, commandError(cp.binary, "info", stderr, err)
	}
	if _, err := fmt.Sscan(string(out), &memBytes, &cpus); err != nil {
		return 0, 0, fmt.Errorf("%s info: unexpected output %q", cp.binary, strings.TrimSpace(string(out)))
	}
	return memBytes, cpus, nil
}
