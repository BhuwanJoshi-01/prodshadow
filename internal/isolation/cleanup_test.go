package isolation

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRemoveOrphansRefusesUnlabelledContainers(t *testing.T) {
	p := newContainerProvider("docker", "")
	var removed []string
	p.run = func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		switch args[0] {
		case "ps":
			return []byte("aaa\nbbb\n"), nil, nil
		case "rm":
			removed = append(removed, args[len(args)-1])
		}
		return nil, nil, nil
	}
	err := RemoveOrphans(context.Background(), p, []string{"aaa", "production-db"})
	if err == nil || !strings.Contains(err.Error(), "production-db is not a ProdShadow container") {
		t.Fatalf("expected refusal, got %v", err)
	}
	if len(removed) != 1 || removed[0] != "aaa" {
		t.Fatalf("removed %v", removed)
	}
}

func TestLocalProviderHasNoOrphans(t *testing.T) {
	if _, err := Orphans(context.Background(), newLocalProvider(""), ""); !errors.Is(err, ErrNoCleanup) {
		t.Fatalf("got %v", err)
	}
}

func TestImagePresent(t *testing.T) {
	p := newContainerProvider("docker", "")
	p.run = func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if args[len(args)-1] == "postgres:17-alpine" {
			return []byte("sha256:abc"), nil, nil
		}
		return nil, []byte("No such image"), errors.New("exit status 1")
	}
	if ok, err := ImagePresent(context.Background(), p, "postgres:17-alpine"); !ok || err != nil {
		t.Fatalf("present image: %v %v", ok, err)
	}
	if ok, err := ImagePresent(context.Background(), p, "postgres:99"); ok || err != nil {
		t.Fatalf("missing image: %v %v", ok, err)
	}
}

func TestEngineResources(t *testing.T) {
	for _, tc := range []struct{ name, format string }{
		{"docker", "{{.MemTotal}} {{.NCPU}}"},
		{"podman", "{{.Host.MemTotal}} {{.Host.CPUs}}"},
	} {
		p := newContainerProvider(tc.name, "")
		p.run = func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
			if args[0] != "info" || args[len(args)-1] != tc.format {
				t.Fatalf("%s: unexpected invocation %v", tc.name, args)
			}
			return []byte("8221589504 4\n"), nil, nil
		}
		mem, cpus, err := EngineResources(context.Background(), p)
		if err != nil || mem != 8221589504 || cpus != 4 {
			t.Fatalf("%s: %d %d %v", tc.name, mem, cpus, err)
		}
	}
	p := newContainerProvider("docker", "")
	p.run = func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		return []byte("<no value>"), nil, nil
	}
	if _, _, err := EngineResources(context.Background(), p); err == nil {
		t.Fatal("unparsable output accepted")
	}
	if _, _, err := EngineResources(context.Background(), newLocalProvider("")); !errors.Is(err, ErrNoCleanup) {
		t.Fatalf("local provider: %v", err)
	}
}
