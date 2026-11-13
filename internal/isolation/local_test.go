package isolation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeBin creates a directory with initdb and postgres placeholders.
func fakeBin(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"initdb", "postgres"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func versionRunner(versions map[string]string) runner {
	return func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if v, ok := versions[name]; ok {
			return []byte("postgres (PostgreSQL) " + v + "\n"), nil, nil
		}
		return nil, nil, errors.New("not found")
	}
}

func TestFindBinDirMatchesMajor(t *testing.T) {
	dir := fakeBin(t)
	p := newLocalProvider(dir)
	p.run = versionRunner(map[string]string{filepath.Join(dir, "postgres"): "16.4 (Ubuntu 16.4-1)"})
	got, err := p.findBinDir(context.Background(), "16")
	if err != nil || got != dir {
		t.Fatalf("findBinDir = %q, %v", got, err)
	}
	_, err = p.findBinDir(context.Background(), "17")
	if err == nil || !strings.Contains(err.Error(), "has PostgreSQL 16") {
		t.Fatalf("version mismatch should be explained, got %v", err)
	}
}

func TestFindBinDirWithoutBinaries(t *testing.T) {
	p := newLocalProvider(t.TempDir())
	p.run = versionRunner(nil)
	if _, err := p.findBinDir(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "no local PostgreSQL binaries") {
		t.Fatalf("got %v", err)
	}
}

func TestLocalProviderRefusesRoot(t *testing.T) {
	p := newLocalProvider(fakeBin(t))
	err := p.Check(context.Background())
	if os.Geteuid() == 0 {
		if err == nil || !strings.Contains(err.Error(), "root") {
			t.Fatalf("running as root must be refused, got %v", err)
		}
		if _, err := p.Start(context.Background(), testSpec()); err == nil {
			t.Fatal("Start must refuse to run as root")
		}
	}
}

func TestLocalListen(t *testing.T) {
	host, port, err := localListen(context.Background(), "none", "/tmp/x/sock")
	if err != nil || host != "/tmp/x/sock" || port != 5432 {
		t.Fatalf("none: %s %d %v", host, port, err)
	}
	host, port, err = localListen(context.Background(), "loopback", "")
	if err != nil || host != "127.0.0.1" || port < 1024 {
		t.Fatalf("loopback: %s %d %v", host, port, err)
	}
}
