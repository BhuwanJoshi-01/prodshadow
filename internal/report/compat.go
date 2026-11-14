package report

import (
	"fmt"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/experiment"
)

// CompatibilityRow summarises one compatibility probe across the candidate
// phases. The embedded outcome is from a phase where the probe broke, if
// any, else from the first phase that ran it.
type CompatibilityRow struct {
	experiment.Compatibility
	Broken int
	Runs   int
}

// CompatibilityRows lists every probe once, in declaration order.
func (r *Report) CompatibilityRows() []CompatibilityRow {
	var rows []CompatibilityRow
	index := map[string]int{}
	for _, p := range r.Phases {
		for _, c := range p.Compatibility {
			key := c.Version + "\x00" + c.Probe
			i, ok := index[key]
			if !ok {
				i = len(rows)
				index[key] = i
				rows = append(rows, CompatibilityRow{Compatibility: c})
			}
			rows[i].Runs++
			if c.Broken() {
				rows[i].Broken++
				rows[i].Compatibility = c
			}
		}
	}
	return rows
}

// BeforeStatus describes the probe's outcome before the migration.
func (c CompatibilityRow) BeforeStatus() string {
	return probeStatus(c.BeforeOK, c.BeforeSQLState, c.BeforeError)
}

// AfterStatus describes the probe's outcome after the migration.
func (c CompatibilityRow) AfterStatus() string {
	return probeStatus(c.AfterOK, c.AfterSQLState, c.AfterError)
}

func probeStatus(ok bool, state, msg string) string {
	switch {
	case ok:
		return "ok"
	case state != "":
		return fmt.Sprintf("failed (%s: %s)", state, msg)
	default:
		return "failed: " + msg
	}
}
