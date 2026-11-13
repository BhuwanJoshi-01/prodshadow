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
