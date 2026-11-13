// Package evidence defines the shared vocabulary used to describe findings:
// how severe they are, how confident ProdShadow is, and what kind of
// evidence supports them. Keeping these values in one place prevents the
// static analyser, the risk engine and the reports from drifting apart.
package evidence

import "fmt"

// Severity ranks the operational importance of a finding.
type Severity string

const (
	Info         Severity = "info"
	Warning      Severity = "warning"
	Critical     Severity = "critical"
	Inconclusive Severity = "inconclusive"
)

// Severities lists known severities in ascending order of importance.
// Inconclusive is listed separately because it is not "worse" than
// critical; it means the experiment could not decide.
var Severities = []Severity{Info, Warning, Critical}

// Rank orders severities for sorting; inconclusive sorts after critical so
// that it is never hidden below informational output.
func (s Severity) Rank() int {
	switch s {
	case Info:
		return 1
	case Warning:
		return 2
	case Critical:
		return 3
	case Inconclusive:
		return 4
	default:
		return 0
	}
}

// Valid reports whether s is a known severity.
func (s Severity) Valid() bool { return s.Rank() > 0 }

// ParseSeverity converts user input into a Severity.
func ParseSeverity(s string) (Severity, error) {
	sev := Severity(s)
	if !sev.Valid() {
		return "", fmt.Errorf("unknown severity %q", s)
	}
	return sev, nil
}

// Confidence states how strongly the evidence supports a finding.
type Confidence string

const (
	High   Confidence = "high"
	Medium Confidence = "medium"
	Low    Confidence = "low"
)

// Rank orders confidence values.
func (c Confidence) Rank() int {
	switch c {
	case Low:
		return 1
	case Medium:
		return 2
	case High:
		return 3
	default:
		return 0
	}
}

// Lower returns the lower of two confidence values.
func (c Confidence) Lower(o Confidence) Confidence {
	if o.Rank() < c.Rank() {
		return o
	}
	return c
}

// Kind states where evidence comes from. Reports keep observed behaviour
// and inference visibly separate.
type Kind string

const (
	// Observed evidence was measured in the disposable database.
	Observed Kind = "observed"
	// Static evidence was inferred from the SQL text and PostgreSQL rules.
	Static Kind = "static"
	// Heuristic evidence is a rule of thumb that may not apply.
	Heuristic Kind = "heuristic"
	// Quality evidence describes the experiment itself.
	Quality Kind = "experiment-quality"
)
