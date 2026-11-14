# QUAL004 · few-repeats

| Default severity | Evidence |
|---|---|
| info | experiment quality |

## What it detects

Fewer than three baseline/candidate pairs were run.

## Why it matters

With one or two repeats the baseline's own variation cannot be estimated, so latency findings carry lower confidence.

## Example

Flagged:

```yaml
workload:
  repeats: 1
```

Preferred:

```yaml
workload:
  repeats: 3
```

## How ProdShadow verifies it

—

## Configuration

Change the severity or turn the rule off in `prodshadow.yaml`:

```yaml
policy:
  rules:
    QUAL004: critical   # or info, warning, off
```

Suppress a reviewed case in the suppressions file (`policy.suppressions`):

```yaml
version: 1
suppressions:
  - rule: QUAL004
    reason: "why this is acceptable"
    expires: 2027-06-30
```

## References

- [docs/configuration.md](../configuration.md)
