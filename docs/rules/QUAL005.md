# QUAL005 · sampler-starved

| Default severity | Evidence |
|---|---|
| warning | experiment quality |

## What it detects

Lock samples were more than five sampling intervals apart, or some samples failed.

## Why it matters

The sampler, the workload and the server share the host. When sampling falls behind, the host is overloaded, which also distorts latency measurements.

## Example

Flagged:

```yaml
workload:
  concurrency: 64
postgres:
  isolation:
    cpus: 1
```

Preferred:

```yaml
workload:
  concurrency: 8
postgres:
  isolation:
    cpus: 2
```

## How ProdShadow verifies it

Reduce concurrency, give the server more CPU, or use a larger runner.

## Configuration

Change the severity or turn the rule off in `prodshadow.yaml`:

```yaml
policy:
  rules:
    QUAL005: critical   # or info, warning, off
```

Suppress a reviewed case in the suppressions file (`policy.suppressions`):

```yaml
version: 1
suppressions:
  - rule: QUAL005
    reason: "why this is acceptable"
    expires: 2027-06-30
```

## References

- [docs/architecture.md](../architecture.md)
