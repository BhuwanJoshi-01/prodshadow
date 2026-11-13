package sqlinspect

import (
	"testing"
	"time"
)

func TestParseTimeout(t *testing.T) {
	cases := map[string]time.Duration{
		"2000": 2 * time.Second, "2s": 2 * time.Second, "1500ms": 1500 * time.Millisecond,
		"1min": time.Minute, "0": 0, "1.5s": 1500 * time.Millisecond, " 3 s ": 3 * time.Second, "2h": 2 * time.Hour,
	}
	for in, want := range cases {
		got, ok := parseTimeout(in)
		if !ok || got != want {
			t.Errorf("parseTimeout(%q) = %v, %v; want %v", in, got, ok, want)
		}
	}
	for _, bad := range []string{"soon", "-1s", ""} {
		if _, ok := parseTimeout(bad); ok {
			t.Errorf("parseTimeout(%q) should fail", bad)
		}
	}
}

func TestPS120LockTimeoutValue(t *testing.T) {
	expectCount(t, findings(t, "PS120", Options{}, "SET lock_timeout = 0; ALTER TABLE t ADD c int;"), 1)
	expectCount(t, findings(t, "PS120", Options{}, "SET lock_timeout = '1min';"), 1)
	expectCount(t, findings(t, "PS120", Options{}, "SET LOCAL lock_timeout TO 30000;"), 1)
	expectCount(t, findings(t, "PS120", Options{}, "SET lock_timeout = '2s';"), 0)
	expectCount(t, findings(t, "PS120", Options{}, "SET statement_timeout = 0;"), 0)
	expectCount(t, findings(t, "PS120", Options{}, "RESET lock_timeout;"), 0)
}

func TestPS122DropIndex(t *testing.T) {
	expectCount(t, findings(t, "PS122", Options{}, "DROP INDEX orders_status_idx;"), 1)
	expectCount(t, findings(t, "PS122", Options{}, "DROP INDEX CONCURRENTLY orders_status_idx;"), 0)
}
