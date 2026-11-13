// Package gen provides deterministic value generators used for fixture rows
// and workload query parameters. Every generator draws from a named random
// stream derived from the experiment seed, so adding a column or scenario
// does not change the values produced for the others.
package gen

import (
	"hash/fnv"
	"math/rand/v2"
)

// Stream returns a random source for name under seed. Equal seeds and names
// always produce identical sequences.
func Stream(seed int64, name string) *rand.Rand {
	h := fnv.New64a()
	_, _ = h.Write([]byte(name))
	return rand.New(rand.NewPCG(uint64(seed), h.Sum64()))
}
