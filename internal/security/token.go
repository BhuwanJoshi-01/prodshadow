package security

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// RandomToken returns a hex string of n random bytes from crypto/rand. It is
// used for ephemeral passwords and the per-run ownership token.
func RandomToken(n int) (string, error) {
	if n < 16 {
		return "", fmt.Errorf("token length %d is too short; use at least 16 bytes", n)
	}
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// RunID returns a short random identifier for naming disposable resources.
func RunID() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return hex.EncodeToString(b), nil
}
