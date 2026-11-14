package pglock

// statementLocks lists the table lock that common application statements
// take, from "Table-Level Lock Modes" in the PostgreSQL manual.
var statementLocks = []struct {
	Statement string
	Mode      Mode
}{
	{"SELECT", AccessShare},
	{"SELECT ... FOR UPDATE/SHARE", RowShare},
	{"INSERT/UPDATE/DELETE/MERGE", RowExclusive},
	{"VACUUM/ANALYZE/CREATE INDEX CONCURRENTLY", ShareUpdateExclusive},
	{"CREATE INDEX", Share},
	{"CREATE TRIGGER/ADD FOREIGN KEY", ShareRowExclusive},
	{"REFRESH MATERIALIZED VIEW CONCURRENTLY", Exclusive},
	{"ALTER TABLE/DROP TABLE/TRUNCATE", AccessExclusive},
}

// BlockedStatements returns the statement classes that wait while m is held
// (or queued) on the same table, in increasing lock strength.
func (m Mode) BlockedStatements() []string {
	var out []string
	for _, s := range statementLocks {
		if Conflicts(m, s.Mode) {
			out = append(out, s.Statement)
		}
	}
	return out
}

// Modes returns every table lock mode, weakest first.
func Modes() []Mode {
	out := make([]Mode, 0, int(AccessExclusive))
	for m := AccessShare; m <= AccessExclusive; m++ {
		out = append(out, m)
	}
	return out
}

// AcquiredBy returns the common statement classes that take m.
func (m Mode) AcquiredBy() []string {
	var out []string
	for _, s := range statementLocks {
		if s.Mode == m {
			out = append(out, s.Statement)
		}
	}
	return out
}

// ConflictingModes returns the modes that cannot be held together with m on
// the same table, weakest first.
func (m Mode) ConflictingModes() []Mode {
	var out []Mode
	for _, o := range Modes() {
		if Conflicts(m, o) {
			out = append(out, o)
		}
	}
	return out
}
