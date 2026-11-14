package sqlinspect

import (
	"strings"
	"testing"
)

func TestPS129ServerSettingChange(t *testing.T) {
	cases := map[string]string{
		"ALTER SYSTEM SET max_connections = 500;":                     "max_connections",
		"ALTER SYSTEM RESET ALL;":                                     "settings",
		"ALTER DATABASE app SET statement_timeout = '5s';":            "ALTER DATABASE app SET changes statement_timeout",
		"ALTER ROLE web SET work_mem = '64MB';":                       "ALTER ROLE web SET changes work_mem",
		"ALTER ROLE ALL SET idle_in_transaction_session_timeout = 0;": "ALTER ROLE ALL",
	}
	for sql, want := range cases {
		got := findings(t, "PS129", Options{}, sql)
		expectCount(t, got, 1)
		if !strings.Contains(got[0].Message, want) {
			t.Errorf("%s: message %q lacks %q", sql, got[0].Message, want)
		}
	}
	for _, sql := range []string{"SET statement_timeout = '5s';", "SET LOCAL work_mem = '64MB';", "ALTER ROLE web WITH LOGIN;"} {
		expectCount(t, findings(t, "PS129", Options{}, sql), 0)
	}
}
