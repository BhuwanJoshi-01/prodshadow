# QUAL014 · sessions-saturated

| Default severity | Evidence |
|---|---|
| info | experiment quality |

## What it detects

In a closed-loop run (no `workload.arrival_rate`), all `workload.concurrency` sessions were queued behind the migration at the same moment in a candidate phase, and a scenario that the lock modes predict to conflict with the migration showed (almost) no stall: less than a tenth of the lock-wait budget.

## Why it matters

Closed-loop sessions send their next query only when the previous one returns. When the migration blocks one table, every session soon issues a query against it and waits there, and no session is left to run the scenarios that would wait on another table the migration locks. The report then shows those scenarios as unaffected, although production clients keep arriving and would queue behind the migration there as well. The [foreign-key-drop example](../../examples/foreign-key-drop) shows the effect: invoice queries stall, customer reads do not, because every session is already waiting on `invoices`.

## Example

Flagged:

```yaml
workload:
  concurrency: 8
  # closed loop: at most 8 queries in flight
```

Preferred:

```yaml
workload:
  concurrency: 32
  arrival_rate: 400   # open loop: requests keep arriving while sessions wait
```

## How ProdShadow verifies it

Compare *Predicted conflict* with *Max during migration* in the scenario comparison; the phases table shows how many sessions queued behind the migration.

## Configuration

Change the severity or turn the rule off in `prodshadow.yaml`:

```yaml
policy:
  rules:
    QUAL014: critical   # or info, warning, off
```

Suppress a reviewed case in the suppressions file (`policy.suppressions`):

```yaml
version: 1
suppressions:
  - rule: QUAL014
    reason: "why this is acceptable"
    expires: 2027-06-30
```

## References

- [Workload reference](../workload.md)
- [Modelling traffic](../guides/modelling-traffic.md)
- [OBS011](OBS011.md)
