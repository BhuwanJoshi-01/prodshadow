package isolation

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/security"
)

// localProvider runs PostgreSQL binaries installed on the host, in a private
// temporary directory. It cannot enforce CPU, memory or disk limits and is
// intended for development machines and CI runners without a container
// engine.
type localProvider struct {
	binDir string
	run    runner

	mu    sync.Mutex
	procs map[string]*exec.Cmd
}

func newLocalProvider(binDir string) *localProvider {
	return &localProvider{binDir: binDir, run: execRunner, procs: map[string]*exec.Cmd{}}
}

func (p *localProvider) Name() string { return "local" }

func (p *localProvider) Check(ctx context.Context) error {
	if os.Geteuid() == 0 {
		return errors.New("the local provider cannot run as root because PostgreSQL refuses to; use docker or podman, or run as an unprivileged user")
	}
	_, err := p.findBinDir(ctx, "")
	return err
}

var versionRe = regexp.MustCompile(`\(PostgreSQL\) (\d+)`)

// findBinDir locates a bin directory containing initdb and postgres. When
// major is non-empty, the binaries must be that major version.
func (p *localProvider) findBinDir(ctx context.Context, major string) (string, error) {
	var candidates []string
	if p.binDir != "" {
		candidates = append(candidates, p.binDir)
	} else {
		if major != "" {
			candidates = append(candidates, filepath.Join("/usr/lib/postgresql", major, "bin"), filepath.Join("/usr/pgsql-"+major, "bin"), filepath.Join("/opt/homebrew/opt/postgresql@"+major, "bin"))
		}
		if out, _, err := p.run(ctx, "pg_config", "--bindir"); err == nil {
			candidates = append(candidates, strings.TrimSpace(string(out)))
		}
		if path, err := exec.LookPath("postgres"); err == nil {
			candidates = append(candidates, filepath.Dir(path))
		}
	}
	var mismatch string
	for _, dir := range candidates {
		if _, err := os.Stat(filepath.Join(dir, "initdb")); err != nil {
			continue
		}
		out, _, err := p.run(ctx, filepath.Join(dir, "postgres"), "--version")
		if err != nil {
			continue
		}
		m := versionRe.FindStringSubmatch(string(out))
		if m == nil {
			continue
		}
		if major != "" && m[1] != major {
			mismatch = fmt.Sprintf("%s has PostgreSQL %s", dir, m[1])
			continue
		}
		return dir, nil
	}
	if mismatch != "" {
		return "", fmt.Errorf("no local PostgreSQL %s binaries found (%s); set postgres.isolation.binary to the right bin directory", major, mismatch)
	}
	return "", errors.New("no local PostgreSQL binaries found; install PostgreSQL or set postgres.isolation.binary")
}

func (p *localProvider) Start(ctx context.Context, spec Spec) (*Server, error) {
	if err := spec.validate(); err != nil {
		return nil, err
	}
	if os.Geteuid() == 0 {
		return nil, errors.New("the local provider cannot run as root because PostgreSQL refuses to")
	}
	bin, err := p.findBinDir(ctx, spec.PostgresMajor)
	if err != nil {
		return nil, err
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

	scratch, err := os.MkdirTemp("", "prodshadow-"+spec.RunID+"-")
	if err != nil {
		return nil, err
	}
	srv := &Server{
		provider: "local",
		id:       scratch,
		scratch:  scratch,
		major:    spec.PostgresMajor,
		token:    token,
		admin:    Credential{User: AdminUser, password: password},
		managed:  true,
	}
	fail := func(err error) (*Server, error) {
		_ = p.Destroy(context.Background(), srv)
		return nil, err
	}

	dataDir, sockDir := filepath.Join(scratch, "data"), filepath.Join(scratch, "sock")
	if err := os.Mkdir(sockDir, 0o700); err != nil {
		return fail(err)
	}
	pwfile := filepath.Join(scratch, "pwfile")
	if err := os.WriteFile(pwfile, []byte(password+"\n"), 0o600); err != nil {
		return fail(err)
	}
	_, stderr, err := p.run(ctx, filepath.Join(bin, "initdb"),
		"--pgdata", dataDir, "--username", AdminUser, "--pwfile", pwfile,
		"--auth-local=scram-sha-256", "--auth-host=scram-sha-256",
		"--encoding", "UTF8", "--locale", "C", "--no-sync")
	_ = os.Remove(pwfile)
	if err != nil {
		return fail(commandError("initdb", "", stderr, err))
	}

	srv.host, srv.port, err = localListen(spec.Network, sockDir)
	if err != nil {
		return fail(err)
	}
	listen := ""
	if spec.Network == "loopback" {
		listen = "127.0.0.1"
	}
	args := []string{"-D", dataDir, "-k", sockDir, "-p", strconv.Itoa(srv.port)}
	for _, kv := range serverSettings(spec, token, listen) {
		args = append(args, "-c", kv)
	}
	logFile, err := os.OpenFile(filepath.Join(scratch, "server.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fail(err)
	}
	cmd := exec.Command(filepath.Join(bin, "postgres"), args...)
	cmd.Stdout, cmd.Stderr = logFile, logFile
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return fail(fmt.Errorf("start postgres: %w", err))
	}
	logFile.Close()
	p.mu.Lock()
	p.procs[srv.id] = cmd
	p.mu.Unlock()

	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	waitCtx, cancel := context.WithTimeout(ctx, spec.StartupTimeout)
	defer cancel()
	for {
		conn, err := srv.Connect(waitCtx, srv.admin, ConnOptions{ApplicationName: "prodshadow/ready", ConnectTimeout: 2 * time.Second})
		if err == nil {
			conn.Close(waitCtx)
			srv.started = time.Now()
			go func() { <-exited }()
			return srv, nil
		}
		select {
		case werr := <-exited:
			p.forget(srv)
			logs, _ := p.Logs(context.Background(), srv)
			return fail(fmt.Errorf("postgres exited during start-up: %v\n%s", werr, lastLines(logs, 20)))
		case <-waitCtx.Done():
			logs, _ := p.Logs(context.Background(), srv)
			return fail(fmt.Errorf("server did not become ready: %w\n%s", waitCtx.Err(), lastLines(logs, 20)))
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// localListen chooses the address the local server listens on.
func localListen(network, sockDir string) (string, int, error) {
	if network == "none" {
		return sockDir, 5432, nil
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", 0, fmt.Errorf("find a free loopback port: %w", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return "127.0.0.1", port, nil
}

func (p *localProvider) forget(srv *Server) {
	p.mu.Lock()
	delete(p.procs, srv.id)
	p.mu.Unlock()
}

func (p *localProvider) Logs(_ context.Context, srv *Server) (string, error) {
	if srv == nil || srv.scratch == "" {
		return "", errors.New("no server directory")
	}
	b, err := os.ReadFile(filepath.Join(srv.scratch, "server.log"))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (p *localProvider) Destroy(_ context.Context, srv *Server) error {
	if srv == nil || !srv.managed {
		return ErrNotManaged
	}
	var errs []error
	p.mu.Lock()
	cmd := p.procs[srv.id]
	delete(p.procs, srv.id)
	p.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		// SIGINT requests a fast shutdown.
		_ = cmd.Process.Signal(os.Interrupt)
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			if err := cmd.Process.Signal(syscallZero()); err != nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		_ = cmd.Process.Kill()
	}
	if srv.scratch != "" {
		if err := os.RemoveAll(srv.scratch); err != nil {
			errs = append(errs, fmt.Errorf("remove server directory: %w", err))
		}
	}
	srv.managed = false
	return errors.Join(errs...)
}
