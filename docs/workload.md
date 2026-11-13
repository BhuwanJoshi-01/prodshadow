# Workload reference

A workload describes the traffic that runs against both the baseline and the
candidate database. Schema: [`schemas/workload.v1.schema.json`](../schemas/workload.v1.schema.json).
Unknown keys are errors: a misspelled key would silently change the
experiment.

```yaml
version: 1
name: orders-api
think_time_ms: 0          # optional pause per closed-loop worker
scenarios:
  - id: recent_orders
    weight: 60
    mode: read            # read or write; used to group results
    sql: "SELECT id, status FROM orders WHERE customer_id = $1 ORDER BY created_at DESC LIMIT 20"
    parameters:
      - {generator: zipf, min: 1, max: 100000, s: 1.1}
  - id: update_order_status
    weight: 30
    mode: write
    sql: "UPDATE orders SET status = $1 WHERE id = $2"
    parameters:
      - {generator: choice, values: [processing, shipped]}
      - {generator: int_range, min: 1, max: 1000000}
  - id: lock_order
    weight: 10
    mode: write
    transaction:
      statements:
        - "SELECT id FROM orders WHERE id = $1 FOR UPDATE"
      hold_ms: 200        # keep the transaction open before COMMIT
    parameters:
      - {generator: int_range, min: 1, max: 1000000}
background:
  - id: nightly_report
    sessions: 1
    start_after_seconds: 2
    repeat: true
    transaction:
      statements: ["SELECT status, count(*) FROM orders GROUP BY status"]
      hold_ms: 6000
```

## Scenarios

| Key | Description |
|---|---|
| `id` | Lower-case identifier, unique across scenarios and background sessions. |
| `weight` | Relative frequency in the foreground mix. |
| `mode` | `read` or `write`. |
| `sql` | One statement. Mutually exclusive with `transaction`. |
| `transaction.statements` | Statements run in one transaction; they share the parameter list. |
| `transaction.hold_ms` | Keep the transaction idle, holding its locks, before committing. Not counted in latency. |
| `parameters` | Generators for `$1`, `$2`, …; every referenced parameter must be defined. |
| `timeout_ms` | Session `statement_timeout` for this scenario (default 30 s). |

Only `SELECT` (including `FOR UPDATE`/`FOR SHARE` and CTEs), `INSERT`,
`UPDATE` and `DELETE` are allowed. DDL, `COPY`, `SET`, transaction control
and `SELECT INTO` are rejected when the workload is loaded.

Workload sessions connect as the unprivileged owner role with
`application_name` `prodshadow/worker/<n>`; this is how lock waits are
attributed.

## Background sessions

Background sessions run outside the weighted mix to model long-running
work. They matter because most migration incidents need one: a transaction
holding even a weak lock forces a migration's `ACCESS EXCLUSIVE` request to
wait, and everything queues behind that request.

| Key | Description |
|---|---|
| `id` | Identifier (`prodshadow/background/<id>/<n>` in telemetry). |
| `sessions` | Number of identical sessions (1–64). |
| `start_after_seconds` | Start offset relative to the measurement window. |
| `transaction` | Statements and `hold_ms`, as above. |
| `parameters` | Generators, as above. |
| `repeat` | Start a new transaction after each one finishes. |

## Scheduling

* **Closed loop** (default): `workload.concurrency` sessions run operations
  back to back. When the database blocks, fewer operations are issued and
  throughput falls; the blocked operations show the wait in their latency.
* **Open loop**: set `workload.arrival_rate` (operations per second).
  Arrivals follow a Poisson process and are dispatched to
  `workload.concurrency` sessions; latency is measured from the intended
  arrival time. Arrivals that find every session busy are recorded as
  `dropped` errors.

Scenario choice and parameters come from deterministic streams derived from
`workload.seed`, the repeat number and the session. The baseline and the
candidate of a repeat therefore issue the same parameter sequences.

## Generators

Used for workload parameters and fixture columns. Every generator accepts
`null_ratio` (probability of `NULL`).

| Generator | Keys | Produces |
|---|---|---|
| `int_range` | `min`, `max` (integers, inclusive) | Uniform integers |
| `float_range` | `min`, `max` | Uniform floats |
| `zipf` | `min`, `max`, `s` (> 1) | Skewed integers; `min` is the hottest key |
| `choice` | `values`, optional `weights` | One of the values |
| `const` | `value` | The value |
| `sequence` | `start` (1), `step` (1) | Increasing integers per stream |
| `text` | `min_length`, `max_length` | Random alphanumeric text |
| `uuid` | — | Version 4 UUIDs (deterministic) |
| `timestamp_range` | `from`, `to` (RFC 3339) | Uniform timestamps |
| `pattern` | `format` containing `{n}` | e.g. `user{n}@example.test` |
| `bool` | `true_ratio` (0.5) | Booleans |

## Fixture generator

`fixture.generator.path` points at a file that fills existing tables with
deterministic rows using `COPY`:

```yaml
version: 1
tables:
  - name: orders            # must exist (create it in fixture.schema)
    rows: 800000
    columns:
      - {name: id, generator: sequence}
      - {name: customer_id, generator: zipf, min: 1, max: 50000, s: 1.1}
      - {name: status, generator: choice, values: [pending, paid], weights: [3, 7]}
      - {name: created_at, generator: timestamp_range, from: "2025-01-01T00:00:00Z", to: "2026-01-01T00:00:00Z"}
```

Values depend only on `fixture.generator.seed`, the table and the column
name, so adding a column does not change the others. Columns not listed use
their defaults. The total is capped by `fixture.max_rows`.
