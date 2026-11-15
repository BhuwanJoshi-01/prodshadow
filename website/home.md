---
title: Rehearse PostgreSQL migrations under load
description: ProdShadow runs your migration in a disposable PostgreSQL server while your workload keeps querying it, and reports who waited, for how long, and why.
template: home.html
hide:
  - navigation
  - toc
---

<div class="ps-split" markdown>

<div markdown>

<span class="ps-eyebrow">The problem</span>

## The outage a linter cannot show you

A statement that takes a millisecond, such as `ALTER TABLE ... ADD COLUMN`,
needs an `ACCESS EXCLUSIVE` lock. If one long transaction holds any lock on the
table, the `ALTER TABLE` waits, and **every query that arrives after it waits
behind it**.

A linter can tell you the lock mode; it cannot show you the queue. ProdShadow
reproduces the queue on purpose and measures what it does to everyone else.

</div>

<div markdown>

```text
  sec      ops  errors      p95       max
     3    15538       0   1.2 ms    6.6 ms
     4    16050       0   1.2 ms    5.8 ms
M    5       63       0  1504 ms   1504 ms   <- ALTER TABLE waits
M    6     7232       0   1.2 ms    9.4 ms
     7    14844       0   1.3 ms    7.1 ms
```

<p class="ps-caption">The candidate phase of an experiment, 8 sessions. <code>M</code> marks the
seconds in which the migration ran.</p>

</div>

</div>

<div class="ps-section-head" markdown>

<span class="ps-eyebrow">What it does</span>

## Evidence, not opinions

Static analysis tells you what a statement locks. ProdShadow also runs it,
under your workload, and shows what that lock does to everyone else.

</div>

<div class="grid cards" markdown>

-   :material-crystal-ball:{ .lg .middle } **Predicts before anything runs**

    ---

    `prodshadow plan` lists every lock each statement takes, including the
    ones on tables it does not name (through foreign keys, partitions, views,
    triggers and indexes), and which of your workload's queries will wait
    for it.

    [:octicons-arrow-right-24: Locks beyond the named table](guides/locks-beyond-the-named-table.md)

-   :material-chart-timeline-variant:{ .lg .middle } **Measures under load**

    ---

    A paired experiment runs your workload with and without the migration on
    fresh copies of the same fixture, samples `pg_locks` to attribute every
    wait, and repeats the pair to tell a real regression from noise.

    [:octicons-arrow-right-24: Reading a report](guides/reading-a-report.md)

-   :material-gavel:{ .lg .middle } **Explains the decision**

    ---

    93 rules turn evidence into findings with a confidence and a reason:
    61 static, 17 observed, 15 about the experiment's own quality. An
    experiment that cannot decide says *inconclusive*, never *pass*.

    [:octicons-arrow-right-24: Rule catalogue](rules.md)

-   :material-source-pull:{ .lg .middle } **Fits your CI**

    ---

    Exit codes, JSON, Markdown, HTML, SARIF and JUnit reports, inline
    annotations on the migration's lines, and a GitHub Action that posts the
    predicted conflicts before the experiment finishes.

    [:octicons-arrow-right-24: Using ProdShadow in CI](ci.md)

-   :material-shield-lock-outline:{ .lg .middle } **Never touches production**

    ---

    The server runs in a container without network access, with a read-only
    root, dropped capabilities and resource limits, and every connection
    checks a per-run ownership token.

    [:octicons-arrow-right-24: Threat model](threat-model.md)

-   :material-check-decagram-outline:{ .lg .middle } **Checked against PostgreSQL**

    ---

    A lock oracle runs 168 statements on PostgreSQL 15, 16, 17 and 18 and
    compares the locks in `pg_locks` with the prediction, so the analysis is
    tested against the server, not against itself.

    [:octicons-arrow-right-24: Why the oracle exists](adr/0012-lock-oracle.md)

</div>

<div class="ps-section-head" markdown>

<span class="ps-eyebrow">How it works</span>

## From SQL file to decision

</div>

<div class="ps-steps" markdown>

1. **Inspect** Every statement is parsed with the PostgreSQL grammar; its
   locks and the hazards visible in the SQL are reported in seconds, without
   a database.
2. **Plan** With your fixture schema, ProdShadow predicts which scenarios of
   your workload each statement will block.
3. **Verify** A disposable server is seeded from your fixture, and baseline
   and candidate phases run your workload, the candidate with the migration
   started part-way through.
4. **Decide** Waits, latency, errors and throughput are compared against
   your policy, and the report says what happened and why.

</div>

<div class="ps-split" markdown>

<div markdown>

<span class="ps-eyebrow">Quick start</span>

## Three commands to a first result

`init` writes a starter configuration, fixture, workload and migration.
`plan` needs no database at all. `verify` runs the full experiment in a
sealed container. See the [introduction](introduction.md) for requirements
and the [CLI reference](cli.md) for every flag.

</div>

<div markdown>

```bash
go install github.com/BhuwanJoshi-01/prodshadow/cmd/prodshadow@latest

prodshadow init     # a starter project
prodshadow plan     # predicted locks, no database
prodshadow verify   # the full experiment
```

</div>

</div>

<div class="ps-section-head" markdown>

<span class="ps-eyebrow">Examples</span>

## Measured on real migrations

Every example is a complete project with its documented outcome, run
against PostgreSQL 17 in CI.

</div>

<div class="ps-card-grid" markdown>

<div class="ps-example" markdown>
<span class="ps-badge ps-badge--block">block</span>
[Long-running transaction](examples/long-running-transaction.md){ .ps-example__link }

`ADD COLUMN` queues behind a report; every query queues behind it.
</div>

<div class="ps-example" markdown>
<span class="ps-badge ps-badge--block">block</span>
[Blocking index build](examples/blocking-index.md){ .ps-example__link }

`CREATE INDEX` stalls writers; `CONCURRENTLY` does not.
</div>

<div class="ps-example" markdown>
<span class="ps-badge ps-badge--block">block</span>
[Heavy backfill](examples/backfill-heavy.md){ .ps-example__link }

One unbatched `UPDATE` holds every row lock.
</div>

<div class="ps-example" markdown>
<span class="ps-badge ps-badge--block">block</span>
[Rolling deploy](examples/rolling-deploy.md){ .ps-example__link }

The previous release still reads a dropped column.
</div>

<div class="ps-example" markdown>
<span class="ps-badge ps-badge--block">block</span>
[Partition maintenance](examples/partition-maintenance.md){ .ps-example__link }

`PARTITION OF` locks the parent; attaching does not.
</div>

<div class="ps-example" markdown>
<span class="ps-badge ps-badge--block">block</span>
[Materialized view refresh](examples/matview-refresh.md){ .ps-example__link }

A plain refresh blocks every reader for seconds.
</div>

<div class="ps-example" markdown>
<span class="ps-badge ps-badge--block">block</span>
[Dropping a foreign key](examples/foreign-key-drop.md){ .ps-example__link }

The drop locks the referenced table too.
</div>

<div class="ps-example" markdown>
<span class="ps-badge ps-badge--pass">pass</span>
[Safe column addition](examples/safe-add-column.md){ .ps-example__link }

A brief lock with a `lock_timeout`.
</div>

</div>

!!! note "A pass is not a guarantee"

    ProdShadow produces evidence and uncertainty. A passing result means no
    configured threshold was exceeded in this experiment, not that the
    migration is safe in production. Read the
    [limitations](limitations.md) before trusting one.

<div class="ps-cta" markdown>

## Rehearse your next migration

Point ProdShadow at your schema, describe your traffic, and see what your
migration does to it, before your users do. Free and open source.

[Get started](introduction.md){ .md-button .md-button--primary }
[View on GitHub](https://github.com/BhuwanJoshi-01/prodshadow){ .md-button }

</div>
