package sqlinspect

import (
	"github.com/BhuwanJoshi-01/prodshadow/internal/pglock"
)

// Conflict is a predicted lock conflict between a migration statement and
// a statement the application runs. While the migration holds its lock, or
// waits for it in the queue, the application statement waits; while the
// application statement holds its lock, the migration waits.
type Conflict struct {
	StatementID   string      `json:"statement_id"`
	Table         string      `json:"table"`
	MigrationLock pglock.Mode `json:"migration_lock"`
	WorkloadLock  pglock.Mode `json:"workload_lock"`
}

// PredictConflicts lists the migration statements whose locks conflict
// with the locks of app, a workload statement. Locks on relations the
// migration itself created are ignored: the application cannot use them
// yet. Resolve both sides with the same Catalog first to include the
// locks they take on relations they do not name.
func PredictConflicts(migration []*Statement, app *Statement) []Conflict {
	var out []Conflict
	for _, s := range migration {
		for _, m := range s.Locks {
			if m.CreatedBy != "" {
				continue
			}
			for _, a := range app.Locks {
				if catalogKey(a.Table) == catalogKey(m.Table) && pglock.Conflicts(m.Mode, a.Mode) {
					out = append(out, Conflict{StatementID: s.ID, Table: m.Table, MigrationLock: m.Mode, WorkloadLock: a.Mode})
				}
			}
		}
	}
	return out
}
