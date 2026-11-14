package experiment

import (
	"strings"
	"testing"

	"github.com/BhuwanJoshi-01/prodshadow/internal/config"
	"github.com/BhuwanJoshi-01/prodshadow/internal/workload"
)

func TestCheckCapacity(t *testing.T) {
	cfg := &config.Config{}
	cfg.ApplyDefaults()
	wl := &workload.Spec{Background: []workload.Background{{Sessions: 4}}}

	cfg.Workload.Concurrency = 90
	if err := CheckCapacity(cfg, wl); err != nil {
		t.Fatalf("90 + 4 + 3 fits into 97: %v", err)
	}
	cfg.Workload.Concurrency = 91
	err := CheckCapacity(cfg, wl)
	if err == nil || !strings.Contains(err.Error(), "max_connections to at least 101") {
		t.Fatalf("expected capacity error, got %v", err)
	}
	cfg.Postgres.Settings = map[string]string{"max_connections": "200"}
	if err := CheckCapacity(cfg, wl); err != nil {
		t.Fatalf("raised max_connections should fit: %v", err)
	}
	cfg.Postgres.Settings = map[string]string{"max_connections": "lots"}
	if err := CheckCapacity(cfg, wl); err == nil {
		t.Fatal("non-numeric max_connections must be rejected")
	}
	if err := CheckCapacity(cfg, nil); err != nil {
		t.Fatal("no workload, nothing to check")
	}
}
