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
