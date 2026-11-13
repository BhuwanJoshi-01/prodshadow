package postgres

import "testing"

func TestIdentRejectsInjection(t *testing.T) {
	for _, bad := range []string{"", "Upper", "a b", `x"; DROP DATABASE prod; --`, "1abc"} {
		if _, err := ident(bad); err == nil {
			t.Errorf("ident(%q) should fail", bad)
		}
	}
	if got, err := ident("app_test"); err != nil || got != `"app_test"` {
		t.Fatalf("ident = %s, %v", got, err)
	}
}

func TestHashSettingsIsOrderIndependent(t *testing.T) {
	a := hashSettings(map[string]string{"a": "1", "b": "2"})
	b := hashSettings(map[string]string{"b": "2", "a": "1"})
	c := hashSettings(map[string]string{"a": "1", "b": "3"})
	if a != b || a == c {
		t.Fatalf("hashes: %s %s %s", a, b, c)
	}
}
