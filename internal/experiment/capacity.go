package experiment

import (
	"fmt"
	"strconv"

	"github.com/BhuwanJoshi-01/prodshadow/internal/config"
	"github.com/BhuwanJoshi-01/prodshadow/internal/workload"
)

// Defaults of the PostgreSQL server parameters that bound connections.
const (
	defaultMaxConnections   = 100
	defaultReservedSuperuse = 3
)

// overheadSessions are ProdShadow's own sessions during a phase: the
// administrative session, the telemetry sampler and the migration.
const overheadSessions = 3

// CheckCapacity verifies that the workload fits within the server's
// connection limit, so a run does not fail halfway with connection errors.
func CheckCapacity(cfg *config.Config, wl *workload.Spec) error {
	if wl == nil {
		return nil
	}
	maxConn, err := intSetting(cfg, "max_connections", defaultMaxConnections)
	if err != nil {
		return err
	}
	reserved, err := intSetting(cfg, "superuser_reserved_connections", defaultReservedSuperuse)
	if err != nil {
		return err
	}
	sessions := cfg.Workload.Concurrency
	for _, b := range wl.Background {
		sessions += b.Sessions
	}
	need := sessions + overheadSessions
	if limit := maxConn - reserved; need > limit {
		return fmt.Errorf("the workload needs %d sessions (%d workload + %d for ProdShadow) but the server allows %d non-superuser connections; lower workload.concurrency or set postgres.settings.max_connections to at least %d",
			need, sessions, overheadSessions, limit, need+reserved)
	}
	return nil
}

func intSetting(cfg *config.Config, name string, def int) (int, error) {
	v, ok := cfg.Postgres.Settings[name]
	if !ok {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("postgres.settings.%s must be a positive integer, got %q", name, v)
	}
	return n, nil
}
