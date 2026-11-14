# QUAL002 · unstable-baseline

| Default severity | Evidence |
|---|---|
| inconclusive | experiment quality |

## What it detects

A scenario's baseline p95 varied by more than 50 % (coefficient of variation) across repeats.

## Why it matters

If identical baselines disagree that much, a candidate difference proves nothing. Shared CI runners, too-short measurements and tiny fixtures are common causes.

## Example

Flagged:

```yaml
workload:
  repeats: 3
  measurement_seconds: 10
```

Preferred:

```yaml
workload:
  repeats: 5
  measurement_seconds: 45
```

## How ProdShadow verifies it

Latency findings on noisy scenarios become inconclusive; lock and failure findings are unaffected.

## Configuration

Change the severity or turn the rule off in `prodshadow.yaml`:

```yaml
policy:
  rules:
    QUAL002: critical   # or info, warning, off
```

Suppress a reviewed case in the suppressions file (`policy.suppressions`):

```yaml
version: 1
suppressions:
  - rule: QUAL002
    reason: "why this is acceptable"
    expires: 2027-06-30
```

## References

- [docs/limitations.md](../limitations.md)
