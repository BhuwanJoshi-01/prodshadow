package isolation

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// runner executes a program with an argument vector. It never goes through
// a shell, so configuration values cannot inject commands.
type runner func(ctx context.Context, name string, args ...string) (stdout, stderr []byte, err error)

func execRunner(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	// name is the configured container CLI or a PostgreSQL binary; args is
	// an argument vector, so no shell ever interprets configuration values.
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // see comment above
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	return out.Bytes(), errOut.Bytes(), err
}

// commandError formats a failed command without echoing its arguments,
// which may contain secrets such as environment assignments.
func commandError(name, verb string, stderr []byte, err error) error {
	msg := strings.TrimSpace(string(stderr))
	if len(msg) > 2000 {
		msg = msg[:2000] + "..."
	}
	if msg == "" {
		return fmt.Errorf("%s %s: %w", name, verb, err)
	}
	return fmt.Errorf("%s %s: %w: %s", name, verb, err, msg)
}
