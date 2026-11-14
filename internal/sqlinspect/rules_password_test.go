package sqlinspect

import (
	"strings"
	"testing"
)

func TestPS139PlaintextPassword(t *testing.T) {
	cases := map[string]int{
		"CREATE ROLE app LOGIN PASSWORD 'correct-horse';":                                    1,
		"CREATE USER reporting WITH PASSWORD 'battery-staple';":                              1,
		"ALTER ROLE app WITH PASSWORD 'new-secret';":                                         1,
		"ALTER ROLE app PASSWORD NULL;":                                                      0,
		"CREATE ROLE app LOGIN PASSWORD 'SCRAM-SHA-256$4096:c2FsdA==$a2V5:c2VydmVy';":        0,
		"CREATE ROLE app LOGIN PASSWORD 'md5" + strings.Repeat("a", 32) + "';":               0,
		"CREATE USER MAPPING FOR app SERVER warehouse OPTIONS (user 'etl', password 'pw1');": 1,
		"ALTER USER MAPPING FOR app SERVER warehouse OPTIONS (SET password 'pw2');":          1,
		"CREATE USER MAPPING FOR app SERVER warehouse OPTIONS (user 'etl');":                 0,
		"CREATE ROLE app LOGIN;":                                                             0,
	}
	for sql, want := range cases {
		got := findings(t, "PS139", Options{}, sql)
		if len(got) != want {
			t.Errorf("%s: expected %d findings, got %d", sql, want, len(got))
		}
		for _, f := range got {
			for _, secret := range []string{"correct-horse", "battery-staple", "new-secret", "pw1", "pw2"} {
				if strings.Contains(f.Message, secret) {
					t.Errorf("finding repeats the password: %s", f.Message)
				}
			}
		}
	}
}
