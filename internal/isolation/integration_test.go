//go:build integration

package isolation

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/security"
)

// Integration tests need a container engine (or local PostgreSQL binaries)
// and run with: go test -tags integration ./internal/isolation/

func integrationImage() (string, string) {
	if img := os.Getenv("PRODSHADOW_IT_IMAGE"); img != "" {
		return img, os.Getenv("PRODSHADOW_IT_MAJOR")
	}
	return "postgres:17-alpine", "17"
}

func startServer(t *testing.T, provider, network string) (Provider, *Server) {
	t.Helper()
	p, err := New(provider, "")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := p.Check(ctx); err != nil {
		t.Skipf("%s provider unavailable: %v", provider, err)
	}
	image, major := integrationImage()
	spec := testSpec()
	spec.Image, spec.PostgresMajor, spec.Network = image, major, network
	spec.RunID, _ = security.RunID()
	if provider == "local" {
		spec.PostgresMajor = os.Getenv("PRODSHADOW_IT_LOCAL_MAJOR")
		if spec.PostgresMajor == "" {
			spec.PostgresMajor = "16"
		}
	}
	srv, err := p.Start(ctx, spec)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = p.Destroy(context.Background(), srv) })
	return p, srv
}

func TestDockerServerLifecycle(t *testing.T) {
	for _, network := range []string{"none", "loopback"} {
		t.Run(network, func(t *testing.T) {
			p, srv := startServer(t, "docker", network)
			ctx := context.Background()

			conn, err := srv.Connect(ctx, srv.Admin(), ConnOptions{ApplicationName: "it"})
			if err != nil {
				t.Fatal(err)
			}
			var lockWaits string
			if err := conn.QueryRow(ctx, "SHOW log_lock_waits").Scan(&lockWaits); err != nil || lockWaits != "on" {
				t.Fatalf("log_lock_waits = %q, %v", lockWaits, err)
			}
			conn.Close(ctx)

			if err := srv.Bootstrap(ctx, nil); err != nil {
				t.Fatal(err)
			}
			app, _ := srv.App()
			appConn, err := srv.Connect(ctx, app, ConnOptions{})
			if err != nil {
				t.Fatal(err)
			}
			_, err = appConn.Exec(ctx, "COPY (SELECT 1) TO PROGRAM 'id'")
			if err == nil {
				t.Fatal("application role must not run server-side programs")
			}
			appConn.Close(ctx)

			wrong := Credential{User: AdminUser, password: "wrong-password"}
			if _, err := srv.Connect(ctx, wrong, ConnOptions{}); err == nil {
				t.Fatal("wrong password accepted")
			}

			id := srv.ID()
			scratch := srv.scratch
			if err := p.Destroy(ctx, srv); err != nil {
				t.Fatal(err)
			}
			if out, _ := exec.Command("docker", "ps", "-aq", "--filter", "id="+id).Output(); strings.TrimSpace(string(out)) != "" {
				t.Fatal("container still exists after destroy")
			}
			if _, err := os.Stat(scratch); !os.IsNotExist(err) {
				t.Fatalf("scratch directory left behind: %v", err)
			}
			if _, err := srv.Connect(ctx, srv.Admin(), ConnOptions{}); !errors.Is(err, ErrNotManaged) {
				t.Fatalf("destroyed server must be unusable, got %v", err)
			}
		})
	}
}

func TestOwnershipTokenIsEnforced(t *testing.T) {
	_, srv := startServer(t, "docker", "none")
	impostor := *srv
	impostor.token = "a-different-run"
	if _, err := impostor.Connect(context.Background(), srv.Admin(), ConnOptions{}); !errors.Is(err, ErrOwnership) {
		t.Fatalf("expected ErrOwnership, got %v", err)
	}
}

func TestContainerIsNetworkIsolated(t *testing.T) {
	_, srv := startServer(t, "docker", "none")
	out, err := exec.Command("docker", "inspect", "--format", "{{.HostConfig.NetworkMode}} {{.HostConfig.ReadonlyRootfs}} {{.HostConfig.CapDrop}}", srv.ID()).Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(out)); got != "none true [ALL]" {
		t.Fatalf("unexpected isolation settings: %s", got)
	}
}

func TestLocalServerLifecycle(t *testing.T) {
	p, srv := startServer(t, "local", "none")
	ctx := context.Background()
	conn, err := srv.Connect(ctx, srv.Admin(), ConnOptions{})
	if err != nil {
		t.Fatal(err)
	}
	conn.Close(ctx)
	if err := p.Destroy(ctx, srv); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
}
