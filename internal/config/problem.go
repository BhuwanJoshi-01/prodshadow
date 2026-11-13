package config

import (
	"fmt"
	"sort"
	"strings"
)

// Level classifies a configuration problem.
type Level string

const (
	// LevelError makes the configuration unusable.
	LevelError Level = "error"
	// LevelWarning is reported but does not prevent a run.
	LevelWarning Level = "warning"
)

// Problem is a single validation result tied to a configuration path.
type Problem struct {
	Level   Level  `json:"level"`
	Path    string `json:"path"`
	Message string `json:"message"`
	// Line is the 1-based line in the source file, when known.
	Line int `json:"line,omitempty"`
}

func (p Problem) String() string {
	loc := p.Path
	if p.Line > 0 {
		loc = fmt.Sprintf("%s (line %d)", p.Path, p.Line)
	}
	return fmt.Sprintf("%s: %s: %s", p.Level, loc, p.Message)
}

// Problems is an ordered collection of validation results.
type Problems []Problem

// HasErrors reports whether any problem is an error.
func (ps Problems) HasErrors() bool {
	for _, p := range ps {
		if p.Level == LevelError {
			return true
		}
	}
	return false
}

// Errors returns only error-level problems.
func (ps Problems) Errors() Problems { return ps.filter(LevelError) }

// Warnings returns only warning-level problems.
func (ps Problems) Warnings() Problems { return ps.filter(LevelWarning) }

func (ps Problems) filter(l Level) Problems {
	var out Problems
	for _, p := range ps {
		if p.Level == l {
			out = append(out, p)
		}
	}
	return out
}

// Sorted returns problems ordered by level (errors first), then path.
func (ps Problems) Sorted() Problems {
	out := append(Problems(nil), ps...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Level != out[j].Level {
			return out[i].Level == LevelError
		}
		return out[i].Path < out[j].Path
	})
	return out
}

// Error renders all problems so Problems can be returned as an error.
func (ps Problems) Error() string {
	lines := make([]string, 0, len(ps))
	for _, p := range ps.Sorted() {
		lines = append(lines, p.String())
	}
	return strings.Join(lines, "\n")
}

// StrictErrors converts warnings to errors, for --strict validation.
func (ps Problems) StrictErrors() Problems {
	out := make(Problems, len(ps))
	for i, p := range ps {
		p.Level = LevelError
		out[i] = p
	}
	return out
}

func (ps *Problems) errorf(path, format string, args ...any) {
	*ps = append(*ps, Problem{Level: LevelError, Path: path, Message: fmt.Sprintf(format, args...)})
}

func (ps *Problems) warnf(path, format string, args ...any) {
	*ps = append(*ps, Problem{Level: LevelWarning, Path: path, Message: fmt.Sprintf(format, args...)})
}
