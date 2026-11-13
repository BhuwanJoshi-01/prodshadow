# Limitations

ProdShadow measures a model of your system. Read this before trusting a
`pass`.

## What the experiment reproduces well

* **Lock modes and lock queues.** Which statement takes which lock, what it
  conflicts with, and what happens when a long transaction is open. These
  depend on PostgreSQL semantics and concurrency, not on data size.
* **Transactional behaviour.** Statements that cannot run in a transaction
  block, failures and rollbacks, `lock_timeout` and `statement_timeout`.
* **Application compatibility within the workload.** If a scenario models an
  old application version, its queries fail when the migration removes what
  they use.

## What it does not reproduce

* **Durations at production scale.** Rewrites, index builds, validations and
  backfills scale with table size, hardware, I/O and concurrent load. A
  fixture with 200,000 rows does not tell you how long a build on 2 billion
  rows takes. Treat durations as lower bounds at best.
* **Your real traffic.** The workload is what you declare. Queries you do
  not model, connection pools, retries and timeouts in your application are
  not exercised.
* **Replication, failover, storage limits and WAL archiving.** The
  disposable server is a single standalone instance on tmpfs.
* **Planner behaviour on production statistics.** Plans on a small fixture
  can differ from production plans.

## Measurement limits

* Server-side lock sampling runs every `telemetry.sample_interval_ms`
  (100 ms by default). Shorter waits can be missed; client-side latency and
  the server log (every wait above `deadlock_timeout`, 200 ms by default in
  the disposable server) are the complementary signals. The report states
  the interval and any gaps.
* Latency on shared CI runners is noisy. ProdShadow repeats pairs, alternates
  their order and treats regressions within three times the baseline's
  variation as inconclusive. Increase `repeats` or `measurement_seconds`, or
  use a dedicated runner, if results are often inconclusive.
* The sampler and workers run on the same host as the server. A saturated
  host distorts everything; `QUAL005` reports when sampling fell behind.

## Parser limits

The bundled parser understands the PostgreSQL 17 grammar. Newer syntax may
not parse; such statements are still executed in the disposable server and
reported as `PS116` (inconclusive by default, configurable). Lock
predictions for `DO` blocks, procedure calls and functions are not possible
(`PS117`).

## Platform limits

* `network: none` shares a unix socket with the container and therefore
  requires Linux. Use `network: loopback` with Docker Desktop.
* The `local` provider runs PostgreSQL binaries on the host without CPU,
  memory or disk limits and cannot run as root.
* Building from source requires cgo and a C compiler.

## Inputs you must provide

ORM migrations (Rails, Django, Prisma, Alembic, ...) must be rendered to SQL
first. Data migrations implemented in application code are not covered.
