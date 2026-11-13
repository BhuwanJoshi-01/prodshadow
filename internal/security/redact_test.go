package security

import (
	"strings"
	"testing"
)

func TestRedactBuiltinPatterns(t *testing.T) {
	r, err := NewRedactor(nil)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"postgres://app:hunter2secret@db:5432/app":                 "hunter2secret",
		"host=/tmp user=app password='s3cr3t-pass' dbname=x":       "s3cr3t-pass",
		"PGPASSWORD=topsecret123 psql":                             "topsecret123",
		"Authorization: Bearer abcdefghijklmnopqrstuvwxyz":         "abcdefghijklmnopqrstuvwxyz",
		"key AKIAABCDEFGHIJKLMNOP leaked":                          "AKIAABCDEFGHIJKLMNOP",
		"token ghp_" + strings.Repeat("a", 36):                     "ghp_",
		"jwt eyJhbGciOiJIUzI1.eyJzdWIiOiIxMjM0.SflKxwRJSMeKKF2QT4": "eyJhbGciOiJIUzI1",
	}
	for in, secret := range cases {
		out := r.String(in)
		if strings.Contains(out, secret) {
			t.Errorf("secret %q survived redaction: %q", secret, out)
		}
		if !strings.Contains(out, Redacted) {
			t.Errorf("no redaction marker in %q", out)
		}
	}
}

func TestRedactKeepsURLStructure(t *testing.T) {
	r, _ := NewRedactor(nil)
	got := r.String("postgres://app:pw123456@localhost/db")
	if got != "postgres://app:"+Redacted+"@localhost/db" {
		t.Fatalf("got %q", got)
	}
}

func TestRedactExactAndUserPatterns(t *testing.T) {
	r, err := NewRedactor([]string{`customer-\d+`})
	if err != nil {
		t.Fatal(err)
	}
	r.Add("ephemeral-run-token-abc", "short")
	got := r.String("token ephemeral-run-token-abc for customer-4411; short words stay")
	if strings.Contains(got, "ephemeral-run-token-abc") || strings.Contains(got, "customer-4411") {
		t.Fatalf("secret survived: %q", got)
	}
	if !strings.Contains(got, "short words stay") {
		t.Fatalf("short values must not be redacted: %q", got)
	}
	if _, err := NewRedactor([]string{"("}); err == nil {
		t.Fatal("invalid user pattern must be rejected")
	}
}

func TestNilRedactorIsSafe(t *testing.T) {
	var r *Redactor
	r.Add("ignored-secret")
	if r.String("x") != "x" {
		t.Fatal("nil redactor should pass text through")
	}
}

func TestHashIdentifier(t *testing.T) {
	a, b := HashIdentifier("SELECT 1"), HashIdentifier("SELECT 2")
	if a == b || !strings.HasPrefix(a, "sha256:") || len(a) != len("sha256:")+16 {
		t.Fatalf("unexpected hashes %q %q", a, b)
	}
	if HashIdentifier("SELECT 1") != a {
		t.Fatal("hash must be deterministic")
	}
}
