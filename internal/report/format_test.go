package report

import (
	"math"
	"testing"
)

func TestFormatting(t *testing.T) {
	r := 2.345
	p := -12.34
	cv := 0.042
	cases := map[string]string{
		esc("a|b\nc"):        `a\|b c`,
		fmtMS(0.123):         "0.12 ms",
		fmtMS(12.34):         "12.3 ms",
		fmtMS(1500):          "1500 ms",
		fmtMS(15000):         "15.0 s",
		fmtMS(math.NaN()):    "–",
		fmtRatio(&r):         "2.35×",
		fmtRatio(nil):        "–",
		fmtPct(&p):           "-12.3%",
		fmtCV(&cv):           "4%",
		fmtInt(1234567):      "1,234,567",
		fmtInt(-1000):        "-1,000",
		fmtInt(12):           "12",
		fmtBytes(512):        "512 B",
		fmtBytes(3 << 20):    "3.0 MiB",
		fmtDuration(125.4):   "2m5s",
		location("m.sql", 4): "m.sql:4",
		location("", 0):      "–",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}
