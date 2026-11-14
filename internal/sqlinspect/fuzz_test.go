package sqlinspect

import (
	"testing"
	"time"
)

func FuzzAnnotations(f *testing.F) {
	f.Add(`prodshadow:ignore PS101 PS102 reason="a \"quoted\" reason" expires=2027-01-31`)
	f.Add("prodshadow:ignore")
	f.Add(`prodshadow:ignore PS1 reason=`)
	f.Fuzz(func(t *testing.T, comment string) {
		s := &Statement{Annotations: parseAnnotations([]string{comment})}
		for _, a := range s.Annotations {
			if a.Problem == "" && (len(a.Rules) == 0 || a.Reason == "") {
				t.Fatalf("annotation accepted without rules or reason: %+v", a)
			}
		}
		s.Suppresses("PS101", time.Now())
	})
}

func FuzzAnalyze(f *testing.F) {
	f.Add("CREATE INDEX i ON t (a); ALTER TABLE t ADD COLUMN c int DEFAULT random();")
	f.Add("BEGIN; CREATE INDEX CONCURRENTLY i ON t (a); COMMIT;")
	f.Add("SET lock_timeout = '1min'; DROP INDEX i; UPDATE t SET a = 1;")
	f.Fuzz(func(t *testing.T, sql string) {
		stmts, err := ParseSources([]Source{{Path: "f.sql", Content: sql}})
		if err != nil {
			return
		}
		for _, mode := range []TransactionMode{TxnModeAuto, TxnModePerFile, TxnModeNone} {
			Analyze(stmts, Options{Transaction: mode})
		}
	})
}

func FuzzCatalogResolve(f *testing.F) {
	f.Add("CREATE TABLE p (id int PRIMARY KEY) PARTITION BY RANGE (id); CREATE TABLE c PARTITION OF p DEFAULT;",
		"CREATE TABLE c2 PARTITION OF p FOR VALUES FROM (1) TO (2); ALTER TABLE p DETACH PARTITION c CONCURRENTLY;")
	f.Add("CREATE TABLE a (id serial PRIMARY KEY); CREATE TABLE b (a_id int REFERENCES a ON DELETE CASCADE); CREATE VIEW v AS SELECT * FROM a;",
		"DELETE FROM a; ALTER TABLE a DROP COLUMN id; CREATE OR REPLACE VIEW v AS SELECT 1; INSERT INTO a DEFAULT VALUES;")
	f.Add("CREATE MATERIALIZED VIEW m AS SELECT 1 AS x WITH NO DATA; CREATE UNIQUE INDEX ON m (x);",
		"REFRESH MATERIALIZED VIEW CONCURRENTLY m; DROP INDEX i; TRUNCATE a CASCADE;")
	f.Fuzz(func(t *testing.T, schema, migration string) {
		s, err := ParseSources([]Source{{Path: "s.sql", Content: schema}})
		if err != nil {
			return
		}
		m, err := ParseSources([]Source{{Path: "m.sql", Content: migration}})
		if err != nil {
			return
		}
		NewCatalog(s).Resolve(m)
		Analyze(m, Options{Catalog: NewCatalog(s)})
		for _, st := range m {
			PredictConflicts(m, st)
		}
	})
}
