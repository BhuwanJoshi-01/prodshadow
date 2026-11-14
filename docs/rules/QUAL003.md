# QUAL003 · representativeness

| Default severity | Evidence |
|---|---|
| info | experiment quality |

## What it detects

Always reported: the fixture's declared representativeness (`fixture.representation`) and its row count.

## Why it matters

Lock behaviour depends on lock modes and concurrency and is reproduced faithfully. Durations depend on data size and hardware and are not predictive of production; this finding keeps that limitation next to every result.

## Example

Flagged:

```sql
fixture:
  representation: manual-fixture
```

Preferred:

```sql
fixture:
  representation: synthetic
  generator:
    path: ./fixtures/generate.yaml
```

## How ProdShadow verifies it

—

## Configuration

Change the severity or turn the rule off in `prodshadow.yaml`:

```yaml
policy:
  rules:
    QUAL003: critical   # or info, warning, off
```

Suppress a reviewed case in the suppressions file (`policy.suppressions`):

```yaml
version: 1
suppressions:
  - rule: QUAL003
    reason: "why this is acceptable"
    expires: 2027-06-30
```

## References

- [docs/limitations.md](../limitations.md)
