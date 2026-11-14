# Architecture decision records

Short records of decisions that shape ProdShadow, why they were made, and
what they cost. A record is not edited once accepted; a later record
supersedes it.

| ADR | Decision | Status |
|---|---|---|
| [0001](0001-paired-alternating-experiment.md) | Measure the migration as a paired, alternating experiment | accepted |
| [0002](0002-no-production-connections.md) | Never connect to a database ProdShadow did not create | accepted |
| [0003](0003-parse-statements-with-postgres-grammar.md) | Split statements ourselves, parse each with PostgreSQL's grammar | accepted |
| [0004](0004-inconclusive-is-not-a-pass.md) | An inconclusive result is never a pass | accepted |
| [0005](0005-report-schema-is-the-contract.md) | The JSON report is a versioned contract | accepted |
| [0006](0006-measure-from-intended-start.md) | In open-loop mode, measure latency from the intended start | accepted |
| [0007](0007-heuristic-findings-say-so.md) | Static rules mark what they cannot see as heuristic | accepted |
| [0008](0008-statistics-from-exported-files.md) | Import production statistics from exported files, not connections | accepted |
