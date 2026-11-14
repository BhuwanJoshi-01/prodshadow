package sqlinspect

import "testing"

func TestPS144RowSecurityDisabled(t *testing.T) {
	cases := map[string]int{
		"ALTER TABLE accounts DISABLE ROW LEVEL SECURITY;":  1,
		"ALTER TABLE accounts NO FORCE ROW LEVEL SECURITY;": 1,
		"DROP POLICY tenant_isolation ON accounts;":         1,
		"ALTER TABLE accounts ENABLE ROW LEVEL SECURITY;":   0,
		"ALTER TABLE accounts FORCE ROW LEVEL SECURITY;":    0,
	}
	for sql, want := range cases {
		if got := findings(t, "PS144", Options{}, sql); len(got) != want {
			t.Errorf("%s: expected %d findings, got %d", sql, want, len(got))
		}
	}
}
