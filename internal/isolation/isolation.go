// Package isolation provisions disposable PostgreSQL servers and is the only
// place that can hand out database connections.
//
// Safety invariants (see docs/threat-model.md):
//
//   - A Server can only be created by a Provider in this package; its
//     credentials and ownership token are unexported.
//   - Connections are only made to unix sockets or loopback addresses, and
//     the dialer re-checks the address at connect time.
//   - Every new connection verifies the per-run ownership token
//     (prodshadow.run_token) before it is returned to the caller.
//   - Servers carry labels so that orphaned resources can be found and
//     removed, and Destroy reports cleanup failures as errors.
package isolation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/BhuwanJoshi-01/prodshadow/internal/security"
)

// Label keys applied to container resources.
const (
	LabelManaged = "io.prodshadow.managed"
	LabelRun     = "io.prodshadow.run"
)

// TokenSetting is the custom server parameter holding the ownership token.
const TokenSetting = "prodshadow.run_token"

// AdminUser is the superuser created inside every disposable server.
const AdminUser = "ps_admin"

// Spec describes the server to create.
type Spec struct {
	RunID          string
	PostgresMajor  string
	Image          string
	Network        string
	MemoryMB       int
	CPUs           float64
	DiskMB         int
	PidsLimit      int
	StartupTimeout time.Duration
	// Settings are extra server parameters, already validated.
	Settings map[string]string
	// Binary overrides the container CLI or local bin directory.
	Binary string
	// Redactor receives every secret generated for the server.
	Redactor *security.Redactor
}

func (s Spec) validate() error {
	switch {
	case s.RunID == "":
		return errors.New("isolation: spec has no run ID")
	case s.PostgresMajor == "":
		return errors.New("isolation: spec has no PostgreSQL major version")
	case s.Network != "none" && s.Network != "loopback":
		return fmt.Errorf("isolation: unsupported network mode %q", s.Network)
	case s.StartupTimeout <= 0:
		return errors.New("isolation: startup timeout must be positive")
	}
	return nil
}

// Credential is a login role on a managed server.
type Credential struct {
	User     string
	password string
}

// Server is a running disposable PostgreSQL server.
type Server struct {
	provider string
	id       string
	host     string
	port     int
	major    string
	token    string
	admin    Credential
	app      *Credential
	// scratch holds provider-owned temporary paths removed on destroy.
	scratch string
	// image is the container image, reused for cleanup helpers.
	image string
	// managed is only set by providers in this package.
	managed bool
	started time.Time
}

// ID identifies the server within its provider (container ID or data dir).
func (s *Server) ID() string { return s.id }

// Provider names the provider that created the server.
func (s *Server) Provider() string { return s.provider }

// Address describes where the server listens, for diagnostics.
func (s *Server) Address() string {
	return fmt.Sprintf("%s:%d", s.host, s.port)
}

// Major returns the PostgreSQL major version requested for the server.
func (s *Server) Major() string { return s.major }

// Admin returns the superuser credential.
func (s *Server) Admin() Credential { return s.admin }

// App returns the unprivileged application credential, if bootstrapped.
func (s *Server) App() (Credential, bool) {
	if s.app == nil {
		return Credential{}, false
	}
	return *s.app, true
}

// StartedAt is when the server became ready.
func (s *Server) StartedAt() time.Time { return s.started }

// Provider creates and destroys disposable servers.
type Provider interface {
	// Name is "docker", "podman" or "local".
	Name() string
	// Check verifies that the provider can run on this machine.
	Check(ctx context.Context) error
	// Start creates a server and waits until it accepts connections.
	Start(ctx context.Context, spec Spec) (*Server, error)
	// Logs returns recent server log output for diagnostics.
	Logs(ctx context.Context, s *Server) (string, error)
	// LogsSince returns server log output written after since. Lines may
	// still need filtering by their own timestamps.
	LogsSince(ctx context.Context, s *Server, since time.Time) (string, error)
	// Destroy removes the server and all of its data.
	Destroy(ctx context.Context, s *Server) error
}

// New returns the provider with the given name.
func New(name, binary string) (Provider, error) {
	switch name {
	case "docker":
		return newContainerProvider("docker", binary), nil
	case "podman":
		return newContainerProvider("podman", binary), nil
	case "local":
		return newLocalProvider(binary), nil
	default:
		return nil, fmt.Errorf("isolation: unknown provider %q", name)
	}
}
