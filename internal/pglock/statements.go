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
