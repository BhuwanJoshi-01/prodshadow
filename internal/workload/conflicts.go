package workload

import "github.com/BhuwanJoshi-01/prodshadow/internal/sqlinspect"

// Conflict is a predicted lock conflict between a migration statement and
// the SQL of a scenario or background session: whichever comes second
// waits for the other.
type Conflict struct {
	Session       string `json:"session"`
	Background    bool   `json:"background,omitempty"`
	StatementID   string `json:"statement_id"`
	Table         string `json:"table"`
	MigrationLock string `json:"migration_lock"`
	WorkloadLock  string `json:"workload_lock"`
}

// PredictConflicts lists, for every scenario and background session, the
// migration statements whose locks conflict with the locks its statements
// take, and the sessions no statement conflicts with. catalog (may be nil)
// resolves locks on relations the SQL does not name, such as the
// referenced table of a foreign key.
func PredictConflicts(spec *Spec, migration []*sqlinspect.Statement, catalog *sqlinspect.Catalog) (conflicts []Conflict, unblocked []string) {
	type session struct {
		id         string
		background bool
		sqls       []string
	}
	sessions := make([]session, 0, len(spec.Scenarios)+len(spec.Background))
	for _, sc := range spec.Scenarios {
		sqls := []string{sc.SQL}
		if sc.Transaction != nil {
			sqls = append(sqls, sc.Transaction.Statements...)
		}
		sessions = append(sessions, session{sc.ID, false, sqls})
	}
	for _, b := range spec.Background {
		sessions = append(sessions, session{b.ID, true, b.Transaction.Statements})
	}
	for _, ses := range sessions {
		seen := map[Conflict]bool{}
		for _, q := range ses.sqls {
			if q == "" {
				continue
			}
			st, err := sqlinspect.ParseOne(q)
			if err != nil {
				continue
			}
			catalog.Resolve([]*sqlinspect.Statement{st})
			for _, c := range sqlinspect.PredictConflicts(migration, st) {
				pc := Conflict{Session: ses.id, Background: ses.background, StatementID: c.StatementID, Table: c.Table,
					MigrationLock: c.MigrationLock.String(), WorkloadLock: c.WorkloadLock.String()}
				if !seen[pc] {
					seen[pc] = true
					conflicts = append(conflicts, pc)
				}
			}
		}
		if len(seen) == 0 {
			unblocked = append(unblocked, ses.id)
		}
	}
	return conflicts, unblocked
}
