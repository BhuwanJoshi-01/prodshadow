package security

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Redacted replaces secret material in artifacts.
const Redacted = "[REDACTED]"

// builtinPatterns match credentials that commonly leak into SQL text, error
// messages and connection strings.
var builtinPatterns = []*regexp.Regexp{
	// URL credentials: postgres://user:secret@host
	regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*://[^\s:/@]+:)[^\s@/]+@`),
	// key=value connection string passwords.
	regexp.MustCompile(`(?i)\b(password|passwd|pwd)\s*=\s*('[^']*'|"[^"]*"|\S+)`),
	// PGPASSWORD style environment assignments.
	regexp.MustCompile(`(?i)\b(PGPASSWORD|POSTGRES_PASSWORD)\s*[=:]\s*\S+`),
	// Bearer tokens.
	regexp.MustCompile(`(?i)\bbearer\s+[a-z0-9._~+/-]{12,}=*`),
	// AWS access key IDs.
	regexp.MustCompile(`\b(AKIA|ASIA)[A-Z0-9]{16}\b`),
	// GitHub tokens.
	regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{36,}\b`),
	// Slack tokens.
	regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{10,}\b`),
	// JSON Web Tokens.
	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`),
	// PEM private keys.
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`),
}

// Redactor removes secrets from text before it reaches an artifact. Exact
// secrets generated during a run (passwords, tokens) are registered with
// Add; pattern-based detection covers common credential formats.
type Redactor struct {
	mu       sync.RWMutex
	exact    []string
	patterns []*regexp.Regexp
}

// NewRedactor compiles user-defined patterns in addition to the built-ins.
func NewRedactor(userPatterns []string) (*Redactor, error) {
	r := &Redactor{patterns: append([]*regexp.Regexp(nil), builtinPatterns...)}
	for _, p := range userPatterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, err
		}
		r.patterns = append(r.patterns, re)
	}
	return r, nil
}

// Add registers exact secrets. Values shorter than 6 characters are ignored
// to avoid redacting common words.
func (r *Redactor) Add(secrets ...string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range secrets {
		if len(s) >= 6 {
			r.exact = append(r.exact, s)
		}
	}
	// Replace longer secrets first so overlapping values are fully removed.
	sort.Slice(r.exact, func(i, j int) bool { return len(r.exact[i]) > len(r.exact[j]) })
}

// String redacts s.
func (r *Redactor) String(s string) string {
	if r == nil || s == "" {
		return s
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, e := range r.exact {
		s = strings.ReplaceAll(s, e, Redacted)
	}
	for i, re := range r.patterns {
		switch i {
		case 0:
			s = re.ReplaceAllString(s, "${1}"+Redacted+"@")
		case 1:
			s = re.ReplaceAllString(s, "${1}="+Redacted)
		default:
			s = re.ReplaceAllString(s, Redacted)
		}
	}
	return s
}

// Strings redacts every element of ss in place and returns it.
func (r *Redactor) Strings(ss []string) []string {
	for i := range ss {
		ss[i] = r.String(ss[i])
	}
	return ss
}

// HashIdentifier returns a stable, non-reversible label for a sensitive
// identifier such as a query text or parameter value.
func HashIdentifier(s string) string {
	sum := sha256.Sum256([]byte(s))
	return "sha256:" + hex.EncodeToString(sum[:8])
}
