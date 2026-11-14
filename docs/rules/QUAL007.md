# QUAL007 · no-experiment

| Default severity | Evidence |
|---|---|
| inconclusive | experiment quality |

## What it detects

No workload or no fixture is configured, so only static analysis ran.

## Why it matters

Static rules cannot see lock queues, failures on real data or latency. Without an experiment the run cannot pass; it is inconclusive.

## Example

Flagged:

```yaml
workload: {}
```

Preferred:

```yaml
workload:
  file: ./workloads/workload.yaml
```

## How ProdShadow verifies it

Run `prodshadow init` to create a starter fixture and workload.

## Configuration

Change the severity or turn the rule off in `prodshadow.yaml`:

```yaml
policy:
  rules:
    QUAL007: critical   # or info, warning, off
```

Suppress a reviewed case in the suppressions file (`policy.suppressions`):

```yaml
version: 1
suppressions:
  - rule: QUAL007
    reason: "why this is acceptable"
    expires: 2027-06-30
```

## References

- [docs/workload.md](../workload.md)
