package sqlinspect

import (
	"strings"
	"testing"
)

func TestPS131ServerFileAccess(t *testing.T) {
	cases := map[string]string{
		"COPY countries FROM '/data/countries.csv' WITH (FORMAT csv);":     "reads the file \"/data/countries.csv\"",
		"COPY countries TO '/tmp/countries.csv';":                          "pg_write_server_files",
		"COPY countries FROM PROGRAM 'curl -s https://example.com/c.csv';": "PROGRAM runs",
		"SELECT pg_read_file('/etc/hostname');":                            "pg_read_file()",
		"INSERT INTO blobs (data) SELECT lo_import('/data/logo.png');":     "lo_import()",
	}
	for sql, want := range cases {
		got := findings(t, "PS131", Options{}, sql)
		expectCount(t, got, 1)
		if !strings.Contains(got[0].Message, want) {
			t.Errorf("%s: message %q lacks %q", sql, got[0].Message, want)
		}
	}
	for _, sql := range []string{"COPY countries FROM STDIN;", "COPY countries TO STDOUT;", "SELECT lower('x');"} {
		expectCount(t, findings(t, "PS131", Options{}, sql), 0)
	}
}
