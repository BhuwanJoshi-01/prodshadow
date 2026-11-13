//go:build unix

package isolation

import (
	"os"
	"syscall"
)

// syscallZero returns signal 0, which checks whether a process exists.
func syscallZero() os.Signal { return syscall.Signal(0) }
