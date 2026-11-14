# QUAL001 · insufficient-samples

| Default severity | Evidence |
|---|---|
| inconclusive | experiment quality |

## What it detects

A foreground scenario completed fewer successful operations than `workload.min_samples_per_scenario` (default 200) in at least one phase.

## Why it matters

Percentiles from a handful of samples are noise. Rather than guessing, the latency comparison is reported as inconclusive.

## Example

Flagged:

```yaml
workload:
  measurement_seconds: 5
  min_samples_per_scenario: 200
```

Preferred:

```yaml
workload:
  measurement_seconds: 30   # or raise the scenario's weight
```

## How ProdShadow verifies it

The finding lists the undersampled scenarios and the smallest count observed.

## Configuration

Change the severity or turn the rule off in `prodshadow.yaml`:

```yaml
policy:
  rules:
    QUAL001: critical   # or info, warning, off
```

Suppress a reviewed case in the suppressions file (`policy.suppressions`):

```yaml
version: 1
suppressions:
  - rule: QUAL001
    reason: "why this is acceptable"
    expires: 2027-06-30
```

## References

- [docs/workload.md](../workload.md)
