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

// pattern is a credential format and its replacement, which may keep a
// leading part of the match (such as the key name) through ${1}.
type pattern struct {
	re          *regexp.Regexp
	replacement string
}

// builtinPatterns match credentials that commonly leak into SQL text, error
// messages and connection strings.
var builtinPatterns = []pattern{
	// URL credentials: postgres://user:secret@host
	{regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*://[^\s:/@]+:)[^\s@/]+@`), "${1}" + Redacted + "@"},
	// key=value connection string passwords.
	{regexp.MustCompile(`(?i)\b(password|passwd|pwd)\s*=\s*('[^']*'|"[^"]*"|\S+)`), "${1}=" + Redacted},
	// SQL password clauses: CREATE/ALTER ROLE ... PASSWORD 'x', and
	// OPTIONS (password 'x') of user mappings and foreign servers.
	{regexp.MustCompile(`(?i)\b((?:encrypted\s+)?password\s+)('(?:[^']|'')*'|"[^"]*")`), "${1}'" + Redacted + "'"},
	// PGPASSWORD style environment assignments.
	{regexp.MustCompile(`(?i)\b(PGPASSWORD|POSTGRES_PASSWORD)\s*[=:]\s*\S+`), Redacted},
	// Bearer tokens.
	{regexp.MustCompile(`(?i)\bbearer\s+[a-z0-9._~+/-]{12,}=*`), Redacted},
	// AWS access key IDs.
	{regexp.MustCompile(`\b(AKIA|ASIA)[A-Z0-9]{16}\b`), Redacted},
	// GitHub tokens.
	{regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{36,}\b`), Redacted},
	// Slack tokens.
	{regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{10,}\b`), Redacted},
	// Stripe secret and restricted keys.
	{regexp.MustCompile(`\b(sk|rk)_(live|test)_[A-Za-z0-9]{16,}\b`), Redacted},
	// Google API keys.
	{regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}\b`), Redacted},
	// JSON Web Tokens.
	{regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`), Redacted},
	// PEM private keys.
	{regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`), Redacted},
}

// Redactor removes secrets from text before it reaches an artifact. Exact
// secrets generated during a run (passwords, tokens) are registered with
// Add; pattern-based detection covers common credential formats.
type Redactor struct {
	mu       sync.RWMutex
	exact    []string
	patterns []pattern
}

// NewRedactor compiles user-defined patterns in addition to the built-ins.
func NewRedactor(userPatterns []string) (*Redactor, error) {
	r := &Redactor{patterns: append([]pattern(nil), builtinPatterns...)}
	for _, p := range userPatterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, err
		}
		r.patterns = append(r.patterns, pattern{re, Redacted})
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
	for _, p := range r.patterns {
		s = p.re.ReplaceAllString(s, p.replacement)
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
