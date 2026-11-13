package security

import (
	"errors"
	"testing"
)

func TestCheckLocalAddress(t *testing.T) {
	allowed := []string{"/tmp/prodshadow-123/socket", "127.0.0.1", "127.8.9.10", "::1", "[::1]"}
	for _, h := range allowed {
		if err := CheckLocalAddress(h); err != nil {
			t.Errorf("%q should be allowed: %v", h, err)
		}
	}
	denied := []string{"", "localhost", "db.internal", "10.0.0.5", "203.0.113.7", "0.0.0.0", "::", "/tmp/../etc", "relative/socket", "prod-db.example.com"}
	for _, h := range denied {
		if err := CheckLocalAddress(h); !errors.Is(err, ErrUnmanagedTarget) {
			t.Errorf("%q should be rejected, got %v", h, err)
		}
	}
}
