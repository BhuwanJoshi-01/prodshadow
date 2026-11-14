package workload

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/sqlinspect"
)

var idRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// MaxScenarios bounds scenarios plus background definitions; results index
// them with a 16-bit number.
const MaxScenarios = 1000

// Validate checks structure, SQL safety and parameter counts.
func (s *Spec) Validate() error {
	var errs []error
	add := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	if s.Version != 1 {
		add("unsupported workload version %d", s.Version)
	}
	if len(s.Scenarios) == 0 {
		add("workload defines no scenarios")
	}
	if n := len(s.Scenarios) + len(s.Background); n > MaxScenarios {
		add("workload defines %d scenarios and background sessions; at most %d are supported", n, MaxScenarios)
	}
	if s.ThinkTimeMS < 0 || s.ThinkTimeMS > 60_000 {
		add("think_time_ms must be between 0 and 60000")
	}
	ids := map[string]bool{}
	checkID := func(where, id string) {
		if !idRe.MatchString(id) {
			add("%s: id %q must be lower-case letters, digits, '_' or '-'", where, id)
		}
		if ids[id] {
			add("%s: duplicate id %q", where, id)
		}
		ids[id] = true
	}

	for i, sc := range s.Scenarios {
		where := fmt.Sprintf("scenarios[%d]", i)
		checkID(where, sc.ID)
		if sc.Weight <= 0 {
			add("%s (%s): weight must be positive", where, sc.ID)
		}
		if sc.Mode != "read" && sc.Mode != "write" {
			add("%s (%s): mode must be \"read\" or \"write\"", where, sc.ID)
		}
		if sc.TimeoutMS < 0 || sc.TimeoutMS > 3_600_000 {
			add("%s (%s): timeout_ms must be between 0 and 3600000", where, sc.ID)
		}
		if t := sc.ThinkTimeMS; t != nil && (*t < 0 || *t > 60_000) {
			add("%s (%s): think_time_ms must be between 0 and 60000", where, sc.ID)
		}
		var stmts []string
		switch {
		case sc.SQL != "" && sc.Transaction != nil:
			add("%s (%s): set either sql or transaction, not both", where, sc.ID)
		case sc.SQL != "":
			stmts = []string{sc.SQL}
		case sc.Transaction != nil:
			stmts = sc.Transaction.Statements
			if err := sc.Transaction.validate(); err != nil {
				add("%s (%s): %v", where, sc.ID, err)
			}
		default:
			add("%s (%s): sql or transaction is required", where, sc.ID)
		}
		if err := validateStatements(stmts, len(sc.Parameters)); err != nil {
			add("%s (%s): %v", where, sc.ID, err)
		}
		for j, p := range sc.Parameters {
			if err := p.Validate(); err != nil {
				add("%s (%s).parameters[%d]: %v", where, sc.ID, j, err)
			}
		}
	}

	for i, bg := range s.Background {
		where := fmt.Sprintf("background[%d]", i)
		checkID(where, bg.ID)
		if bg.Sessions < 1 || bg.Sessions > 64 {
			add("%s (%s): sessions must be between 1 and 64", where, bg.ID)
		}
		if bg.StartAfterSeconds < 0 {
			add("%s (%s): start_after_seconds must not be negative", where, bg.ID)
		}
		if err := bg.Transaction.validate(); err != nil {
			add("%s (%s): %v", where, bg.ID, err)
		}
		if err := validateStatements(bg.Transaction.Statements, len(bg.Parameters)); err != nil {
			add("%s (%s): %v", where, bg.ID, err)
		}
		for j, p := range bg.Parameters {
			if err := p.Validate(); err != nil {
				add("%s (%s).parameters[%d]: %v", where, bg.ID, j, err)
			}
		}
	}
	errs = append(errs, s.validateCompatibility()...)
	return errors.Join(errs...)
}

// MaxProbes bounds the compatibility probes of a workload.
const MaxProbes = 200

func (s *Spec) validateCompatibility() []error {
	var errs []error
	add := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }
	versions := map[string]bool{}
	total := 0
	for i, v := range s.Compatibility {
		where := fmt.Sprintf("compatibility[%d]", i)
		switch {
		case v.Version == "":
			add("%s: version is required", where)
		case len(v.Version) > 64:
			add("%s: version must be at most 64 characters", where)
		case versions[v.Version]:
			add("%s: duplicate version %q", where, v.Version)
		}
		versions[v.Version] = true
		if len(v.Probes) == 0 {
			add("%s (%s): at least one probe is required", where, v.Version)
		}
		ids := map[string]bool{}
		for j, p := range v.Probes {
			total++
			pw := fmt.Sprintf("%s.probes[%d]", where, j)
			if !idRe.MatchString(p.ID) {
				add("%s: id %q must be lower-case letters, digits, '_' or '-'", pw, p.ID)
			}
			if ids[p.ID] {
				add("%s: duplicate id %q in version %s", pw, p.ID, v.Version)
			}
			ids[p.ID] = true
			if p.SQL == "" {
				add("%s (%s): sql is required", pw, p.ID)
			} else if err := validateStatements([]string{p.SQL}, len(p.Parameters)); err != nil {
				add("%s (%s): %v", pw, p.ID, err)
			}
			for k, g := range p.Parameters {
				if err := g.Validate(); err != nil {
					add("%s (%s).parameters[%d]: %v", pw, p.ID, k, err)
				}
			}
		}
	}
	if total > MaxProbes {
		add("workload defines %d compatibility probes; at most %d are supported", total, MaxProbes)
	}
	return errs
}

func (t *TxnSpec) validate() error {
	if len(t.Statements) == 0 {
		return errors.New("transaction needs at least one statement")
	}
	if t.HoldMS < 0 || t.HoldMS > 600_000 {
		return errors.New("hold_ms must be between 0 and 600000")
	}
	return nil
}

// validateStatements parses each statement, allows only DML, and checks
// that every statement's parameters are covered by the parameter list.
func validateStatements(stmts []string, params int) error {
	for i, sql := range stmts {
		st, err := sqlinspect.ParseOne(sql)
		if err != nil {
			return fmt.Errorf("statement %d: %w", i+1, err)
		}
		if err := st.RequireDML(); err != nil {
			return fmt.Errorf("statement %d: %w", i+1, err)
		}
		if n := st.MaxParam(); n > params {
			return fmt.Errorf("statement %d references $%d but only %d parameter(s) are defined", i+1, n, params)
		}
	}
	return nil
}
