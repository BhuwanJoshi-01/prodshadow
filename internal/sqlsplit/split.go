// Package sqlsplit splits a SQL script into statements while tracking exact
// byte offsets and line numbers.
//
// The splitter understands the lexical constructs that may contain
// semicolons without ending a statement: single-quoted and escape strings,
// quoted identifiers, dollar-quoted bodies, line comments, nested block
// comments and SQL-standard "BEGIN ATOMIC ... END" function bodies. It does
// not interpret psql meta-commands; a statement starting with a backslash is
// reported as unsupported.
package sqlsplit

import (
	"errors"
	"fmt"
	"strings"
)

// Statement is one statement of a script.
type Statement struct {
	// Text is the statement without its terminating semicolon and without
	// leading comments or whitespace.
	Text string
	// Start and End are byte offsets of Text within the script.
	Start, End int
	// StartLine and EndLine are 1-based line numbers of Text.
	StartLine, EndLine int
	// LeadingComments are comment lines directly before the statement,
	// with comment markers removed. They carry inline annotations.
	LeadingComments []string
}

// ErrUnterminated is returned when a string, identifier, comment or
// dollar-quoted body is not closed before the end of input.
var ErrUnterminated = errors.New("unterminated literal or comment")

// ErrMetaCommand is returned for psql backslash commands.
var ErrMetaCommand = errors.New("psql meta-commands are not supported")

// SyntaxError carries the location of a lexical error.
type SyntaxError struct {
	Line int
	Err  error
}

func (e *SyntaxError) Error() string { return fmt.Sprintf("line %d: %v", e.Line, e.Err) }

func (e *SyntaxError) Unwrap() error { return e.Err }

// Split returns the non-empty statements of script.
func Split(script string) ([]Statement, error) {
	s := &scanner{src: script, line: 1}
	return s.run()
}

type scanner struct {
	src  string
	pos  int
	line int
}

func (s *scanner) run() ([]Statement, error) {
	var out []Statement
	for {
		comments, err := s.skipTrivia()
		if err != nil {
			return out, err
		}
		if s.pos >= len(s.src) {
			return out, nil
		}
		if s.src[s.pos] == ';' {
			s.pos++
			continue
		}
		if s.src[s.pos] == '\\' {
			return out, &SyntaxError{Line: s.line, Err: ErrMetaCommand}
		}
		st, err := s.statement()
		if err != nil {
			return out, err
		}
		st.LeadingComments = comments
		out = append(out, st)
	}
}

// skipTrivia advances over whitespace and comments, collecting comment text.
func (s *scanner) skipTrivia() ([]string, error) {
	var comments []string
	for s.pos < len(s.src) {
		c := s.src[s.pos]
		switch {
		case c == '\n':
			s.line++
			s.pos++
		case c == ' ' || c == '\t' || c == '\r' || c == '\f':
			s.pos++
		case strings.HasPrefix(s.src[s.pos:], "--"):
			end := strings.IndexByte(s.src[s.pos:], '\n')
			if end < 0 {
				end = len(s.src) - s.pos
			}
			comments = append(comments, strings.TrimSpace(s.src[s.pos+2:s.pos+end]))
			s.pos += end
		case strings.HasPrefix(s.src[s.pos:], "/*"):
			start := s.pos
			if err := s.blockComment(); err != nil {
				return comments, err
			}
			body := strings.TrimSuffix(strings.TrimPrefix(s.src[start:s.pos], "/*"), "*/")
			comments = append(comments, strings.TrimSpace(body))
		default:
			return comments, nil
		}
	}
	return comments, nil
}

// statement scans until a top-level semicolon or end of input.
func (s *scanner) statement() (Statement, error) {
	st := Statement{Start: s.pos, StartLine: s.line}
	lastSignificant := s.pos
	lastLine := s.line
	atomicDepth, caseDepth := 0, 0
	var prevWord string

	for s.pos < len(s.src) {
		c := s.src[s.pos]
		switch {
		case c == ';' && atomicDepth == 0:
			st.End, st.EndLine = lastSignificant, lastLine
			st.Text = s.src[st.Start:st.End]
			s.pos++
			return st, nil
		case c == '\n':
			s.line++
			s.pos++
			continue
		case c == ' ' || c == '\t' || c == '\r' || c == '\f':
			s.pos++
			continue
		case strings.HasPrefix(s.src[s.pos:], "--"):
			end := strings.IndexByte(s.src[s.pos:], '\n')
			if end < 0 {
				end = len(s.src) - s.pos
			}
			s.pos += end
			continue
		case strings.HasPrefix(s.src[s.pos:], "/*"):
			if err := s.blockComment(); err != nil {
				return st, err
			}
			continue
		case c == '\'':
			escape := s.pos > 0 && (s.src[s.pos-1] == 'E' || s.src[s.pos-1] == 'e') && !isIdentChar(s.byteAt(s.pos-2))
			if err := s.quoted('\'', escape); err != nil {
				return st, err
			}
		case c == '"':
			if err := s.quoted('"', false); err != nil {
				return st, err
			}
		case c == '$':
			if tag, ok := s.dollarTag(); ok {
				if err := s.dollarBody(tag); err != nil {
					return st, err
				}
			} else {
				s.pos++
			}
		case isIdentStart(c):
			start := s.pos
			for s.pos < len(s.src) && isIdentChar(s.src[s.pos]) {
				s.pos++
			}
			word := strings.ToUpper(s.src[start:s.pos])
			switch {
			case word == "ATOMIC" && prevWord == "BEGIN":
				atomicDepth++
			case atomicDepth > 0 && word == "CASE":
				caseDepth++
			case atomicDepth > 0 && word == "END":
				if caseDepth > 0 {
					caseDepth--
				} else {
					atomicDepth--
				}
			}
			prevWord = word
			lastSignificant, lastLine = s.pos, s.line
			continue
		default:
			s.pos++
		}
		lastSignificant, lastLine = s.pos, s.line
	}
	if atomicDepth > 0 {
		return st, &SyntaxError{Line: st.StartLine, Err: fmt.Errorf("%w: BEGIN ATOMIC without END", ErrUnterminated)}
	}
	st.End, st.EndLine = lastSignificant, lastLine
	st.Text = s.src[st.Start:st.End]
	return st, nil
}

func (s *scanner) byteAt(i int) byte {
	if i < 0 || i >= len(s.src) {
		return 0
	}
	return s.src[i]
}

// blockComment consumes a possibly nested /* ... */ comment.
func (s *scanner) blockComment() error {
	startLine := s.line
	depth := 0
	for s.pos < len(s.src) {
		switch {
		case strings.HasPrefix(s.src[s.pos:], "/*"):
			depth++
			s.pos += 2
		case strings.HasPrefix(s.src[s.pos:], "*/"):
			depth--
			s.pos += 2
			if depth == 0 {
				return nil
			}
		default:
			if s.src[s.pos] == '\n' {
				s.line++
			}
			s.pos++
		}
	}
	return &SyntaxError{Line: startLine, Err: fmt.Errorf("%w: block comment", ErrUnterminated)}
}

// quoted consumes a quoted string or identifier. Doubled quotes escape the
// quote character; backslashes escape characters in E” strings.
func (s *scanner) quoted(q byte, backslash bool) error {
	startLine := s.line
	s.pos++
	for s.pos < len(s.src) {
		c := s.src[s.pos]
		switch {
		case c == '\n':
			s.line++
			s.pos++
		case backslash && c == '\\':
			if s.byteAt(s.pos+1) == '\n' {
				s.line++
			}
			s.pos += 2
		case c == q:
			if s.byteAt(s.pos+1) == q {
				s.pos += 2
				continue
			}
			s.pos++
			return nil
		default:
			s.pos++
		}
	}
	kind := "string"
	if q == '"' {
		kind = "quoted identifier"
	}
	return &SyntaxError{Line: startLine, Err: fmt.Errorf("%w: %s", ErrUnterminated, kind)}
}

// dollarTag recognises $tag$ or $$ at the current position. A '$' that is
// part of an identifier or a positional parameter ($1) is not a tag.
func (s *scanner) dollarTag() (string, bool) {
	if s.pos > 0 && isIdentChar(s.src[s.pos-1]) {
		return "", false
	}
	end := s.pos + 1
	for end < len(s.src) && s.src[end] != '$' {
		if !isIdentChar(s.src[end]) {
			return "", false
		}
		end++
	}
	if end >= len(s.src) {
		return "", false
	}
	tag := s.src[s.pos : end+1]
	if len(tag) > 2 && tag[1] >= '0' && tag[1] <= '9' {
		return "", false
	}
	return tag, true
}

func (s *scanner) dollarBody(tag string) error {
	startLine := s.line
	s.pos += len(tag)
	idx := strings.Index(s.src[s.pos:], tag)
	if idx < 0 {
		return &SyntaxError{Line: startLine, Err: fmt.Errorf("%w: dollar-quoted body %s", ErrUnterminated, tag)}
	}
	s.line += strings.Count(s.src[s.pos:s.pos+idx], "\n")
	s.pos += idx + len(tag)
	return nil
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}

func isIdentChar(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9') || c == '$'
}
