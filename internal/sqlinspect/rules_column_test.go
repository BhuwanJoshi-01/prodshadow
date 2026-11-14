package sqlinspect

import (
	"testing"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"
)

func TestPS103AddColumnRewrite(t *testing.T) {
	expectCount(t, findings(t, "PS103", Options{}, "ALTER TABLE orders ADD COLUMN ref uuid DEFAULT gen_random_uuid();"), 1)
	expectCount(t, findings(t, "PS103", Options{}, "ALTER TABLE orders ADD COLUMN flag bool DEFAULT false;"), 0)
	got := findings(t, "PS103", Options{}, "ALTER TABLE orders ADD COLUMN x text DEFAULT app_default();")
	expectCount(t, got, 1)
	if got[0].Confidence != evidence.Low || got[0].Evidence != evidence.Heuristic {
		t.Fatalf("unknown functions should be a low-confidence heuristic: %+v", got[0])
	}
	expectCount(t, findings(t, "PS103", Options{}, "CREATE TABLE n (id int); ALTER TABLE n ADD COLUMN s bigserial;"), 0)
}

func TestPS104NotNullWithoutDefault(t *testing.T) {
	expectCount(t, findings(t, "PS104", Options{}, "ALTER TABLE orders ADD COLUMN region text NOT NULL;"), 1)
	expectCount(t, findings(t, "PS104", Options{}, "ALTER TABLE orders ADD COLUMN region text NOT NULL DEFAULT 'eu';"), 0)
	expectCount(t, findings(t, "PS104", Options{}, "ALTER TABLE orders ADD COLUMN id2 int NOT NULL GENERATED ALWAYS AS IDENTITY;"), 0)
	expectCount(t, findings(t, "PS104", Options{}, "ALTER TABLE orders ADD COLUMN region text;"), 0)
}

func TestPS105AlterColumnType(t *testing.T) {
	got := findings(t, "PS105", Options{}, "ALTER TABLE orders ALTER COLUMN amount TYPE bigint;")
	expectCount(t, got, 1)
	if got[0].Confidence != evidence.Medium {
		t.Fatalf("confidence = %s", got[0].Confidence)
	}
	got = findings(t, "PS105", Options{}, "ALTER TABLE orders ALTER COLUMN note TYPE text;")
	expectCount(t, got, 1)
	if got[0].Confidence != evidence.Low {
		t.Fatalf("text target should lower confidence, got %s", got[0].Confidence)
	}
	cases := map[string]evidence.Confidence{
		"ALTER TABLE orders ALTER COLUMN code TYPE varchar(100);":                     evidence.Low,
		"ALTER TABLE orders ALTER COLUMN price TYPE numeric(14, 2);":                  evidence.Low,
		"ALTER TABLE orders ALTER COLUMN ref TYPE integer USING ref::integer;":        evidence.High,
		"ALTER TABLE orders ALTER COLUMN note TYPE text USING lower(note);":           evidence.High,
		"ALTER TABLE orders ALTER COLUMN note TYPE text USING note;":                  evidence.Low,
		"ALTER TABLE orders ALTER COLUMN code TYPE varchar(10) USING left(code, 10);": evidence.High,
		"ALTER TABLE orders ALTER COLUMN created TYPE timestamptz;":                   evidence.Medium,
	}
	for sql, want := range cases {
		got := findings(t, "PS105", Options{}, sql)
		expectCount(t, got, 1)
		if got[0].Confidence != want {
			t.Errorf("%s: confidence = %s, want %s", sql, got[0].Confidence, want)
		}
	}
}

func TestPS106SetNotNull(t *testing.T) {
	got := findings(t, "PS106", Options{}, "ALTER TABLE orders ALTER COLUMN status SET NOT NULL;")
	expectCount(t, got, 1)
	if got[0].Remediation == "" {
		t.Fatal("expected remediation")
	}
}
