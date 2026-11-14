# QUAL012 · compatibility-not-checked

| Default severity | Evidence |
|---|---|
| info | experiment quality |

## What it detects

A static finding of `PS109` (a dropped table, column or other object) or `PS110` (a rename) that is not suppressed, while the workload has no `compatibility` section.

## Why it matters

Drops and renames are safe only once no running code uses the old name, which the migration alone cannot show. Compatibility probes run the previous release's queries before and after the migration and report a release that breaks as `OBS013`; without them the experiment does not answer the question these findings raise.

## Example

Flagged:

```sql
# migration: ALTER TABLE products DROP COLUMN legacy_sku;
scenarios:
  - {id: get_product, weight: 1, mode: read, sql: "SELECT name, sku FROM products WHERE id = $1", parameters: [{generator: int_range, min: 1, max: 20000}]}
```

Preferred:

```sql
compatibility:
  - version: "2026.10"   # the release still running during the deploy
    probes:
      - id: get_product
        sql: "SELECT name, legacy_sku FROM products WHERE id = $1"
        parameters: [{generator: int_range, min: 1, max: 20000}]
```

## How ProdShadow verifies it

Derived from the static findings and the workload definition.

## Configuration

Change the severity or turn the rule off in `prodshadow.yaml`:

```yaml
policy:
  rules:
    QUAL012: critical   # or info, warning, off
```

Suppress a reviewed case in the suppressions file (`policy.suppressions`):

```yaml
version: 1
suppressions:
  - rule: QUAL012
    reason: "why this is acceptable"
    expires: 2027-06-30
```

## References

- [Compatibility probes](../workload.md#compatibility-probes)
- [OBS013](OBS013.md)
