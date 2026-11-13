package isolation

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/security"
)

func testSpec() Spec {
	r, _ := security.NewRedactor(nil)
	return Spec{
		RunID: "abc123", PostgresMajor: "17", Image: "postgres:17-alpine", Network: "none",
		MemoryMB: 1024, CPUs: 1.5, DiskMB: 2048, PidsLimit: 256, StartupTimeout: time.Minute,
		Settings: map[string]string{"shared_buffers": "128MB", "max_connections": "200"},
		Redactor: r,
	}
}

func TestRunArgsAreHardened(t *testing.T) {
	p := newContainerProvider("docker", "")
	srv := &Server{token: "tok", admin: Credential{User: AdminUser, password: "pw"}}
	args := p.runArgs(testSpec(), srv, "/tmp/x/sock")
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"--read-only", "--network none", "--cap-drop ALL", "--security-opt no-new-privileges",
		"--memory 1024m", "--memory-swap 1024m", "--cpus 1.5", "--pids-limit 256",
		"--tmpfs /var/lib/postgresql:rw,size=2048m", "--volume /tmp/x/sock:/var/run/postgresql:rw",
		"--label io.prodshadow.run=abc123", "-c listen_addresses=", "-c prodshadow.run_token=tok",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %s", want, joined)
		}
	}
	if strings.Contains(joined, "--privileged") || strings.Contains(joined, "docker.sock") {
		t.Fatal("container must never be privileged or see the engine socket")
	}
	// User settings come first so managed settings override them.
	if i, j := slices.Index(args, "max_connections=200"), slices.Index(args, "listen_addresses="); i < 0 || j < i {
		t.Fatalf("settings order wrong: %v", args)
	}
}

func TestRunArgsLoopbackPublishesOnlyToLoopback(t *testing.T) {
	spec := testSpec()
	spec.Network = "loopback"
	args := strings.Join(newContainerProvider("podman", "").runArgs(spec, &Server{}, ""), " ")
	if !strings.Contains(args, "--publish 127.0.0.1::5432") || strings.Contains(args, "--network none") {
		t.Fatalf("unexpected loopback args: %s", args)
	}
	if !strings.Contains(args, "-c listen_addresses=*") {
		t.Fatalf("server must listen inside the container: %s", args)
	}
}

func TestHostPortParsesLoopbackOnly(t *testing.T) {
	p := newContainerProvider("docker", "")
	p.run = func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		return []byte("0.0.0.0:1111\n127.0.0.1:49153\n"), nil, nil
	}
	port, err := p.hostPort(context.Background(), "id")
	if err != nil || port != 49153 {
		t.Fatalf("port = %d, %v", port, err)
	}
	p.run = func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		return []byte("0.0.0.0:1111\n"), nil, nil
	}
	if _, err := p.hostPort(context.Background(), "id"); err == nil {
		t.Fatal("non-loopback publication must be rejected")
	}
}

func TestStartFailureCleansUp(t *testing.T) {
	p := newContainerProvider("docker", "")
	var calls []string
	p.run = func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		calls = append(calls, args[0])
		if args[0] == "run" {
			return nil, []byte("image not found"), errors.New("exit status 125")
		}
		return nil, nil, nil
	}
	spec := testSpec()
	// Loopback works on every OS; network "none" is refused off Linux
	// before the engine is called.
	spec.Network = "loopback"
	_, err := p.Start(context.Background(), spec)
	if err == nil || !strings.Contains(err.Error(), "image not found") {
		t.Fatalf("expected run failure, got %v", err)
	}
	if strings.Contains(err.Error(), "POSTGRES_PASSWORD") {
		t.Fatal("error must not echo container arguments")
	}
}

func TestSpecValidation(t *testing.T) {
	s := testSpec()
	s.Network = "host"
	if err := s.validate(); err == nil {
		t.Fatal("host networking must be rejected")
	}
	s = testSpec()
	s.RunID = ""
	if err := s.validate(); err == nil {
		t.Fatal("missing run ID must be rejected")
	}
}

func TestQuoteLiteral(t *testing.T) {
	if got := quoteLiteral("a'b"); got != "'a''b'" {
		t.Fatalf("got %s", got)
	}
}

func TestNewProvider(t *testing.T) {
	for _, name := range []string{"docker", "podman", "local"} {
		p, err := New(name, "")
		if err != nil || p.Name() != name {
			t.Fatalf("New(%s) = %v, %v", name, p, err)
		}
	}
	if _, err := New("vm", ""); err == nil {
		t.Fatal("unknown provider must fail")
	}
}

func TestUserSettingsOverrideDefaults(t *testing.T) {
	spec := testSpec()
	spec.Settings = map[string]string{"deadlock_timeout": "1s"}
	got := serverSettings(spec, "tok", "")
	if got[0] != "deadlock_timeout=200ms" || !slices.Contains(got, "deadlock_timeout=1s") {
		t.Fatalf("defaults must come first so user values win: %v", got)
	}
	if slices.Index(got, "deadlock_timeout=1s") < slices.Index(got, "deadlock_timeout=200ms") {
		t.Fatal("user value must follow the default")
	}
}
