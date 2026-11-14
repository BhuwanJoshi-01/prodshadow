package cli

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// changedMigrations narrows the migration files (after glob expansion) to
// those git reports as added, modified or renamed between ref and HEAD,
// keeping their order. It returns absolute paths.
func changedMigrations(base string, files []string, ref string) ([]string, error) {
	// A ref starting with "-" would be read as a git option, e.g.
	// --output=FILE.
	if ref == "" || strings.HasPrefix(ref, "-") || strings.ContainsAny(ref, " \t\n") {
		return nil, fmt.Errorf("invalid ref %q", ref)
	}
	paths, err := expandMigrations(base, files)
	if err != nil {
		return nil, err
	}
	root, err := git(base, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	root = strings.TrimSpace(root)
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	// ref...HEAD compares with the merge base, so changes made on the base
	// branch after the branch point are not counted.
	out, err := git(root, "diff", "--name-only", "--diff-filter=AMR", ref+"...HEAD")
	if err != nil {
		return nil, err
	}
	changed := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			changed[filepath.Clean(filepath.Join(root, filepath.FromSlash(line)))] = true
		}
	}
	var keep []string
	for _, p := range paths {
		abs := p
		if resolved, err := filepath.EvalSymlinks(p); err == nil {
			abs = resolved
		}
		if changed[filepath.Clean(abs)] {
			keep = append(keep, p)
		}
	}
	return keep, nil
}

// git runs a git command in dir and returns its standard output.
func git(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// Arguments are passed as a vector, never through a shell, and the only
	// user-supplied one (the ref) is validated by changedMigrations.
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...) //nolint:gosec // see above
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", args[0], msg)
	}
	return stdout.String(), nil
}
