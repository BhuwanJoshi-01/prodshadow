package sqlinspect

import (
	"strings"
	"testing"
)

func TestPS134UnbalancedTransactionControl(t *testing.T) {
	cases := []struct {
		files []string
		want  string
	}{
		{[]string{"BEGIN;\nALTER TABLE t ADD COLUMN c int;\nCOMMIT;\n"}, ""},
		{[]string{"BEGIN;\nALTER TABLE t ADD COLUMN c int;\n"}, "never committed"},
		{[]string{"ALTER TABLE t ADD COLUMN c int;\nCOMMIT;\n"}, "COMMIT without a BEGIN"},
		{[]string{"BEGIN;\nBEGIN;\nALTER TABLE t ADD COLUMN c int;\nCOMMIT;\n"}, "BEGIN inside the transaction opened by statement 1.1"},
		{[]string{"START TRANSACTION;\nUPDATE t SET c = 1 WHERE id = 1;\nROLLBACK;\n"}, ""},
		// Each file is checked on its own.
		{[]string{"BEGIN;\nUPDATE t SET c = 1 WHERE id = 1;\n", "COMMIT;\n"}, "never committed"},
	}
	for _, tc := range cases {
		got := findings(t, "PS134", Options{}, tc.files...)
		if tc.want == "" {
			expectCount(t, got, 0)
			continue
		}
		if len(got) == 0 || !strings.Contains(got[0].Message, tc.want) {
			t.Errorf("%q: want a finding containing %q, got %+v", tc.files, tc.want, got)
		}
	}
	if got := findings(t, "PS134", Options{}, "BEGIN;\nUPDATE t SET c = 1 WHERE id = 1;\n", "COMMIT;\n"); len(got) != 2 {
		t.Errorf("an open transaction in one file and a stray COMMIT in the next are two findings, got %+v", got)
	}
}
