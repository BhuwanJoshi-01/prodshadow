# Rules

Every finding states its rule, severity, confidence (with the reason),
evidence kind (`static`, `heuristic`, `observed`, `experiment-quality`),
location and — where applicable — the observed value and the threshold.
`prodshadow rules` prints this catalogue; `policy.rules` changes a rule's
severity or turns it `off`.

Static rules explain *why* a statement can block; observed rules report
*what happened* in the experiment. ProdShadow deliberately keeps the static
set small and conservative — keep using your migration linter alongside it.

Static findings are skipped for tables created earlier in the same
migration, because nothing else uses them yet.

## Static rules

| ID | Name | Default | What it flags |
|---|---|---|---|
| `PS101` | create-index-blocks-writes | warning | `CREATE INDEX` without `CONCURRENTLY` takes `SHARE`; writes wait for the whole build. |
| `PS102` | concurrent-operation-in-transaction | critical | `CREATE/DROP INDEX CONCURRENTLY`, `REINDEX CONCURRENTLY`, `VACUUM`, `DETACH PARTITION CONCURRENTLY` inside a transaction block — PostgreSQL rejects them. |
| `PS103` | add-column-rewrites-table | warning | `ADD COLUMN` with a volatile default (`gen_random_uuid()`, `clock_timestamp()`, …), serial, identity or stored generated column rewrites the table under `ACCESS EXCLUSIVE`. Unknown functions give a low-confidence heuristic finding. |
| `PS104` | add-column-not-null-without-default | warning | `ADD COLUMN … NOT NULL` without a default fails on any non-empty table. |
| `PS105` | alter-column-type | warning | `ALTER COLUMN … TYPE` usually rewrites the table and its indexes; heuristic, because some conversions (e.g. `varchar(n)` → `text`) do not. |
| `PS106` | set-not-null-scans-table | warning | `SET NOT NULL` scans every row under `ACCESS EXCLUSIVE` unless a validated `CHECK (col IS NOT NULL)` exists. |
| `PS107` | constraint-validated-in-place | warning | Foreign key or `CHECK` added without `NOT VALID` — including an inline `ADD COLUMN … REFERENCES`, which cannot be `NOT VALID` — validates all rows while holding a lock. |
| `PS108` | unique-constraint-builds-index | warning | `UNIQUE`, `PRIMARY KEY` or `EXCLUDE` constraint without `USING INDEX` builds its index under `ACCESS EXCLUSIVE`. |
| `PS109` | destructive-change | warning | `DROP TABLE`, `DROP COLUMN`, `TRUNCATE`: data loss and application compatibility need explicit review. |
| `PS110` | rename-breaks-running-code | warning | Renaming a table or column breaks instances still using the old name. |
| `PS111` | unbounded-dml | warning | `UPDATE`/`DELETE` without `WHERE` locks every row in one transaction. |
| `PS112` | explicit-table-lock | warning | `LOCK TABLE` in a mode that blocks writes or reads. |
| `PS113` | rewrite-under-exclusive-lock | warning | `VACUUM FULL`, `CLUSTER`, non-concurrent `REINDEX` and `REFRESH MATERIALIZED VIEW`, `SET TABLESPACE/LOGGED/UNLOGGED/ACCESS METHOD`. |
| `PS114` | missing-lock-timeout | info | A blocking lock is requested before any `SET lock_timeout`. |
| `PS115` | lock-held-across-statements | warning | A blocking lock is held until `COMMIT` while later statements of the same transaction run. Uses the same transaction plan as the executor. |
| `PS116` | statement-not-analysed | inconclusive | The bundled PostgreSQL 17 grammar could not parse the statement. It is still executed. Set to `info` if you rely on newer syntax. |
| `PS117` | procedural-code | info | `DO` blocks and `CALL` cannot be analysed statically. |
| `PS118` | trigger-change-blocks-writes | info | `CREATE TRIGGER` and `ENABLE/DISABLE TRIGGER` take `SHARE ROW EXCLUSIVE`. |
| `PS119` | foreign-key-locks-referenced-table | info | A new table's foreign key takes `SHARE ROW EXCLUSIVE` on the referenced table. |
| `PS120` | ineffective-lock-timeout | warning | `SET lock_timeout = 0`, or a value above 10 s: queries can queue behind a waiting migration for that long. |
| `PS121` | concurrent-index-caveats | info | `CREATE INDEX CONCURRENTLY` waits for older transactions and leaves an `INVALID` index if it fails. |
| `PS122` | drop-index-blocks-traffic | warning | `DROP INDEX` without `CONCURRENTLY` takes `ACCESS EXCLUSIVE` on the owning table. |

## Observed rules

| ID | Name | Default | Evaluated as |
|---|---|---|---|
| `OBS001` | lock-wait-budget | critical | Longest wait of any workload session while the migration ran — the larger of the sampled lock wait and the client-side stall beyond the baseline p99 — exceeds `policy.max_lock_wait_ms`. |
| `OBS002` | migration-lock-timeout | critical | The migration failed with `lock_not_available` (55P03). The message names the sessions it waited for. |
| `OBS003` | migration-failed | critical | Any other migration failure, with SQLSTATE and (redacted) message. |
| `OBS004` | migration-statement-timeout | critical | A statement exceeded `migration.statement_timeout_ms` (57014). |
| `OBS005` | p95-latency-regression | critical | Candidate/baseline p95 of a scenario exceeds `policy.max_p95_latency_ratio` **and** three times the baseline's coefficient of variation. Within the noise band, or on an unstable baseline, the finding is `inconclusive`. |
| `OBS006` | p99-latency-regression | critical | As `OBS005` for p99, when `policy.max_p99_latency_ratio` is set. |
| `OBS007` | error-rate-increase | critical | Error-rate increase in percentage points exceeds `policy.max_error_rate_increase_pct_points`. |
| `OBS008` | throughput-drop | critical | Throughput drop exceeds `policy.max_throughput_drop_pct` (optional). |
| `OBS009` | migration-duration | critical | Median migration duration exceeds `policy.max_migration_seconds` (optional). |
| `OBS010` | deadlocks | warning | Deadlocks counted in candidate phases. |
| `OBS011` | sessions-queued-behind-migration | info | How many sessions queued behind the migration's lock request, and what the migration waited for. |

Confidence of observed findings is `high` when every one of at least three
candidate repeats shows the effect, `medium` for a majority, `low` otherwise,
and is lowered when only client-side evidence exists.

## Experiment-quality rules

| ID | Name | Default | Meaning |
|---|---|---|---|
| `QUAL001` | insufficient-samples | inconclusive | A scenario had fewer than `workload.min_samples_per_scenario` successful operations in some phase. |
| `QUAL002` | unstable-baseline | inconclusive | Baseline p95 varied by more than 50 % between repeats. |
| `QUAL003` | representativeness | info | What the fixture claims to represent, and its size. Always reported. |
| `QUAL004` | few-repeats | info | Fewer than three repeats. |
| `QUAL005` | sampler-starved | warning | Telemetry samples were more than five intervals apart, or failed. |
| `QUAL006` | connect-failures | warning | Workload sessions failed to connect. |
| `QUAL007` | no-experiment | inconclusive | No workload or no fixture; only static analysis ran. |
| `QUAL008` | no-blocker-modelled | info | No session holds a transaction open when the migration starts, so the most common outage pattern — a lock queue behind a long transaction — is not exercised. |

## Suppressions

Suppressions must explain themselves and should expire.

**Inline**, in the comment directly above a statement:

```sql
-- prodshadow:ignore PS101 reason="lookup table with 40 rows" expires=2027-01-31
CREATE INDEX countries_name_idx ON countries (name);
```

Inline annotations also apply to observed findings that ProdShadow
attributes to the statement (for example `OBS001` or `OBS002`).

**In a file**, referenced by `policy.suppressions`:

```yaml
version: 1
suppressions:
  - rule: OBS005
    scenario: monthly_export        # optional: limit to a scenario
    file: migrations/0042.sql       # optional: limit to a file
    reason: "Export latency is not user-facing; accepted by the data team"
    expires: 2027-03-31             # optional, YYYY-MM-DD
    approved_by: data-team          # optional
```

A suppression without a reason is rejected. Suppressed findings remain in
the report (marked as suppressed with their reason) and do not affect the
decision. Expired suppressions are listed so they get renewed or removed.
