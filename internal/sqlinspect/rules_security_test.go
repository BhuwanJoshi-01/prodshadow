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

func TestPS145GrantedToPublic(t *testing.T) {
	cases := map[string]int{
		"GRANT SELECT ON customers TO PUBLIC;":                                        1,
		"GRANT USAGE ON SCHEMA reporting TO PUBLIC;":                                  1,
		"GRANT SELECT ON ALL TABLES IN SCHEMA public TO reporting, PUBLIC;":           1,
		"ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO PUBLIC;": 1,
		"GRANT SELECT ON customers TO reporting;":                                     0,
		"REVOKE ALL ON customers FROM PUBLIC;":                                        0,
	}
	for sql, want := range cases {
		if got := findings(t, "PS145", Options{}, sql); len(got) != want {
			t.Errorf("%s: expected %d findings, got %d", sql, want, len(got))
		}
	}
}
