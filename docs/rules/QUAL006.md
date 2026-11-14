# QUAL006 · connect-failures

| Default severity | Evidence |
|---|---|
| warning | experiment quality |

## What it detects

Workload sessions failed to connect.

## Why it matters

Missing sessions change the load being measured. The usual causes are too many sessions for `max_connections` (which `validate` checks) or a server killed for exceeding its memory limit.

## Example

Flagged:

```yaml
workload:
  concurrency: 120
```

Preferred:

```sql
postgres:
  settings:
    max_connections: "200"
```

## How ProdShadow verifies it

—

## Configuration

Change the severity or turn the rule off in `prodshadow.yaml`:

```yaml
policy:
  rules:
    QUAL006: critical   # or info, warning, off
```

Suppress a reviewed case in the suppressions file (`policy.suppressions`):

```yaml
version: 1
suppressions:
  - rule: QUAL006
    reason: "why this is acceptable"
    expires: 2027-06-30
```

## References

- [max_connections](https://www.postgresql.org/docs/current/runtime-config-connection.html#GUC-MAX-CONNECTIONS)
