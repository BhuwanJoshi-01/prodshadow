# Landscape and gap analysis

*Research snapshot: November 2026. Versions and activity dates were taken from
public repositories, package registries and project documentation at that
time; re-check before quoting them.*

This document is the Phase 0 deliverable of the blueprint: it records why the
problem is worth solving, what already exists, and where ProdShadow is — and
is not — differentiated.

## 1. Is the problem real?

Yes. Public incident reports show the same pattern again and again: the DDL
itself is fast, but its lock request (usually `ACCESS EXCLUSIVE`, or `SHARE`
for a plain `CREATE INDEX`) queues behind an already running transaction, and
every later query on the table queues behind the DDL.

| Incident | What happened |
|---|---|
| [Railway, 2025-10-28](https://blog.railway.com/p/incident-report-oct-28th-2025) | `CREATE INDEX` without `CONCURRENTLY` on a ~1B-row table blocked API writes; remediation was a CI check enforcing `CONCURRENTLY`. |
| [Railway, 2025-12-08](https://blog.railway.com/p/incident-report-december-8-2025) | A migration conflicted with a long-running query; replicas became unhealthy. |
| [Autumn, 2026-03](https://useautumn.com/blog/post-mortem-database-outage-caused-by-collation-migration-locking-conflict) | `ALTER TABLE` needing an exclusive lock contended with active queries and caused an outage. |
| [GoCardless](https://gocardless.com/blog/zero-downtime-postgres-migrations-the-hard-parts/) | Lock-queue behaviour during a planned migration caused API downtime. |
| [GitLab.com, 2018](https://gitlab.com/gitlab-com/gl-infra/production/-/work_items/300) | Lock contention during a deploy's index build made inserts fail for ~90 minutes. |

The common root cause — **lock-queue amplification behind a long
transaction** — is a property of lock modes and concurrency, not only of data
size. That is what a disposable, workload-driven rehearsal can reproduce.

## 2. What already exists

| Tool | Executes migration on a real DB | Concurrent load during migration | Baseline vs candidate | Static lock analysis | License |
|---|---|---|---|---|---|
| [DBLab Engine – DB Migration Checker](https://github.com/postgres-ai/database-lab-engine) | Yes, on thin clones of production; flags long dangerous locks | No | No | No | Apache-2.0 |
| [GitLab database testing pipeline](https://docs.gitlab.com/development/database/database_migration_pipeline/) | Yes, on clones of GitLab.com | No | No | n/a | Internal |
| [eugene](https://github.com/kaaveland/eugene) (`trace`) | Yes, temporary local DB; records locks taken | No | No | Yes | MIT |
| [pgfence](https://github.com/flvmnt/pgfence) (trace mode) | Yes, in Docker; reads `pg_locks` | No | No | Yes | MIT |
| [pg-migration-check](https://github.com/Ahmedabied/pg-migration-check) | Yes, scratch DB sized from statistics; measures lock mode and hold time | No (documents this gap) | No | Partial | MIT |
| [squawk](https://github.com/sbdchd/squawk) | No | No | No | Yes | Apache-2.0/MIT |
| [strong_migrations](https://github.com/ankane/strong_migrations) | Rule checks at run time | No | No | Rules | MIT |
| [django-pg-zero-downtime-migrations](https://github.com/tbicr/django-pg-zero-downtime-migrations) | Runtime guard | No | No | Rules | MIT |
| [pgroll](https://github.com/xataio/pgroll) / [Reshape](https://github.com/fabianlindfors/reshape) | They are migration runners | No | No | No | Apache-2.0 / MIT |
| [Atlas](https://atlasgo.io/testing/migrate) | `migrate lint` / `migrate test` on a dev DB | No | No | Yes (lock analyzers in paid tier) | Apache-2.0 community build |
| [Stripe pg-schema-diff](https://github.com/stripe/pg-schema-diff) | Validates plan on a temporary DB | No | No | Hazard tags | MIT |
| Bytebase, Liquibase, Flyway checks | No | No | No | Rules | Mixed / commercial tiers |
| Neon / Supabase branching | Yes (branch per PR) | Do-it-yourself | No | No | SaaS |

**Gap:** no packaged tool was found that runs a concurrent workload *while*
the migration executes and compares the result with an equivalent baseline.
Static linting and single-session lock tracing, by contrast, are crowded.

## 3. Consequences for ProdShadow's design

1. **Do not compete on linting.** The static stage exists to *explain*
   observed behaviour (which lock mode, which tables) and to work offline.
   Its rule set stays small and conservative; teams should keep using their
   linter of choice.
2. **Model the blocker, not just the traffic.** Real outages needed a
   long-running or idle-in-transaction session. Workloads therefore support
   first-class *blocker sessions* (`hold` scenarios and background long
   transactions) so lock-queue amplification is reproduced deliberately.
3. **Measure client-side, attribute server-side.** Every workload query is
   timed on the client, which catches waits that polling misses. Polling of
   `pg_stat_activity`, `pg_locks.waitstart` and `pg_blocking_pids()` is used
   for attribution (who blocked whom), with its sampling interval reported.
4. **Fight CI noise explicitly.** Paired runs alternate order (ABBA),
   repeat at least three times, report baseline noise, and also compare the
   pre-migration and during-migration windows of the *same* candidate run.
   Unstable baselines yield `inconclusive`, never `pass`.
5. **The database is authoritative, the parser is advisory.** The bundled
   parser (pg_query_go v6) understands the PostgreSQL 17 grammar. Statements
   it cannot parse are still executed in the disposable server and reported
   as an analysis limitation.
6. **Be honest about scale.** A fixture cannot reproduce the duration of an
   index build on a billion rows. Reports carry a representativeness level
   and never claim a migration is "safe for production".

## 4. Risks to the idea

- **Fidelity:** small fixtures on shared runners understate rewrite and
  index-build durations by orders of magnitude.
- **False negatives:** without an injected blocker, short OLTP scenarios may
  never form a lock queue.
- **Noise:** p95/p99 on shared CI runners is volatile; the tool must prefer
  `inconclusive` over flaky blocking.
- **Thin moat:** DBLab (with realistic clones) or commercial vendors could
  add a workload step.
- **Input friction:** ORM migrations must be rendered to SQL first; data
  migrations written in application code are out of scope.
- **Parser lag and cgo:** the Go parser binding trails the newest grammar and
  requires cgo, which complicates cross-compiled releases.

## 5. Naming

No package named `prodshadow` was found on npm, PyPI, crates.io, RubyGems or
the Go module proxy. The `prodshadow.com` domain belongs to an unrelated
music business. "Shadow database" is also an established Prisma Migrate term,
and "Prod" may suggest the tool touches production — it never does.

## 6. Kill criterion (unchanged from the blueprint)

If most target users already rehearse migrations under load with a single
command and do not value additional evidence, narrow the scope to the
blocker-injection rehearsal or stop.
