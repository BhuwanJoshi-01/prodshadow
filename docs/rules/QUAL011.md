# QUAL011 · table-not-exercised

| Default severity | Evidence |
|---|---|
| warning | experiment quality |

## What it detects

The migration takes a lock that blocks reads or writes on a table that existed before it, and no workload scenario or background session reads or writes that table (judged from the tables their SQL names).

## Why it matters

The experiment measures what the migration does to the workload. If nothing in the workload touches the locked table, every number in the report is about other tables, and a migration that would stall all traffic on that table in production can pass. Tables created by the migration itself are not reported, since nothing uses them yet.

## Example

Flagged:

```sql
# migration: ALTER TABLE invoices ADD COLUMN note text;
scenarios:
  - {id: read_order, weight: 1, mode: read, sql: "SELECT status FROM orders WHERE id = $1", parameters: [{generator: int_range, min: 1, max: 100000}]}
```

Preferred:

```yaml
scenarios:
  - {id: read_order, weight: 2, mode: read, sql: "SELECT status FROM orders WHERE id = $1", parameters: [{generator: int_range, min: 1, max: 100000}]}
  - {id: read_invoice, weight: 1, mode: read, sql: "SELECT total FROM invoices WHERE id = $1", parameters: [{generator: int_range, min: 1, max: 50000}]}
```

## How ProdShadow verifies it

Derived from the migration's predicted locks and the relations named in the workload's SQL.

## Configuration

Change the severity or turn the rule off in `prodshadow.yaml`:

```yaml
policy:
  rules:
    QUAL011: critical   # or info, warning, off
```

Suppress a reviewed case in the suppressions file (`policy.suppressions`):

```yaml
version: 1
suppressions:
  - rule: QUAL011
    reason: "why this is acceptable"
    expires: 2027-06-30
```

## References

- [Workload reference](../workload.md)
- [Modelling a workload](../guides/modelling-traffic.md)
