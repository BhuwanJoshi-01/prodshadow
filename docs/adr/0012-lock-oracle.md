# ADR 0012: The lock oracle is the classifier's acceptance test

**Status:** accepted

## Context

Lock predictions came from the PostgreSQL documentation and source reading,
checked by unit tests that compared the classifier with itself. As the
lock oracle ran the predictions against a real server, it found gaps in
more than thirty statements: `CREATE TABLE AS` and views reading
their sources, `COMMENT ON`, `DROP TRIGGER`, policies, rules, `MERGE`,
writable CTEs, sequences, foreign key checks, default partitions. Lock
levels also change between major versions (`GRANT` takes `ACCESS SHARE`
since PostgreSQL 18).

## Decision

* `internal/sqlinspect/locks_integration_test.go` runs every statement of
  a list inside a transaction on a disposable server, reads its granted
  relation locks from `pg_locks` (by OID, so renames and drops are
  compared), resolves the same statement against a catalog of the
  oracle's schema, and requires both to match.
* A second test requires statements classified as unable to run in a
  transaction block to be refused inside one (SQLSTATE 25001); a third
  requires statements that rules report as failing to fail with the
  expected SQLSTATE.
* Locks the classifier cannot know go into `oracleKnownGaps` with a
  reason, and version-specific differences into `oracleVersionGaps`.
  Locks a statement may take (partitions behind pruning, foreign key
  checks and row-level triggers that run only for affected rows) are
  accepted when absent; other cases supply the rows that make them appear.
* The oracle runs on pull requests that change `internal/sqlinspect` and
  nightly on PostgreSQL 15 to 18.
* Every change to lock classification adds its statements to the oracle
  (CONTRIBUTING).

## Consequences

* A wrong prediction fails CI on the version where it is wrong.
* The oracle's schema has to contain the objects each case needs, which
  makes it a growing fixture; cases whose locks depend on data need data
  (referencing rows, a statement-level trigger).
