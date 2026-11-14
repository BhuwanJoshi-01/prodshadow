# Changelog

All notable changes are documented here. The project follows semantic
versioning; the report schema (`schema_version`) is versioned separately.

## [Unreleased]

### Added

- `prodshadow init`, `validate`, `inspect`, `verify`, `report`, `doctor`,
  `cleanup`, `rules`, `schema` and `version` commands.
- Static analysis with the PostgreSQL 17 grammar: per-statement lock
  classification and rules PS101–PS122, inline
  `prodshadow:ignore` annotations with reason and expiry.
- Hardened Docker/Podman provider (no network, read-only root, tmpfs data,
  dropped capabilities, resource limits) and a local-binaries provider.
- Per-run ownership token verified on every connection; connections limited
  to unix sockets and loopback addresses.
- Deterministic fixture generator and workload parameter generators
  (including Zipf hot keys).
- Concurrent workload runner with closed-loop and open-loop modes and
  background blocker sessions.
- Lock telemetry from `pg_stat_activity`, `pg_locks` and
  `pg_blocking_pids()` with blocker attribution, plus WAL and deadlock
  counters.
- Paired baseline/candidate experiment on fresh template copies with
  alternating order.
- Risk evaluation (OBS001–OBS011, QUAL001–QUAL008) with confidence, noise
  bands, inconclusive handling and audited suppressions.
- JSON (schema 1.0.0), Markdown and self-contained HTML reports with
  redaction.
- Examples: safe column addition, long-running transaction, blocking index,
  heavy backfill, rolling deploy.
- `prodshadow plan` shows the schedule, transaction groups and an estimated
  duration without starting anything.
- SARIF 2.1.0 and JUnit XML report formats; GitHub Actions annotations on
  migration lines.
- Static rules PS120 (ineffective `lock_timeout`) and PS122 (blocking
  `DROP INDEX`); quality rule QUAL008 (no long-running transaction modelled).
- Connection budget check against `max_connections` in `validate`,
  `plan` and `verify`.
- Reports record the effective configuration.
- Inline `prodshadow:ignore` annotations also apply to observed findings
  attributed to the statement.
- Static rules PS123 (`ATTACH PARTITION` scans), PS124 (`CASCADE`), PS125
  (foreign key enforcement disabled), PS126 (database-wide maintenance),
  PS127 (extensions that need preloading), PS128 (enum value used before
  commit), PS129 (persistent setting changes), PS130 (sequence resets),
  PS131 (server-side file access), PS132 (unnamed concurrent indexes),
  PS133 (foreign keys without an index), PS134 (unbalanced transaction
  control) and PS135 (`statement_timeout` disabled). PS109 and PS110 also
  cover views, functions, types, schemas and enum values.
- Application-version compatibility probes: the workload's `compatibility`
  section lists the queries each deployed release depends on; they run
  before and after the migration (`OBS013`, `QUAL009`). Report schema 1.1.0
  adds `phases[].compatibility`.
- Optional `policy.max_queued_sessions` budget (`OBS012`).
- `prodshadow history` summarises stored reports as a timeline;
  `prodshadow completion` prints bash, zsh and fish completion;
  `prodshadow rules ID...` shows rule details; `inspect --migration -`
  reads standard input; `version --short`.
- Every rule has a documentation page under `docs/rules/`, linked from
  SARIF results.
- Guides for Alembic, Django, Prisma, Flyway/Liquibase and
  golang-migrate/goose; troubleshooting guide, glossary and architecture
  decision records.

### Fixed

- `REINDEX SCHEMA/DATABASE/SYSTEM`, `ALTER SYSTEM`, `CREATE/DROP DATABASE`,
  `CREATE/DROP TABLESPACE` and `CLUSTER` without a table are recognised as
  statements that cannot run in a transaction block.
- Relations created by `CREATE TABLE AS`, `CREATE MATERIALIZED VIEW` and
  `CREATE SEQUENCE` count as new, like `CREATE TABLE`.
- The disposable server is stopped gracefully before removal, and socket
  files owned by the container are cleaned up on non-root CI runners.

### Verified

- Integration tests pass on PostgreSQL 15, 16, 17 and 18 (`postgres:*-alpine`).
- Every example reproduces its documented decision; see
  [docs/benchmarks.md](docs/benchmarks.md).
