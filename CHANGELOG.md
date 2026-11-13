# Changelog

All notable changes are documented here. The project follows semantic
versioning; the report schema (`schema_version`) is versioned separately.

## [Unreleased]

### Added

- `prodshadow init`, `validate`, `inspect`, `verify`, `report`, `doctor`,
  `cleanup`, `rules`, `schema` and `version` commands.
- Static analysis with the PostgreSQL 17 grammar: per-statement lock
  classification and rules PS101–PS121, inline
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
- Risk evaluation (OBS001–OBS011, QUAL001–QUAL007) with confidence, noise
  bands, inconclusive handling and audited suppressions.
- JSON (schema 1.0.0), Markdown and self-contained HTML reports with
  redaction.
- Examples: safe column addition, long-running transaction, blocking index,
  heavy backfill, rolling deploy.
