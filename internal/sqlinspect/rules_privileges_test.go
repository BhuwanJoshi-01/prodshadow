package sqlinspect

import (
	"strings"
	"testing"
)

func TestPS138PrivilegesRevoked(t *testing.T) {
	cases := map[string]string{
		"REVOKE SELECT ON orders FROM reporting;":                                           "REVOKE on table from reporting",
		"REVOKE ALL ON ALL TABLES IN SCHEMA public FROM web, worker;":                       "all tables in schema from web, worker",
		"REVOKE EXECUTE ON FUNCTION order_total(bigint) FROM PUBLIC;":                       "from PUBLIC",
		"ALTER DEFAULT PRIVILEGES IN SCHEMA public REVOKE SELECT ON TABLES FROM reporting;": "objects created later",
		"DROP OWNED BY legacy_app;":                                                         "DROP OWNED BY legacy_app",
		"DROP ROLE legacy_app;":                                                             "cluster-wide",
	}
	for sql, want := range cases {
		got := findings(t, "PS138", Options{}, sql)
		expectCount(t, got, 1)
		if !strings.Contains(got[0].Message, want) {
			t.Errorf("%s: message %q lacks %q", sql, got[0].Message, want)
		}
	}
	for _, sql := range []string{"GRANT SELECT ON orders TO reporting;", "ALTER DEFAULT PRIVILEGES GRANT SELECT ON TABLES TO reporting;"} {
		expectCount(t, findings(t, "PS138", Options{}, sql), 0)
	}
}
