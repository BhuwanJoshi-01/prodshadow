# QUAL008 · no-blocker-modelled

| Default severity | Evidence |
|---|---|
| info | experiment quality |

## What it detects

No session keeps a transaction open when the migration starts: no background session is active at `migration.start_after_seconds`, and no scenario uses `hold_ms`.

## Why it matters

Most migration outages need a long-running or idle-in-transaction session. Without one, the experiment cannot show the lock queue that turns a millisecond `ALTER TABLE` into an outage.

## Example

Flagged:

```yaml
background: []
```

Preferred:

```yaml
background:
  - id: nightly_report
    sessions: 1
    start_after_seconds: 2
    repeat: true
    transaction:
      statements: ["SELECT count(*) FROM orders"]
      hold_ms: 6000
```

## How ProdShadow verifies it

—

## Configuration

Change the severity or turn the rule off in `prodshadow.yaml`:

```yaml
policy:
  rules:
    QUAL008: critical   # or info, warning, off
```

Suppress a reviewed case in the suppressions file (`policy.suppressions`):

```yaml
version: 1
suppressions:
  - rule: QUAL008
    reason: "why this is acceptable"
    expires: 2027-06-30
```

## References

- [examples/long-running-transaction](../../examples/long-running-transaction)
