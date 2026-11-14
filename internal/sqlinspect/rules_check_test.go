package sqlinspect

import (
	"strings"
	"testing"
)

func TestPS151NonImmutableCheck(t *testing.T) {
	cases := map[string]string{
		"ALTER TABLE orders ADD CONSTRAINT not_future CHECK (created_at <= now()) NOT VALID;":        "now()",
		"ALTER TABLE orders ADD CONSTRAINT recent CHECK (ordered_on > CURRENT_DATE - 30) NOT VALID;": "CURRENT_DATE",
		"CREATE TABLE notes (author text CHECK (author = CURRENT_USER));":                            "CURRENT_USER",
		"CREATE TABLE tokens (id int, CONSTRAINT expiry CHECK (expires_at > clock_timestamp()));":    "clock_timestamp()",
	}
	for sql, want := range cases {
		got := findings(t, "PS151", Options{}, sql)
		if len(got) != 1 || !strings.Contains(got[0].Message, want) {
			t.Errorf("%s: want one finding naming %q, got %+v", sql, want, got)
		}
	}
	for _, sql := range []string{
		"ALTER TABLE orders ADD CONSTRAINT positive CHECK (total >= 0) NOT VALID;",
		"ALTER TABLE orders ADD CONSTRAINT lower_code CHECK (code = lower(code)) NOT VALID;",
		"CREATE TABLE events (at timestamptz DEFAULT now());",
	} {
		expectCount(t, findings(t, "PS151", Options{}, sql), 0)
	}
}
