# Roadmap and plan

This roadmap turns the original product blueprint into milestones, updated
after the landscape research in [research/landscape.md](research/landscape.md).
Dates are not promises; each milestone ends with an exit criterion.

## Positioning

ProdShadow is a **lock-queue rehearsal**: it runs a migration next to a
declared workload, including the long transactions that real outages need,
and reports what everybody else experienced. It complements linters
(squawk, Atlas, strong_migrations, eugene, pgfence) and clone-based checks
(DBLab); it does not replace them.

## 0.1 — MVP (this release)

| Area | Status |
|---|---|
| Configuration contract, JSON Schemas, validation with line numbers | done |
| Statement splitter, PostgreSQL-grammar parsing, lock classification | done |
| Static rules PS101–PS121, inline and file suppressions | done |
| Hardened Docker/Podman provider, local provider, ownership token | done |
| Deterministic fixtures and generators | done |
| Workload engine: closed/open loop, background blocker sessions | done |
| Lock telemetry with blocker attribution, WAL/deadlock counters | done |
| Paired experiment with alternating order, fresh template copies | done |
| Risk evaluation with noise bands and inconclusive handling | done |
| JSON, Markdown and HTML reports with redaction | done |
| GitHub Action, CI workflows for PostgreSQL 15–18 | done |
| Five end-to-end examples with documented outcomes | done |

**Exit criterion:** an external developer can add ProdShadow to a pull
request workflow without a service account, and the examples produce their
documented decisions in CI.

## 0.1 → 0.2 validation (before building more)

The blueprint's kill criterion still applies. Before investing in 0.2:

* Interview at least 15 maintainers of services with PostgreSQL migrations
  (review flow, last lock incident, CI budget, willingness to keep a fixture
  and workload).
* Recruit 3–5 trial projects; collect false positives and missing signals.
* Publish the baseline-noise benchmark (same workload, two fresh databases)
  on GitHub-hosted runners.

If teams find the static lock mode plus a lock_timeout check sufficient, narrow
ProdShadow to the blocker-injection rehearsal rather than broadening it.

## 0.2 candidates

1. **Read-only statistics import** (separate command, separate credentials,
   allowlisted catalog queries only) to size fixtures and weight scenarios
   from `pg_stat_statements` and `pg_class`. Requires a security review
   first.
2. **Application-version compatibility probes**: declare queries per
   release and report which versions break.
3. **Lock-wait events from the server log** (`log_lock_waits`) to
   corroborate sampled waits.
4. **SQL rendering helpers** for Django, Rails, Alembic and Prisma.
5. **PostgreSQL 19** in the matrix once a parser binding for the newer
   grammar is released.
6. **Prebuilt release binaries** (the release workflow exists; publish the first tagged build) for linux/amd64, linux/arm64 and
   darwin/arm64 (cgo requires native builders).

## Later / research

* Historical trends and a static dashboard over stored JSON reports.
* Multi-node and replica-aware experiments.
* MySQL support behind the provider interface.

## Never

* Executing migrations against production.
* A "safe for production" verdict.
* Opt-out telemetry.

## Issue backlog

| ID | Priority | Issue | Acceptance |
|---|---|---|---|
| PS-001 | P0 | Threat model and isolation invariants | `docs/threat-model.md` with linked negative tests |
| PS-002 | P0 | Config parser and JSON Schema | Valid/invalid fixtures, schema/struct sync test |
| PS-003 | P0 | Managed ephemeral PostgreSQL | Remote addresses refused before dialing |
| PS-004 | P0 | SQL reader and AST classifier | File/line attribution, unparsable statements reported |
| PS-005 | P0 | Deterministic fixture | Hash-stable generated data |
| PS-006 | P0 | Workload parser | Typed, validated scenarios |
| PS-007 | P0 | Concurrent workload engine | Open-loop arrivals, cancellation |
| PS-008 | P0 | Paired orchestrator | Fresh database per phase |
| PS-009 | P0 | Lock and latency telemetry | Samples attributed to sessions and statements |
| PS-010 | P0 | Evidence schema and policies | Inconclusive handled, never a pass |
| PS-011 | P0 | JSON and Markdown report | Golden tests, schema validation |
| PS-012 | P0 | Cleanup and sandbox hardening | Leftover check in CI, cleanup command |
| PS-013 | P1 | GitHub Actions integration | Composite action, documented workflow |
| PS-014 | P1 | Realistic benchmark fixtures | Five examples with expected outcomes |
| PS-015 | P1 | Read-only statistics adapter | Security review, approval, redaction |
| PS-016 | P2 | Web report viewer | Static, no hosted service |
