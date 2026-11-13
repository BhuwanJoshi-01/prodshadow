package security

import "testing"

func TestRandomToken(t *testing.T) {
	a, err := RandomToken(24)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := RandomToken(24)
	if len(a) != 48 || a == b {
		t.Fatalf("unexpected tokens %q %q", a, b)
	}
	if _, err := RandomToken(8); err == nil {
		t.Fatal("short tokens must be rejected")
	}
}

func TestRunID(t *testing.T) {
	id, err := RunID()
	if err != nil || len(id) != 12 {
		t.Fatalf("RunID = %q, %v", id, err)
	}
}
