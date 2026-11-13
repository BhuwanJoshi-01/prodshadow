package gen

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

// Spec declares a generator in YAML. Only the fields relevant to the chosen
// generator are used; Validate rejects contradictory combinations.
type Spec struct {
	Generator string `yaml:"generator" json:"generator"`

	// int_range, float_range, zipf
	Min *float64 `yaml:"min" json:"min,omitempty"`
	Max *float64 `yaml:"max" json:"max,omitempty"`
	// S is the zipf skew exponent (> 1). Larger values concentrate traffic
	// on fewer keys, modelling hot rows.
	S float64 `yaml:"s" json:"s,omitempty"`

	// choice
	Values  []any     `yaml:"values" json:"values,omitempty"`
	Weights []float64 `yaml:"weights" json:"weights,omitempty"`

	// const
	Value any `yaml:"value" json:"value,omitempty"`

	// sequence
	Start *int64 `yaml:"start" json:"start,omitempty"`
	Step  *int64 `yaml:"step" json:"step,omitempty"`

	// text
	MinLength int `yaml:"min_length" json:"min_length,omitempty"`
	MaxLength int `yaml:"max_length" json:"max_length,omitempty"`

	// timestamp_range (RFC 3339)
	From string `yaml:"from" json:"from,omitempty"`
	To   string `yaml:"to" json:"to,omitempty"`

	// pattern: "{n}" is replaced by a per-stream counter starting at 1.
	Format string `yaml:"format" json:"format,omitempty"`

	// bool
	TrueRatio *float64 `yaml:"true_ratio" json:"true_ratio,omitempty"`

	// NullRatio is the probability of producing NULL, for any generator.
	NullRatio float64 `yaml:"null_ratio" json:"null_ratio,omitempty"`
}

// Names lists the supported generators.
var Names = []string{"int_range", "float_range", "zipf", "choice", "const", "sequence", "text", "uuid", "timestamp_range", "pattern", "bool"}

// Validate checks the spec without constructing a generator.
func (s Spec) Validate() error {
	if s.NullRatio < 0 || s.NullRatio > 1 {
		return fmt.Errorf("null_ratio %g must be between 0 and 1", s.NullRatio)
	}
	switch s.Generator {
	case "int_range", "float_range":
		if s.Min == nil || s.Max == nil {
			return fmt.Errorf("%s requires min and max", s.Generator)
		}
		if *s.Min > *s.Max {
			return fmt.Errorf("%s: min %g is greater than max %g", s.Generator, *s.Min, *s.Max)
		}
		if s.Generator == "int_range" && (*s.Min != math.Trunc(*s.Min) || *s.Max != math.Trunc(*s.Max)) {
			return errors.New("int_range: min and max must be integers")
		}
	case "zipf":
		if s.Min == nil || s.Max == nil {
			return errors.New("zipf requires min and max")
		}
		if *s.Max <= *s.Min {
			return errors.New("zipf: max must be greater than min")
		}
		if s.S <= 1 {
			return fmt.Errorf("zipf: s must be greater than 1, got %g", s.S)
		}
	case "choice":
		if len(s.Values) == 0 {
			return errors.New("choice requires values")
		}
		if len(s.Weights) > 0 && len(s.Weights) != len(s.Values) {
			return fmt.Errorf("choice: %d weights for %d values", len(s.Weights), len(s.Values))
		}
		total := 0.0
		for _, w := range s.Weights {
			if w < 0 {
				return errors.New("choice: weights must not be negative")
			}
			total += w
		}
		if len(s.Weights) > 0 && total == 0 {
			return errors.New("choice: weights must not all be zero")
		}
	case "const":
	case "sequence":
		if s.Step != nil && *s.Step == 0 {
			return errors.New("sequence: step must not be zero")
		}
	case "text":
		if s.MinLength < 0 || s.MaxLength < s.MinLength || s.MaxLength > 1<<20 {
			return errors.New("text: require 0 <= min_length <= max_length <= 1048576")
		}
		if s.MaxLength == 0 {
			return errors.New("text requires max_length")
		}
	case "uuid":
	case "timestamp_range":
		from, err := time.Parse(time.RFC3339, s.From)
		if err != nil {
			return fmt.Errorf("timestamp_range: from: %w", err)
		}
		to, err := time.Parse(time.RFC3339, s.To)
		if err != nil {
			return fmt.Errorf("timestamp_range: to: %w", err)
		}
		if !to.After(from) {
			return errors.New("timestamp_range: to must be after from")
		}
	case "pattern":
		if !strings.Contains(s.Format, "{n}") {
			return errors.New("pattern: format must contain {n}")
		}
	case "bool":
		if s.TrueRatio != nil && (*s.TrueRatio < 0 || *s.TrueRatio > 1) {
			return errors.New("bool: true_ratio must be between 0 and 1")
		}
	case "":
		return errors.New("generator is required")
	default:
		return fmt.Errorf("unknown generator %q (supported: %s)", s.Generator, strings.Join(Names, ", "))
	}
	return nil
}
