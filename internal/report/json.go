package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// JSON renders the report with stable indentation.
func (r *Report) JSON() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(r); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Read loads a report produced by this or an earlier compatible version.
// Reports from a newer major schema version are rejected rather than
// misread.
func Read(rd io.Reader) (*Report, error) {
	var r Report
	dec := json.NewDecoder(rd)
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("decode report: %w", err)
	}
	if major(r.SchemaVersion) != major(SchemaVersion) {
		return nil, fmt.Errorf("report schema %s is not compatible with this build (%s)", r.SchemaVersion, SchemaVersion)
	}
	return &r, nil
}

// ReadFile loads a report from disk.
func ReadFile(path string) (*Report, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return Read(f)
}

func major(v string) string {
	m, _, _ := strings.Cut(v, ".")
	return m
}
