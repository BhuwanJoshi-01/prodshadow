package config

import "testing"

// FuzzParse checks that arbitrary documents never panic the decoder, the
// key index or validation.
func FuzzParse(f *testing.F) {
	f.Add([]byte(sampleConfig))
	f.Add([]byte("version: 1\nproject: x\npostgres: {settings: {a: b}}\n"))
	f.Add([]byte("a: &x [*x]\n"))
	f.Add([]byte("policy:\n  rules:\n    PS101: off\n  max_lock_wait_ms: -1\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		cfg, _, err := Parse(data, t.TempDir())
		if err != nil || cfg == nil {
			return
		}
		_ = cfg.Validate()
	})
}
