package fixture

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
)

// Hash identifies a fixture: two runs with the same hash seeded byte-identical
// databases (given the same PostgreSQL version).
type Hash struct{ h hash.Hash }

// NewHash starts a fixture hash.
func NewHash() *Hash { return &Hash{h: sha256.New()} }

// Add mixes a named input into the hash. Names keep "schema" and "seed"
// with the same content distinguishable.
func (h *Hash) Add(name string, content []byte) {
	fmt.Fprintf(h.h, "%s\x00%d\x00", name, len(content))
	h.h.Write(content)
}

// AddSeed mixes the generator seed into the hash.
func (h *Hash) AddSeed(seed int64) { fmt.Fprintf(h.h, "seed\x00%d\x00", seed) }

// String returns "sha256:" followed by 16 hex characters.
func (h *Hash) String() string {
	return "sha256:" + hex.EncodeToString(h.h.Sum(nil)[:8])
}
