// Package experiment orchestrates a paired experiment: one disposable
// server, a seeded template database, and for every repeat a baseline phase
// (workload only) and a candidate phase (workload while the migration runs),
// each on a fresh copy of the template.
package experiment

// Kind distinguishes the two phases of a pair.
type Kind string

const (
	Baseline  Kind = "baseline"
	Candidate Kind = "candidate"
)

// Step is one phase in the execution schedule.
type Step struct {
	Kind   Kind `json:"kind"`
	Repeat int  `json:"repeat"`
}

// Schedule alternates phase order between repeats (AB, BA, AB, ...). Running
// the baseline first every time would let warm caches or background
// activity systematically favour one side.
func Schedule(repeats int) []Step {
	var out []Step
	for r := 0; r < repeats; r++ {
		if r%2 == 0 {
			out = append(out, Step{Baseline, r}, Step{Candidate, r})
		} else {
			out = append(out, Step{Candidate, r}, Step{Baseline, r})
		}
	}
	return out
}
