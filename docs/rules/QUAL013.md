# QUAL013 · scenario-always-fails

| Default severity | Evidence |
|---|---|
| inconclusive | experiment quality |

## What it detects

A workload scenario failed at least 90 % of its operations across the baseline phases (at least 10 operations), before any migration ran. The finding quotes the scenario's first error message, with its SQLSTATE.

## Why it matters

A scenario that fails without any migration is broken: a misspelled table or column, a parameter type PostgreSQL cannot infer, or a missing privilege. Its latency and error rate say nothing about the migration, and the comparison for it is meaningless, so the run cannot be trusted to pass. Previously this showed up only as too few samples (`QUAL001`).

## Example

Flagged:

```sql
- id: record_event
  mode: write
  # $1 is used as bigint and as integer: PostgreSQL cannot deduce one type
  sql: "INSERT INTO events (id, at) VALUES ($1, now() + make_interval(secs => $1 % 1000))"
```

Preferred:

```sql
- id: record_event
  mode: write
  sql: "INSERT INTO events (id, at) VALUES ($1::bigint, now() + ($1::bigint % 1000) * interval '1 second')"
```

## How ProdShadow verifies it

Each scenario's first error is kept per phase (`phases[].scenarios[].sample_error`, redacted) together with the error counts.

## Configuration

Change the severity or turn the rule off in `prodshadow.yaml`:

```yaml
policy:
  rules:
    QUAL013: critical   # or info, warning, off
```

Suppress a reviewed case in the suppressions file (`policy.suppressions`):

```yaml
version: 1
suppressions:
  - rule: QUAL013
    reason: "why this is acceptable"
    expires: 2027-06-30
```

## References

- [Workload reference](../workload.md#scenarios)
- [Troubleshooting](../troubleshooting.md)
