package pglock

import (
	"encoding/json"
	"testing"
)

func TestConflictMatrixIsSymmetric(t *testing.T) {
	for a := AccessShare; a <= AccessExclusive; a++ {
		for b := AccessShare; b <= AccessExclusive; b++ {
			if Conflicts(a, b) != Conflicts(b, a) {
				t.Errorf("asymmetric conflict between %s and %s", a, b)
			}
		}
	}
}

func TestDocumentedConflicts(t *testing.T) {
	cases := []struct {
		a, b Mode
		want bool
	}{
		{AccessShare, AccessShare, false},
		{AccessShare, Exclusive, false},
		{AccessShare, AccessExclusive, true},
		{RowExclusive, RowExclusive, false},
		{RowExclusive, Share, true},
		{ShareUpdateExclusive, ShareUpdateExclusive, true},
		{Share, Share, false},
		{ShareRowExclusive, ShareRowExclusive, true},
		{RowShare, Exclusive, true},
		{RowShare, Share, false},
	}
	for _, c := range cases {
		if got := Conflicts(c.a, c.b); got != c.want {
			t.Errorf("Conflicts(%s, %s) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestImpact(t *testing.T) {
	cases := map[Mode]Impact{
		None:                 ImpactNone,
		AccessShare:          ImpactNone,
		RowExclusive:         ImpactNone,
		ShareUpdateExclusive: ImpactDDLOnly,
		Share:                ImpactWrites,
		ShareRowExclusive:    ImpactWrites,
		Exclusive:            ImpactWrites,
		AccessExclusive:      ImpactAllTraffic,
	}
	for m, want := range cases {
		if got := m.Impact(); got != want {
			t.Errorf("%s.Impact() = %s, want %s", m, got, want)
		}
	}
}

func TestParseBothSpellings(t *testing.T) {
	for m := AccessShare; m <= AccessExclusive; m++ {
		for _, s := range []string{m.String(), m.PgLocksName(), " " + m.PgLocksName() + " "} {
			got, err := Parse(s)
			if err != nil || got != m {
				t.Errorf("Parse(%q) = %v, %v", s, got, err)
			}
		}
	}
	if _, err := Parse("SuperLock"); err == nil {
		t.Error("expected error for unknown mode")
	}
}

func TestJSONRoundTrip(t *testing.T) {
	b, err := json.Marshal(struct{ M Mode }{ShareRowExclusive})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"M":"SHARE ROW EXCLUSIVE"}` {
		t.Fatalf("got %s", b)
	}
	var out struct{ M Mode }
	if err := json.Unmarshal(b, &out); err != nil || out.M != ShareRowExclusive {
		t.Fatalf("round trip: %v %v", out.M, err)
	}
}

func TestStrongestAndString(t *testing.T) {
	if Strongest() != None || Strongest(Share, AccessShare, Exclusive) != Exclusive {
		t.Fatal("Strongest returned the wrong mode")
	}
	if Mode(42).String() != "Mode(42)" || Mode(42).Valid() {
		t.Fatal("out-of-range mode handling")
	}
}

func TestBlockedStatements(t *testing.T) {
	share := Share.BlockedStatements()
	if len(share) == 0 || share[0] != "INSERT/UPDATE/DELETE/MERGE" {
		t.Fatalf("SHARE should block writes first: %v", share)
	}
	for _, s := range share {
		if s == "SELECT" {
			t.Fatal("SHARE must not block plain reads")
		}
	}
	all := AccessExclusive.BlockedStatements()
	if len(all) != 8 || all[0] != "SELECT" {
		t.Fatalf("ACCESS EXCLUSIVE blocks everything: %v", all)
	}
	if len(AccessShare.BlockedStatements()) != 1 {
		t.Fatalf("ACCESS SHARE only conflicts with ACCESS EXCLUSIVE: %v", AccessShare.BlockedStatements())
	}
}

func TestModeReference(t *testing.T) {
	if got := Modes(); len(got) != 8 || got[0] != AccessShare || got[7] != AccessExclusive {
		t.Fatalf("Modes() = %v", got)
	}
	if got := Share.AcquiredBy(); len(got) != 1 || got[0] != "CREATE INDEX" {
		t.Fatalf("Share.AcquiredBy() = %v", got)
	}
	if got := RowShare.AcquiredBy(); len(got) != 1 {
		t.Fatalf("RowShare.AcquiredBy() = %v", got)
	}
	if got := AccessShare.ConflictingModes(); len(got) != 1 || got[0] != AccessExclusive {
		t.Fatalf("AccessShare conflicts with %v", got)
	}
	if got := AccessExclusive.ConflictingModes(); len(got) != 8 {
		t.Fatalf("AccessExclusive conflicts with %v", got)
	}
	// The conflict relation is symmetric.
	for _, a := range Modes() {
		for _, b := range a.ConflictingModes() {
			if !Conflicts(b, a) {
				t.Errorf("%s conflicts with %s but not the reverse", a, b)
			}
		}
	}
}
