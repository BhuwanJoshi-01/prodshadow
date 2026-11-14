# QUAL010 · write-blocking-not-exercised

| Default severity | Evidence |
|---|---|
| info | experiment quality |

## What it detects

A migration statement takes a lock that blocks writes but not reads (`SHARE` from `CREATE INDEX`, `SHARE ROW EXCLUSIVE` from `CREATE TRIGGER` or a foreign key, `EXCLUSIVE`), and the workload has no `write` scenario.

## Why it matters

Readers are not affected by these locks, so a read-only workload shows no stall even when every insert and update in production would wait for the whole index build. The run can then pass without having measured the risk it was meant to measure.

## Example

Flagged:

```yaml
scenarios:
  - {id: read_order, weight: 1, mode: read, sql: "SELECT status FROM orders WHERE id = $1", parameters: [{generator: int_range, min: 1, max: 100000}]}
```

Preferred:

```yaml
scenarios:
  - {id: read_order, weight: 3, mode: read, sql: "SELECT status FROM orders WHERE id = $1", parameters: [{generator: int_range, min: 1, max: 100000}]}
  - {id: update_status, weight: 1, mode: write, sql: "UPDATE orders SET status = 'paid' WHERE id = $1", parameters: [{generator: int_range, min: 1, max: 100000}]}
```

## How ProdShadow verifies it

Derived from the migration's predicted locks and the workload's scenario modes.

## Configuration

Change the severity or turn the rule off in `prodshadow.yaml`:

```yaml
policy:
  rules:
    QUAL010: critical   # or info, warning, off
```

Suppress a reviewed case in the suppressions file (`policy.suppressions`):

```yaml
version: 1
suppressions:
  - rule: QUAL010
    reason: "why this is acceptable"
    expires: 2027-06-30
```

## References

- [Workload reference](../workload.md)
- [Table-level lock modes](https://www.postgresql.org/docs/current/explicit-locking.html#LOCKING-TABLES)
