package cli

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/isolation"
)

type check struct {
	name   string
	ok     bool
	warn   bool
	detail string
}

func runDoctor(ctx context.Context, env *Env, args []string) int {
	fs := newFlags(env, "doctor", "[--config prodshadow.yaml]")
	cfgPath := fs.String("config", "", "configuration to check (default: prodshadow.yaml if present)")
	if code := parseFlags(fs, args); code >= 0 {
		return code
	}
	var checks []check
	add := func(c check) { checks = append(checks, c) }

	add(check{name: "platform", ok: true, detail: fmt.Sprintf("%s/%s, %d CPUs", runtime.GOOS, runtime.GOARCH, runtime.NumCPU())})

	path := *cfgPath
	if path == "" {
		if _, err := os.Stat(env.abs(DefaultConfig)); err == nil {
			path = DefaultConfig
		}
	}
	provider, binary, network, image := "docker", "", "none", ""
	memoryMB, cpus := 0, 0.0
	if path != "" {
		p, err := loadProject(env, path)
		switch {
		case err != nil:
			add(check{name: "configuration", detail: err.Error()})
		case p.problems.HasErrors():
			add(check{name: "configuration", detail: fmt.Sprintf("%d error(s); run 'prodshadow validate'", len(p.problems.Errors()))})
		default:
			add(check{name: "configuration", ok: true, detail: display(env.Dir, p.cfg.Path())})
			provider, binary, network = p.cfg.Postgres.Isolation.Provider, p.cfg.Postgres.Isolation.Binary, p.cfg.Postgres.Isolation.Network
			image = p.cfg.Postgres.Image
			memoryMB, cpus = p.cfg.Postgres.Isolation.MemoryMB, p.cfg.Postgres.Isolation.CPUs
		}
	} else {
		add(check{name: "configuration", ok: true, warn: true, detail: "no prodshadow.yaml found; checking the docker provider"})
	}

	prov, err := isolation.New(provider, binary)
	if err != nil {
		add(check{name: "provider", detail: err.Error()})
	} else {
		cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err := prov.Check(cctx)
		cancel()
		if err != nil {
			add(check{name: provider, detail: err.Error()})
		} else {
			add(check{name: provider, ok: true, detail: "available"})
		}
		if image != "" && err == nil {
			if ok, ierr := isolation.ImagePresent(ctx, prov, image); ierr == nil && ok {
				add(check{name: "image", ok: true, detail: image + " is available locally"})
			} else if ierr == nil {
				add(check{name: "image", ok: true, warn: true, detail: image + " will be pulled on first run (needs registry access)"})
			}
		}
		if memoryMB > 0 && err == nil {
			if c, ok := resourceCheck(ctx, prov, memoryMB, cpus); ok {
				add(c)
			}
		}
		if orphans, err := isolation.Orphans(ctx, prov, ""); err == nil && len(orphans) > 0 {
			add(check{name: "orphaned servers", ok: true, warn: true, detail: fmt.Sprintf("%d left behind by interrupted runs; run 'prodshadow cleanup'", len(orphans))})
		}
	}
	if provider != "local" && network == "none" && runtime.GOOS != "linux" {
		add(check{name: "network", detail: "network: none shares a unix socket with the container and only works on Linux; use network: loopback"})
	}
	if provider == "local" {
		add(check{name: "resource limits", ok: true, warn: true, detail: "the local provider cannot cap CPU, memory or disk"})
	}

	failed := false
	for _, c := range checks {
		status := "ok  "
		switch {
		case !c.ok:
			status, failed = "FAIL", true
		case c.warn:
			status = "warn"
		}
		fmt.Fprintf(env.Stdout, "[%s] %-18s %s\n", status, c.name, c.detail)
	}
	if failed {
		return ExitInfra
	}
	return ExitOK
}

// resourceCheck compares the configured server size with what the
// container engine can provide. Docker Desktop and Podman machines are
// virtual machines that are often smaller than the host.
func resourceCheck(ctx context.Context, prov isolation.Provider, memoryMB int, cpus float64) (check, bool) {
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	mem, ncpu, err := isolation.EngineResources(cctx, prov)
	if err != nil {
		return check{}, false
	}
	return compareResources(int(mem>>20), ncpu, memoryMB, cpus), true
}

func compareResources(engineMB, ncpu, memoryMB int, cpus float64) check {
	detail := fmt.Sprintf("engine has %d MiB and %d CPUs; the server is configured for %d MiB and %g CPUs", engineMB, ncpu, memoryMB, cpus)
	switch {
	case memoryMB > engineMB:
		return check{name: "resources", detail: detail + "; lower postgres.isolation.memory_mb or give the engine more memory"}
	case float64(ncpu) < cpus:
		return check{name: "resources", ok: true, warn: true, detail: detail + "; the CPU limit cannot be met, so latencies will be noisier"}
	case memoryMB*2 > engineMB:
		return check{name: "resources", ok: true, warn: true, detail: detail + "; little memory is left for the workload and ProdShadow itself"}
	default:
		return check{name: "resources", ok: true, detail: detail}
	}
}
