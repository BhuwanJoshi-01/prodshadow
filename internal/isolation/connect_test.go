package isolation

import (
	"context"
	"errors"
	"testing"

	"github.com/BhuwanJoshi-01/prodshadow/internal/security"
)

func TestConnectRefusesUnmanagedServer(t *testing.T) {
	s := &Server{host: "/tmp/sock", port: 5432, token: "t"}
	_, err := s.Connect(context.Background(), Credential{User: "x"}, ConnOptions{})
	if !errors.Is(err, ErrNotManaged) {
		t.Fatalf("expected ErrNotManaged, got %v", err)
	}
	var nilServer *Server
	if _, err := nilServer.connConfig(Credential{User: "x"}, ConnOptions{}); !errors.Is(err, ErrNotManaged) {
		t.Fatalf("nil server: %v", err)
	}
}

func TestConnectRefusesRemoteHostBeforeDialing(t *testing.T) {
	s := &Server{host: "203.0.113.10", port: 5432, managed: true, token: "t"}
	_, err := s.connConfig(Credential{User: "x"}, ConnOptions{})
	if !errors.Is(err, security.ErrUnmanagedTarget) {
		t.Fatalf("expected ErrUnmanagedTarget, got %v", err)
	}
}

func TestConnConfigIgnoresEnvironment(t *testing.T) {
	t.Setenv("PGHOST", "prod-db.example.com")
	t.Setenv("PGPORT", "6543")
	t.Setenv("PGUSER", "prod")
	t.Setenv("PGDATABASE", "prod")
	t.Setenv("PGSSLMODE", "require")
	s := &Server{host: "/tmp/ps/sock", port: 5432, managed: true, token: "t"}
	cfg, err := s.connConfig(Credential{User: AdminUser, password: "pw"}, ConnOptions{ApplicationName: "prodshadow/test"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "/tmp/ps/sock" || cfg.Port != 5432 || cfg.User != AdminUser || cfg.Database != "postgres" {
		t.Fatalf("environment leaked into config: %+v", cfg)
	}
	if cfg.TLSConfig != nil || len(cfg.Fallbacks) != 0 {
		t.Fatal("TLS and fallbacks must be cleared")
	}
	if cfg.RuntimeParams["application_name"] != "prodshadow/test" {
		t.Fatalf("application_name = %q", cfg.RuntimeParams["application_name"])
	}
}

func TestGuardedDialRejectsRemoteAddresses(t *testing.T) {
	for _, tc := range []struct{ network, addr string }{
		{"tcp", "203.0.113.10:5432"},
		{"tcp", "db.internal:5432"},
		{"tcp6", "[2001:db8::1]:5432"},
		{"udp", "127.0.0.1:5432"},
		{"unix", "relative/.s.PGSQL.5432"},
	} {
		if err := checkDialTarget(tc.network, tc.addr); !errors.Is(err, security.ErrUnmanagedTarget) {
			t.Errorf("%s %s should be rejected, got %v", tc.network, tc.addr, err)
		}
	}
	for _, tc := range []struct{ network, addr string }{
		{"tcp", "127.0.0.1:49153"},
		{"tcp6", "[::1]:49153"},
		{"unix", "/tmp/ps/sock/.s.PGSQL.5432"},
	} {
		if err := checkDialTarget(tc.network, tc.addr); err != nil {
			t.Errorf("%s %s should be allowed: %v", tc.network, tc.addr, err)
		}
	}
}

func TestConnConfigRejectsInvalidPort(t *testing.T) {
	s := &Server{host: "127.0.0.1", port: 70000, managed: true}
	if _, err := s.connConfig(Credential{User: "x"}, ConnOptions{}); err == nil {
		t.Fatal("out-of-range port must be rejected")
	}
}
