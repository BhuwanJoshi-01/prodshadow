# ADR 0011: Predict locks on relations a statement does not name

**Status:** accepted

## Context

The classifier predicted locks from a statement's text alone. Many
statements lock relations the text does not name: `DROP INDEX` locks the
index's table, dropping a foreign key locks the referenced table, an
`INSERT` checks the referenced row of every foreign key, DDL on a
partitioned table recurses to its partitions, queries through a view lock
its tables, triggers and rules run more statements. Without them `plan`
missed conflicts, rules such as PS114 named only part of what a statement
blocks, and several failures (a `DROP TABLE` other tables still reference,
a concurrent refresh without a unique index) could not be predicted at
all. A user's fixture schema, which every ProdShadow project already has,
contains the information.

## Decision

* `sqlinspect.Catalog` learns from schema statements: indexes and their
  tables, partitions (including DEFAULT partitions), views and the columns
  they read, materialized views, column sequences, keys and foreign keys,
  functions (static SQL of PL/pgSQL and SQL bodies), triggers and rules.
  It parses the schema with the same grammar as the migration and never
  connects to a database.
* `Catalog.Resolve` adds the implied locks to each migration statement and
  keeps learning from the migration itself (creates, renames, drops), so
  later statements see the schema as the migration left it. Implied locks
  carry `via` with their origin; reports show it (schema 1.7.0).
* Rules that need the existing schema read facts `Resolve` records on the
  statement, or `Options.Catalog`, which describes the schema before the
  migration.
* Locks that depend on run-time data (row-level triggers, partition
  pruning) are predicted as locks a statement *may* take.
* Without a schema nothing changes: `inspect` without `--schema` or
  `--config` behaves as before.

## Consequences

* `plan` predicts conflicts through foreign keys, partitions, views and
  triggers, and reports show the prediction next to the measurement.
* Predictions are only as good as the fixture schema; `validate` warns
  about tables the workload or migration uses that it does not create.
* Dynamic SQL, functions in other languages and triggers fired by
  triggers are not modelled; the experiment still observes them.
* Every resolved lock is checked against a real server by the lock oracle
  ([ADR 0012](0012-lock-oracle.md)).
