# Modelling a workload from production statistics

ProdShadow never connects to production (see
[ADR 0002](../adr/0002-no-production-connections.md)). To make the workload
resemble production, run a few read-only queries yourself, on a replica or
with a read-only role, and translate the results into the workload file by
hand. Review the output before it leaves the database: query text can
contain literals.

## 1. Which statements touch the table

With `pg_stat_statements` installed:

```sql
SELECT calls,
       round(calls / extract(epoch FROM now() - stats_reset)) AS per_second,
       round(mean_exec_time::numeric, 2) AS mean_ms,
       left(query, 120) AS query
FROM pg_stat_statements
JOIN pg_stat_statements_info ON true
WHERE query ILIKE '%orders%'
ORDER BY calls DESC
LIMIT 20;
```

Each frequent statement becomes a scenario. Use `calls` as the `weight`:
only the ratios matter. Mark `SELECT`s as `mode: read` and everything else
as `mode: write`. Replace the `$1` placeholders with generators: `int_range`
over the key range for uniform access, `zipf` when a few rows are hot (most
recent orders, popular products).

```yaml
scenarios:
  - id: order_by_id
    weight: 5400
    mode: read
    sql: "SELECT id, status, total FROM orders WHERE id = $1"
    parameters:
      - {generator: zipf, min: 1, max: 2000000, s: 1.2}
  - id: update_status
    weight: 800
    mode: write
    sql: "UPDATE orders SET status = $1 WHERE id = $2"
    parameters:
      - {generator: choice, values: [paid, shipped]}
      - {generator: zipf, min: 1, max: 2000000, s: 1.2}
```

## 2. How busy it is

`per_second` from the query above, summed over the scenarios, is the
arrival rate. Either set `workload.arrival_rate` to it (open loop, see
[ADR 0006](../adr/0006-measure-from-intended-start.md)) or choose
`workload.concurrency` near the number of application connections that use
the table at once:

```sql
SELECT count(*) FILTER (WHERE state = 'active') AS active,
       count(*) AS connected
FROM pg_stat_activity
WHERE datname = current_database() AND backend_type = 'client backend';
```

A small disposable server cannot match production throughput; what matters
is that enough sessions arrive while the migration waits for its lock.

## 3. The long transactions

Lock-queue outages need something holding a lock. Look for long
transactions and who runs them:

```sql
SELECT application_name, usename,
       now() - xact_start AS running_for, state,
       left(query, 80) AS current_query
FROM pg_stat_activity
WHERE xact_start < now() - interval '5 seconds'
ORDER BY xact_start;
```

Run it a few times over a day, or check the server log if
`log_min_duration_statement` is set. Each recurring job becomes a
background session whose `hold_ms` is a typical duration:

```yaml
background:
  - id: nightly_report
    sessions: 1
    start_after_seconds: 2
    repeat: true
    transaction:
      statements: ["SELECT customer_id, sum(total) FROM orders GROUP BY 1"]
      hold_ms: 30000
```

## 4. Data volume

`pg_class.reltuples` gives row counts without a full count:

```sql
SELECT relname, reltuples::bigint AS rows, pg_size_pretty(pg_total_relation_size(oid)) AS size
FROM pg_class
WHERE relkind = 'r' AND relnamespace = 'public'::regnamespace
ORDER BY pg_total_relation_size(oid) DESC
LIMIT 20;
```

Lock behaviour does not need production volume: a queue forms behind a
waiting migration on a table of any size. Durations of rewrites and index
builds do, so set the generator's row counts high enough that those
statements take noticeable time, and read their durations as relative, not
predictive (the report says so too).

## 5. Keep it current

Store the queries next to the workload file and repeat the exercise when
traffic changes. `prodshadow history` marks runs whose workload changed, so
results before and after are not compared by mistake.
