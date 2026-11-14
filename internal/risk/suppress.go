package risk

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"time"

	"go.yaml.in/yaml/v3"
)

// Suppression waives a rule with an audited reason. Suppressions live in a
// file in the repository so that version control records who added them.
type Suppression struct {
	Rule       string `yaml:"rule" json:"rule"`
	Scenario   string `yaml:"scenario" json:"scenario,omitempty"`
	AppVersion string `yaml:"app_version" json:"app_version,omitempty"`
	File       string `yaml:"file" json:"file,omitempty"`
	Reason     string `yaml:"reason" json:"reason"`
	Expires    string `yaml:"expires" json:"expires,omitempty"`
	ApprovedBy string `yaml:"approved_by" json:"approved_by,omitempty"`
}

type suppressionFile struct {
	Version      int           `yaml:"version"`
	Suppressions []Suppression `yaml:"suppressions"`
}

var ruleIDRe = regexp.MustCompile(`^[A-Z]{2,5}[0-9]{3}$`)

// LoadSuppressions reads and validates a suppressions file.
func LoadSuppressions(path string) ([]Suppression, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f suppressionFile
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if f.Version != 1 {
		return nil, fmt.Errorf("%s: unsupported version %d", path, f.Version)
	}
	for i, s := range f.Suppressions {
		if !ruleIDRe.MatchString(s.Rule) {
			return nil, fmt.Errorf("%s: suppressions[%d]: invalid rule %q", path, i, s.Rule)
		}
		if len(s.Reason) < 10 {
			return nil, fmt.Errorf("%s: suppressions[%d]: a reason of at least 10 characters is required", path, i)
		}
		if s.Expires != "" {
			if _, err := time.Parse(time.DateOnly, s.Expires); err != nil {
				return nil, fmt.Errorf("%s: suppressions[%d]: expires must be YYYY-MM-DD", path, i)
			}
		}
	}
	return f.Suppressions, nil
}

// matches reports whether s applies to f at time now.
func (s Suppression) matches(f Finding, now time.Time) bool {
	if s.Rule != f.RuleID {
		return false
	}
	if s.Scenario != "" && s.Scenario != f.Scenario {
		return false
	}
	if s.AppVersion != "" && s.AppVersion != f.AppVersion {
		return false
	}
	if s.File != "" && s.File != f.File {
		return false
	}
	if s.Expires != "" {
		exp, _ := time.Parse(time.DateOnly, s.Expires)
		if !now.Before(exp.AddDate(0, 0, 1)) {
			return false
		}
	}
	return true
}

// applySuppressions marks matching findings. Expired suppressions are
// returned so that reports can flag them.
func applySuppressions(fs []Finding, sups []Suppression, now time.Time) []Suppression {
	var expired []Suppression
	for _, s := range sups {
		if s.Expires != "" {
			exp, _ := time.Parse(time.DateOnly, s.Expires)
			if !now.Before(exp.AddDate(0, 0, 1)) {
				expired = append(expired, s)
				continue
			}
		}
		for i := range fs {
			if !fs[i].Suppressed && s.matches(fs[i], now) {
				fs[i].Suppressed, fs[i].SuppressionReason, fs[i].SuppressionExpiry = true, s.Reason, s.Expires
			}
		}
	}
	return expired
}
