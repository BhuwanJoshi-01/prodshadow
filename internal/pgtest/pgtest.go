// Package pgtest starts disposable PostgreSQL servers for integration
// tests. The image and major version come from PRODSHADOW_IT_IMAGE and
// PRODSHADOW_IT_MAJOR (default postgres:17-alpine), and the provider from
// PRODSHADOW_IT_PROVIDER (default docker), so the CI matrix exercises every
// supported version with the same tests.
package pgtest

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/isolation"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/security"
)

// Image returns the image and major version under test.
func Image() (image, major string) {
	image, major = os.Getenv("PRODSHADOW_IT_IMAGE"), os.Getenv("PRODSHADOW_IT_MAJOR")
	if image == "" {
		return "postgres:17-alpine", "17"
	}
	if major == "" {
		major = "17"
	}
	return image, major
}

// ProviderName returns the provider under test.
func ProviderName() string {
	if p := os.Getenv("PRODSHADOW_IT_PROVIDER"); p != "" {
		return p
	}
	return "docker"
}

// Provider returns the provider under test or skips the test when it is
// not available on this machine.
func Provider(t testing.TB) isolation.Provider {
	t.Helper()
	p, err := isolation.New(ProviderName(), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Check(context.Background()); err != nil {
		t.Skipf("%s provider unavailable: %v", p.Name(), err)
	}
	return p
}

// Spec returns a small server specification for tests.
func Spec(t testing.TB) isolation.Spec {
	t.Helper()
	image, major := Image()
	runID, err := security.RunID()
	if err != nil {
		t.Fatal(err)
	}
	return isolation.Spec{RunID: runID, PostgresMajor: major, Image: image, Network: "none",
		MemoryMB: 1024, CPUs: 2, DiskMB: 1024, PidsLimit: 256, StartupTimeout: 2 * time.Minute}
}

// Start creates a server that is destroyed when the test ends.
func Start(t testing.TB) (isolation.Provider, *isolation.Server) {
	t.Helper()
	p := Provider(t)
	srv, err := p.Start(context.Background(), Spec(t))
	if err != nil {
		t.Fatalf("start disposable server: %v", err)
	}
	t.Cleanup(func() { _ = p.Destroy(context.Background(), srv) })
	return p, srv
}
