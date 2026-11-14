package sqlinspect

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/sqlsplit"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

// ParserGrammar names the PostgreSQL grammar bundled with the parser. SQL
// that only newer servers accept may fail to parse; it is still executed in
// the disposable database.
const ParserGrammar = "PostgreSQL 17 (libpg_query via pg_query_go v6)"

// MaxSourceSize bounds a single migration file.
const MaxSourceSize = 16 << 20

// Source is one migration file.
type Source struct {
	// Path is the path shown in reports, usually relative to the project.
	Path    string
	Content string
}

// ReadSource loads a migration file, enforcing size and encoding limits.
func ReadSource(path, display string) (Source, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return Source{}, err
	}
	if !fi.Mode().IsRegular() {
		return Source{}, fmt.Errorf("%s is not a regular file", display)
	}
	if fi.Size() > MaxSourceSize {
		return Source{}, fmt.Errorf("%s is larger than %d bytes", display, MaxSourceSize)
	}
	f, err := os.Open(path)
	if err != nil {
		return Source{}, err
	}
	defer f.Close()
	return ReadSourceFrom(f, display)
}

// ReadSourceFrom loads a migration from r, for example standard input,
// enforcing the same size and encoding limits as ReadSource.
func ReadSourceFrom(r io.Reader, display string) (Source, error) {
	b, err := io.ReadAll(io.LimitReader(r, MaxSourceSize+1))
	if err != nil {
		return Source{}, fmt.Errorf("reading %s: %w", display, err)
	}
	if len(b) > MaxSourceSize {
		return Source{}, fmt.Errorf("%s is larger than %d bytes", display, MaxSourceSize)
	}
	if !utf8.Valid(b) {
		return Source{}, fmt.Errorf("%s is not valid UTF-8", display)
	}
	return Source{Path: display, Content: strings.TrimPrefix(string(b), "\uFEFF")}, nil
}

// SourceError reports a file that could not be split into statements.
type SourceError struct {
	File string
	Err  error
}

func (e *SourceError) Error() string { return e.File + ": " + e.Err.Error() }

func (e *SourceError) Unwrap() error { return e.Err }

// ParseSources splits and parses every source in order. Statement IDs are
// "<file>.<n>" with both numbers starting at 1. A file that cannot be split
// stops parsing, because statement boundaries — and therefore the
// statements that would be executed — are unknown.
func ParseSources(sources []Source) ([]*Statement, error) {
	var out []*Statement
	for fi, src := range sources {
		chunks, err := sqlsplit.Split(src.Content)
		if err != nil {
			return out, &SourceError{File: src.Path, Err: err}
		}
		for si, ch := range chunks {
			st := &Statement{
				ID:          fmt.Sprintf("%d.%d", fi+1, si+1),
				File:        src.Path,
				StartLine:   ch.StartLine,
				EndLine:     ch.EndLine,
				SQL:         ch.Text,
				Annotations: parseAnnotations(ch.LeadingComments),
			}
			parseStatement(st)
			out = append(out, st)
		}
	}
	return out, nil
}

func parseStatement(st *Statement) {
	res, err := pg_query.Parse(st.SQL)
	if err != nil {
		st.ParseError = err.Error()
		classify(st)
		return
	}
	if len(res.Stmts) != 1 {
		st.ParseError = fmt.Sprintf("expected one statement, parser found %d", len(res.Stmts))
		classify(st)
		return
	}
	st.Parsed = true
	st.node = res.Stmts[0].Stmt
	if fp, err := pg_query.Fingerprint(st.SQL); err == nil {
		st.Fingerprint = fp
	}
	classify(st)
}
