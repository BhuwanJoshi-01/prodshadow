package isolation

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/security"
)

// containerSocketDir is where the official image's server creates its socket.
const containerSocketDir = "/var/run/postgresql"

// containerProvider runs PostgreSQL in Docker or Podman.
type containerProvider struct {
	name   string
	binary string
	run    runner
}

func newContainerProvider(name, binary string) *containerProvider {
	if binary == "" {
		binary = name
	}
	return &containerProvider{name: name, binary: binary, run: execRunner}
}

func (p *containerProvider) Name() string { return p.name }

func (p *containerProvider) Check(ctx context.Context) error {
	_, stderr, err := p.run(ctx, p.binary, "version", "--format", "{{.Server.Version}}")
	if err != nil {
		return commandError(p.binary, "version", stderr, err)
	}
	return nil
}

func (p *containerProvider) Start(ctx context.Context, spec Spec) (*Server, error) {
	if err := spec.validate(); err != nil {
		return nil, err
	}
	if spec.Network == "none" && runtime.GOOS != "linux" {
		return nil, errors.New("isolation: network \"none\" shares a unix socket with the container and is only supported on Linux; use network: loopback")
	}
	password, err := security.RandomToken(24)
	if err != nil {
		return nil, err
	}
	token, err := security.RandomToken(16)
	if err != nil {
		return nil, err
	}
	spec.Redactor.Add(password, token)

	srv := &Server{
		provider: p.name,
		major:    spec.PostgresMajor,
		token:    token,
		admin:    Credential{User: AdminUser, password: password},
		port:     5432,
		managed:  true,
	}

	// The private parent directory (0700) keeps other host users away from
	// the socket even though the socket directory itself must be writable
	// by the container's postgres user.
	scratch, err := os.MkdirTemp("", "prodshadow-"+spec.RunID+"-")
	if err != nil {
		return nil, fmt.Errorf("create scratch directory: %w", err)
	}
	srv.scratch = scratch
	sockDir := filepath.Join(scratch, "sock")
	if spec.Network == "none" {
		if err := os.Mkdir(sockDir, 0o700); err != nil {
			return nil, errors.Join(err, p.cleanupScratch(srv))
		}
		// The container's postgres user must create its socket here; the
		// 0700 parent directory still keeps other host users out.
		if err := os.Chmod(sockDir, 0o777); err != nil { //nolint:gosec // see comment above
			return nil, errors.Join(err, p.cleanupScratch(srv))
		}
		srv.host = sockDir
	}

	args := p.runArgs(spec, srv, sockDir)
	out, stderr, err := p.run(ctx, p.binary, args...)
	if err != nil {
		return nil, errors.Join(commandError(p.binary, "run", stderr, err), p.cleanupScratch(srv))
	}
	srv.id = strings.TrimSpace(string(out))
	if srv.id == "" {
		return nil, errors.Join(fmt.Errorf("%s run returned no container ID", p.binary), p.cleanupScratch(srv))
	}

	if spec.Network == "loopback" {
		port, err := p.hostPort(ctx, srv.id)
		if err != nil {
			return nil, p.failStart(srv, err)
		}
		srv.host, srv.port = "127.0.0.1", port
	}

	waitCtx, cancel := context.WithTimeout(ctx, spec.StartupTimeout)
	defer cancel()
	if err := p.waitReady(waitCtx, srv); err != nil {
		return nil, p.failStart(srv, err)
	}
	srv.started = time.Now()
	return srv, nil
}

// runArgs builds the hardened container invocation.
func (p *containerProvider) runArgs(spec Spec, srv *Server, sockDir string) []string {
	args := []string{
		"run", "--detach",
		"--name", "prodshadow-" + spec.RunID,
		"--label", LabelManaged + "=true",
		"--label", LabelRun + "=" + spec.RunID,
		"--read-only",
		"--tmpfs", fmt.Sprintf("/var/lib/postgresql:rw,size=%dm", spec.DiskMB),
		"--tmpfs", "/tmp:rw,size=64m",
		"--memory", fmt.Sprintf("%dm", spec.MemoryMB),
		"--memory-swap", fmt.Sprintf("%dm", spec.MemoryMB),
		"--cpus", strconv.FormatFloat(spec.CPUs, 'f', -1, 64),
		"--pids-limit", strconv.Itoa(spec.PidsLimit),
		"--shm-size", "256m",
		"--security-opt", "no-new-privileges",
		"--cap-drop", "ALL",
		// The image entrypoint starts as root to prepare the data
		// directory, then drops to the postgres user.
		"--cap-add", "CHOWN", "--cap-add", "SETUID", "--cap-add", "SETGID",
		"--cap-add", "DAC_OVERRIDE", "--cap-add", "FOWNER",
		"--env", "POSTGRES_USER=" + AdminUser,
		"--env", "POSTGRES_PASSWORD=" + srv.admin.password,
		"--env", "PGDATA=/var/lib/postgresql/data",
		"--env", "POSTGRES_INITDB_ARGS=--auth-local=scram-sha-256 --auth-host=scram-sha-256",
	}
	listen := "*"
	switch spec.Network {
	case "none":
		args = append(args, "--network", "none", "--volume", sockDir+":"+containerSocketDir+":rw")
		listen = ""
	case "loopback":
		args = append(args, "--publish", "127.0.0.1::5432", "--tmpfs", containerSocketDir+":rw,size=1m,mode=1777")
	}
	args = append(args, spec.Image, "postgres")
	for _, kv := range serverSettings(spec, srv.token, listen) {
		args = append(args, "-c", kv)
	}
	return args
}

// defaultSettings apply unless the configuration sets them. A lower
// deadlock_timeout makes log_lock_waits record waits from 200 ms instead
// of 1 s; in a disposable server the faster deadlock check costs nothing.
var defaultSettings = []string{"deadlock_timeout=200ms"}

// serverSettings returns the "-c name=value" parameters for the server in a
// deterministic order: defaults, then user settings, then managed
// parameters, so that later values win.
func serverSettings(spec Spec, token, listen string) []string {
	keys := make([]string, 0, len(spec.Settings))
	for k := range spec.Settings {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := append([]string(nil), defaultSettings...)
	for _, k := range keys {
		out = append(out, k+"="+spec.Settings[k])
	}
	return append(out,
		"listen_addresses="+listen,
		TokenSetting+"="+token,
		// Lock waits longer than deadlock_timeout are logged; useful
		// corroboration for the polling sampler.
		"log_lock_waits=on",
	)
}

func (p *containerProvider) hostPort(ctx context.Context, id string) (int, error) {
	out, stderr, err := p.run(ctx, p.binary, "port", id, "5432/tcp")
	if err != nil {
		return 0, commandError(p.binary, "port", stderr, err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		host, port, err := net.SplitHostPort(strings.TrimSpace(line))
		if err != nil || host != "127.0.0.1" {
			continue
		}
		n, err := strconv.Atoi(port)
		if err == nil {
			return n, nil
		}
	}
	return 0, fmt.Errorf("no loopback port published for container %s", id)
}

// waitReady waits for the image's initialisation to finish and then for a
// verified connection. The entrypoint briefly runs a temporary server on
// the same socket during initialisation, so a successful connection alone
// is not enough.
func (p *containerProvider) waitReady(ctx context.Context, srv *Server) error {
	for {
		logs, _ := p.Logs(ctx, srv)
		if idx := strings.Index(logs, "init process complete"); idx >= 0 &&
			strings.Contains(logs[idx:], "ready to accept connections") {
			conn, err := srv.Connect(ctx, srv.admin, ConnOptions{ApplicationName: "prodshadow/ready", ConnectTimeout: 2 * time.Second})
			if err == nil {
				Close(conn)
				return nil
			}
			if errors.Is(err, ErrOwnership) {
				return err
			}
		}
		if state, err := p.state(ctx, srv.id); err == nil && state != "running" && state != "created" {
			return fmt.Errorf("container exited during start-up (state %s)", state)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("server did not become ready: %w", ctx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func (p *containerProvider) state(ctx context.Context, id string) (string, error) {
	out, stderr, err := p.run(ctx, p.binary, "inspect", "--format", "{{.State.Status}}", id)
	if err != nil {
		return "", commandError(p.binary, "inspect", stderr, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (p *containerProvider) Logs(ctx context.Context, srv *Server) (string, error) {
	if srv == nil || srv.id == "" {
		return "", errors.New("no container")
	}
	out, stderr, err := p.run(ctx, p.binary, "logs", "--tail", "500", srv.id)
	if err != nil {
		return "", commandError(p.binary, "logs", stderr, err)
	}
	// Server logs are written to stderr by the image.
	return string(out) + string(stderr), nil
}

func (p *containerProvider) LogsSince(ctx context.Context, srv *Server, since time.Time) (string, error) {
	if srv == nil || srv.id == "" {
		return "", errors.New("no container")
	}
	out, stderr, err := p.run(ctx, p.binary, "logs", "--since", since.UTC().Format(time.RFC3339Nano), srv.id)
	if err != nil {
		return "", commandError(p.binary, "logs", stderr, err)
	}
	return string(out) + string(stderr), nil
}

func (p *containerProvider) Destroy(ctx context.Context, srv *Server) error {
	if srv == nil || !srv.managed {
		return ErrNotManaged
	}
	var errs []error
	if srv.id != "" {
		if _, stderr, err := p.run(ctx, p.binary, "rm", "--force", "--volumes", srv.id); err != nil {
			errs = append(errs, commandError(p.binary, "rm", stderr, err))
		}
	}
	if err := p.cleanupScratch(srv); err != nil {
		errs = append(errs, err)
	}
	srv.managed = false
	return errors.Join(errs...)
}

func (p *containerProvider) cleanupScratch(srv *Server) error {
	if srv.scratch == "" {
		return nil
	}
	if err := os.RemoveAll(srv.scratch); err != nil {
		return fmt.Errorf("remove scratch directory: %w", err)
	}
	srv.scratch = ""
	return nil
}

// failStart destroys a server whose start-up failed and attaches its logs.
func (p *containerProvider) failStart(srv *Server, cause error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	logs, _ := p.Logs(ctx, srv)
	if derr := p.Destroy(ctx, srv); derr != nil {
		cause = errors.Join(cause, derr)
	}
	if logs = lastLines(logs, 20); logs != "" {
		return fmt.Errorf("%w\nserver log (last lines):\n%s", cause, logs)
	}
	return cause
}

// ListOrphans returns container IDs labelled as ProdShadow resources. When
// runID is empty, containers from every run are returned.
func (p *containerProvider) ListOrphans(ctx context.Context, runID string) ([]string, error) {
	args := []string{"ps", "--all", "--quiet", "--filter", "label=" + LabelManaged + "=true"}
	if runID != "" {
		args = append(args, "--filter", "label="+LabelRun+"="+runID)
	}
	out, stderr, err := p.run(ctx, p.binary, args...)
	if err != nil {
		return nil, commandError(p.binary, "ps", stderr, err)
	}
	return strings.Fields(string(out)), nil
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
