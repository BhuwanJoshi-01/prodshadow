# Choosing policy thresholds

The policy turns measurements into a decision. Defaults are deliberately
generic; set them from what your application can tolerate, not from what a
first run happens to produce.

## `max_lock_wait_ms` (default 2000)

The longest any workload session may wait while the migration runs
(`OBS001`). Start from the shortest timeout on the request path: an HTTP
handler with a 3 s timeout and a database client with a 2 s statement
timeout can tolerate well under 2 s. Leave a margin for the rest of the
request.

## `migration.lock_timeout_ms` and `lock_retries`

Not a policy, but they decide how long others wait: a waiting migration
holds everyone behind it for up to `lock_timeout_ms`. Keep it below
`max_lock_wait_ms`, often 1 s or less, and rehearse the deploy's retries
with `lock_retries` (`OBS014`).

## `max_p95_latency_ratio` (default 1.5) and `max_p99_latency_ratio`

How much slower a scenario may get while the migration runs (`OBS005`,
`OBS006`). Ratios carry over between fast and slow runners, unlike absolute
numbers. Check the baseline noise in the report first: a ratio limit close
to the noise band produces inconclusive results, so either lengthen
`measurement_seconds` or accept a looser ratio.

## `max_error_rate_increase_pct_points` (default 1)

Errors per hundred operations the migration may add (`OBS007`). Errors from
dropped or renamed objects are better caught with compatibility probes
(`OBS013`), which name the release that breaks.

## `max_queued_sessions` (optional)

How many sessions may queue behind the migration at once (`OBS012`). Set it
below the application's connection pool size per instance: a queue that
reaches the pool size means new requests cannot get a connection at all.
`validate` warns if the workload cannot produce that many sessions.

## `max_migration_seconds` (optional)

A ceiling on the migration's duration (`OBS009`). Durations scale with data
size, so derive it from the fixture: if a statement takes 3 s on a fixture
one hundredth of production size, expect minutes in production.

## `max_extra_wal_mb` (optional)

WAL the migration may write beyond the baseline (`OBS015`). Relate it to
how much WAL your replicas can apply per second and the replication lag you
accept, scaled by the fixture's size relative to production.

## `fail_on` and `inconclusive`

`fail_on: [critical]` blocks on critical findings only; add `warning` once
the warnings are under control. Keep `inconclusive: fail` unless you
understand why runs are inconclusive; [ADR 0004](../adr/0004-inconclusive-is-not-a-pass.md)
explains why an inconclusive result is not a pass.

## Per-rule overrides

`policy.rules` raises or lowers individual rules, or turns them off:

```yaml
policy:
  rules:
    PS133: off        # we index foreign keys in a separate review step
    QUAL011: info     # the workload deliberately covers only hot tables
```

Prefer a suppression with a reason and expiry for a single reviewed case.
