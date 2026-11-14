# QUAL009 · compatibility-probe-invalid

| Default severity | Evidence |
|---|---|
| inconclusive | experiment quality |

## What it detects

A compatibility probe failed before the migration ran, on the unchanged schema.

## Why it matters

A probe that fails on the original schema cannot show whether the migration breaks its version: usually the probe refers to a table or column that is not in the fixture, or it violates a constraint. That version's compatibility was not checked, so the result is inconclusive rather than a pass.

## Example

Flagged:

```sql
probes:
  - id: order_lines
    sql: "SELECT * FROM order_line WHERE order_id = 1"   # the table is order_lines
```

Preferred:

```sql
probes:
  - id: order_lines
    sql: "SELECT * FROM order_lines WHERE order_id = 1"
```

## How ProdShadow verifies it

The message includes the SQLSTATE and server message from the run before the migration.

## Configuration

Change the severity or turn the rule off in `prodshadow.yaml`:

```yaml
policy:
  rules:
    QUAL009: critical   # or info, warning, off
```

Suppress a reviewed case in the suppressions file (`policy.suppressions`):

```yaml
version: 1
suppressions:
  - rule: QUAL009
    reason: "why this is acceptable"
    expires: 2027-06-30
```

## References

- [Workload reference: compatibility probes](../workload.md#compatibility-probes)
