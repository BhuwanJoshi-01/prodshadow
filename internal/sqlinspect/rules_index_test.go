package sqlinspect

import (
	"testing"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"
)

func TestPS101BlockingIndex(t *testing.T) {
	got := findings(t, "PS101", Options{}, "\n\nCREATE INDEX idx ON orders (customer_id);")
	expectCount(t, got, 1)
	if got[0].Line != 3 || got[0].Table != "orders" || got[0].Severity != evidence.Warning {
		t.Fatalf("unexpected finding %+v", got[0])
	}
	expectCount(t, findings(t, "PS101", Options{}, "CREATE INDEX CONCURRENTLY idx ON orders (customer_id);"), 0)
}

func TestPS101IgnoresTablesCreatedInMigration(t *testing.T) {
	got := findings(t, "PS101", Options{}, "CREATE TABLE fresh (id int); CREATE INDEX ON fresh (id);")
	expectCount(t, got, 0)
	got = findings(t, "PS101", Options{}, "CREATE TABLE public.fresh (id int); CREATE INDEX ON fresh (id);")
	expectCount(t, got, 0)
}

func TestPS102ConcurrentInsideTransaction(t *testing.T) {
	got := findings(t, "PS102", Options{}, "BEGIN; CREATE INDEX CONCURRENTLY idx ON orders (a); COMMIT;")
	expectCount(t, got, 1)
	if got[0].Severity != evidence.Critical {
		t.Fatalf("severity = %s", got[0].Severity)
	}
	expectCount(t, findings(t, "PS102", Options{Transaction: TxnModePerFile}, "CREATE INDEX CONCURRENTLY idx ON orders (a);"), 1)
	expectCount(t, findings(t, "PS102", Options{}, "CREATE INDEX CONCURRENTLY idx ON orders (a);"), 0)
}

func TestPS121ConcurrentCaveats(t *testing.T) {
	got := findings(t, "PS121", Options{}, "CREATE INDEX CONCURRENTLY idx ON orders (a);")
	expectCount(t, got, 1)
	if got[0].Severity != evidence.Info {
		t.Fatalf("severity = %s", got[0].Severity)
	}
}

func TestOverridesAndSuppression(t *testing.T) {
	sql := "CREATE INDEX idx ON orders (a);"
	got := findings(t, "PS101", Options{Overrides: map[string]string{"PS101": "critical"}}, sql)
	if len(got) != 1 || got[0].Severity != evidence.Critical {
		t.Fatalf("override not applied: %+v", got)
	}
	expectCount(t, findings(t, "PS101", Options{Overrides: map[string]string{"PS101": "off"}}, sql), 0)

	annotated := "-- prodshadow:ignore PS101 reason=\"orders has 40 rows\"\n" + sql
	got = findings(t, "PS101", Options{}, annotated)
	if len(got) != 1 || !got[0].Suppressed || got[0].SuppressionReason != "orders has 40 rows" {
		t.Fatalf("suppression not applied: %+v", got)
	}
}

func TestNewRelationsAreNotTraffic(t *testing.T) {
	// Relations created earlier in the migration have no traffic yet, so
	// locking them is not flagged, whichever statement created them.
	for _, create := range []string{
		"CREATE TABLE totals (id int);",
		"CREATE TABLE totals AS SELECT 1 AS id;",
		"CREATE MATERIALIZED VIEW totals AS SELECT 1 AS id;",
	} {
		expectCount(t, findings(t, "PS101", Options{}, create+"\nCREATE INDEX totals_id ON totals (id);"), 0)
	}
	expectCount(t, findings(t, "PS101", Options{}, "CREATE INDEX totals_id ON totals (id);"), 1)
}
