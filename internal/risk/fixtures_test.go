package risk

import (
	"time"

	"github.com/BhuwanJoshi-01/prodshadow/internal/experiment"
	"github.com/BhuwanJoshi-01/prodshadow/internal/migrate"
	"github.com/BhuwanJoshi-01/prodshadow/internal/stats"
	"github.com/BhuwanJoshi-01/prodshadow/internal/telemetry"
	"github.com/BhuwanJoshi-01/prodshadow/internal/workload"
)

// scen builds scenario stats with the given p95/p99 and max latency.
func scen(id string, ops, errors int, p95, p99, max float64) workload.ScenarioStats {
	rate := 0.0
	if ops > 0 {
		rate = float64(errors) / float64(ops)
	}
	return workload.ScenarioStats{ID: id, Mode: "read", Ops: ops, Errors: errors, ErrorRate: rate,
		Throughput: float64(ops-errors) / 10, MaxObservedMS: max,
		Latency: stats.Summary{Count: uint64(ops - errors), P50MS: p95 / 3, P95MS: p95, P99MS: p99, MaxMS: max}}
}

// blockingResult simulates three repeats where the migration timed out
// behind a long transaction and reads queued for ~1.5 s.
func blockingResult() *experiment.Result {
	res := &experiment.Result{}
	for r := 0; r < 3; r++ {
		res.Phases = append(res.Phases, experiment.PhaseResult{
			Step:      experiment.Step{Kind: experiment.Baseline, Repeat: r},
			Scenarios: []workload.ScenarioStats{scen("read", 2000, 0, 1.0+0.05*float64(r), 2, 5)},
			Locks:     telemetry.Summary{SampleIntervalMS: 100, Samples: 100},
		})
		res.Phases = append(res.Phases, experiment.PhaseResult{
			Step:            experiment.Step{Kind: experiment.Candidate, Repeat: r},
			Scenarios:       []workload.ScenarioStats{scen("read", 1800, 0, 2.5, 1500, 1520)},
			DuringMigration: []workload.ScenarioStats{scen("read", 40, 0, 1400, 1500, 1520)},
			Locks: telemetry.Summary{SampleIntervalMS: 100, Samples: 100, MaxLockWaitMS: 1480, MaxWaitingSessions: 4,
				MaxBlockedByMigration: 4, MigrationMaxWaitMS: 1495, MigrationBlockedBy: []string{"prodshadow/background/report/0"},
				Relations: []telemetry.RelationWait{{Relation: "orders", Mode: "AccessExclusiveLock", MaxWaitMS: 1495}}},
			Migration: &migrate.Outcome{Status: migrate.Failed, Started: time.Now(), SQLState: "55P03", FailedStatement: "1.1",
				Error: "canceling statement due to lock timeout", DurationMS: 1500,
				Statements: []migrate.StatementOutcome{{StatementID: "1.1", File: "m.sql", Line: 1, Tag: "ALTER TABLE", Status: migrate.Failed, SQLState: "55P03", DurationMS: 1500}}},
		})
	}
	return res
}

// quietResult simulates three repeats where nothing changed.
func quietResult() *experiment.Result {
	res := &experiment.Result{}
	for r := 0; r < 3; r++ {
		res.Phases = append(res.Phases,
			experiment.PhaseResult{Step: experiment.Step{Kind: experiment.Baseline, Repeat: r},
				Scenarios: []workload.ScenarioStats{scen("read", 2000, 0, 1.0, 2.0, 6)}, Locks: telemetry.Summary{SampleIntervalMS: 100, Samples: 100}},
			experiment.PhaseResult{Step: experiment.Step{Kind: experiment.Candidate, Repeat: r},
				Scenarios:       []workload.ScenarioStats{scen("read", 2000, 0, 1.05, 2.1, 7)},
				DuringMigration: []workload.ScenarioStats{scen("read", 300, 0, 1.1, 2.2, 7)},
				Locks:           telemetry.Summary{SampleIntervalMS: 100, Samples: 100},
				Migration: &migrate.Outcome{Status: migrate.Succeeded, Started: time.Now(), DurationMS: 120,
					Statements: []migrate.StatementOutcome{{StatementID: "1.1", Tag: "CREATE INDEX CONCURRENTLY", Status: migrate.Succeeded, DurationMS: 120}}}},
		)
	}
	return res
}
