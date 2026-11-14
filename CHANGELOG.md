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
  control), PS135 (`statement_timeout` disabled), PS136 (validated in the
  same transaction), PS137 (constraint left `NOT VALID`), PS138 (revoked
  privileges), PS139 (plain-text passwords), PS140 (blocking `DETACH
  PARTITION`), PS141 (`PARTITION OF` locks the parent), PS142 (autovacuum
  disabled), PS143 (unlogged tables), PS144 (row-level security disabled),
  PS145 (grants to `PUBLIC`), PS146 (`SECURITY DEFINER` without
  `search_path`), PS147 (replica identity changes), PS148 to PS150
  (malformed, expired and unused inline suppressions), PS151 (`CHECK`
  constraints that are not immutable), PS152 (32-bit primary keys),
  PS153 (tables without a primary key) and PS154 (`CREATE INDEX
  CONCURRENTLY` on a partitioned table). PS107 and PS108 also report
  inline `CHECK`, `UNIQUE` and `PRIMARY KEY` on `ADD COLUMN`. PS109 and PS110 also
  cover views, functions, types, schemas, extensions and enum values; PS111
  covers unbounded `SELECT ... FOR UPDATE`; PS106 recognises a validated
  `CHECK (col IS NOT NULL)`; PS105 uses `USING` clauses and type limits for
  its confidence.
- Application-version compatibility probes: the workload's `compatibility`
  section lists the queries each deployed release depends on; they run
  before and after the migration (`OBS013`, `QUAL009`). Report schema 1.1.0
  adds `phases[].compatibility`.
- Optional `policy.max_queued_sessions` budget (`OBS012`).
- `prodshadow history` summarises stored reports as a timeline;
  `prodshadow completion` prints bash, zsh and fish completion;
  `prodshadow rules ID...` shows rule details; `inspect --migration -`
  reads standard input; `version --short`; `prodshadow locks` explains
  lock modes; `inspect --format junit` and `inspect --quiet`;
  `history --markdown`; `doctor` checks the engine's memory and CPUs;
  `validate` warns about an unreachable `max_queued_sessions`.
- Report schema 1.2.0: `findings[].app_version`; suppressions can be
  limited to an application version; rule IDs link to their pages in
  Markdown, HTML and GitHub annotations; compatibility probes appear in the
  HTML report and in `plan`.
- `prodshadow draft-workload` drafts a workload file from a
  `pg_stat_statements` CSV export, and `prodshadow draft-fixture` a
  generator file from the fixture schema and optional `pg_class` row
  counts; `history --html` renders a page with
  charts; `inspect --format markdown` links rule pages.
- Rules can reason about other rules' findings (`checkFindings`), and
  PS114 follows the scope of `SET LOCAL`, `RESET` and zero timeouts.
- `migration.lock_retries` and `retry_backoff_ms` rehearse a deploy tool
  that retries on `lock_timeout`; retries are recorded per candidate
  (report schema 1.3.0) and reported as `OBS014`.
- `--changed-since REF` on `inspect`, `plan` and `verify`, and a
  `changed-since` input on the GitHub Action, limit a run to the migrations
  added or changed since a git ref.
- Observed rules `OBS015` (optional `policy.max_extra_wal_mb` budget for WAL
  written beyond the baseline) and `OBS016` (sorts or index builds spilling
  to temporary files).
- Per-scenario `think_time_ms`; JSON Schema for suppression files
  (`prodshadow schema suppressions`); GitHub Action input `args`.
- Built-in redaction of SQL `PASSWORD '...'` clauses and Stripe and Google
  API keys.
- Every rule has a documentation page under `docs/rules/`, linked from
  SARIF results.
- Guides for Alembic, Atlas, Django, Prisma, Flyway/Liquibase,
  golang-migrate/goose and sqitch/dbmate; troubleshooting guide, glossary and architecture
  decision records.

### Fixed

- `REINDEX SCHEMA/DATABASE/SYSTEM`, `ALTER SYSTEM`, `CREATE/DROP DATABASE`,
  `CREATE/DROP TABLESPACE` and `CLUSTER` without a table are recognised as
  statements that cannot run in a transaction block.
- Relations created by `CREATE TABLE AS`, `CREATE MATERIALIZED VIEW` and
  `CREATE SEQUENCE` count as new, like `CREATE TABLE`.
- `SELECT` statements, and the tables read by `INSERT ... SELECT`,
  `UPDATE ... FROM` and `DELETE ... USING`, now record their locks.
- `on` and `off` in storage parameters are read correctly.
- Compatibility probe results are paired by version and probe ID, so a
  missing result is never reported as a broken probe.
- The disposable server is stopped gracefully before removal, and socket
  files owned by the container are cleaned up on non-root CI runners.

### Verified

- Integration tests pass on PostgreSQL 15, 16, 17 and 18 (`postgres:*-alpine`).
- Every example reproduces its documented decision; see
  [docs/benchmarks.md](docs/benchmarks.md).
