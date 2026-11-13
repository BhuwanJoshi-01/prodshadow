# ProdShadow report: orders-api

**Decision: BLOCK** · exit code 2 · status `completed_with_findings`

Blocking findings: OBS002, OBS005.

> ProdShadow measures how a migration behaves in a disposable database under a modelled workload. A passing result is not a guarantee that the migration is safe in production.

| Run | PostgreSQL | Runtime | Fixture | Workload | Duration |
|---|---|---|---|---|---|
| `run-1` | 17.12 | docker, network none | manual-fixture, 20,000 rows | closed-loop, 16 sessions, 3 × 45 s | 2m5s |

## Findings

| Severity | Rule | Confidence | Evidence | Where | Finding |
|---|---|---|---|---|---|
| **critical** | OBS002 migration-lock-timeout | high (observed in 3 of 3 candidate repeats) | observed | migrations/0042_add_note.sql:2 | The migration could not acquire its lock within lock_timeout (1500 ms) because prodshadow/background/report/0 held a conflicting lock. In production, without a lock_timeout, it would have waited — and every query queued behind it. |
| **critical** | OBS005 p95-latency-regression | high (median of 3 repeats; baseline noise 0%) | observed | scenario `read_order` | p95 latency of read_order rose 3.00× (limit 1.50×). |
| info | OBS011 sessions-queued-behind-migration | high (observed in 3 of 3 candidate repeats) | observed | – | Up to 4 workload session(s) queued behind the migration's lock request; the longest sampled wait was 1450 ms. The migration itself waited for: prodshadow/background/report/0. |
| info | PS114 missing-lock-timeout | high | static | migrations/0042_add_note.sql:2 | ALTER TABLE requests ACCESS EXCLUSIVE on orders without a lock_timeout. If a long transaction holds a conflicting lock, the request queues indefinitely and every later query on orders queues behind it. ProdShadow applies migration.lock_timeout_ms during rehearsal; production runs need the same protection. |
| info | QUAL003 representativeness | high (declared in fixture.representation) | experiment-quality | – | Fixture representativeness: manual-fixture (20000 rows). Lock behaviour depends on lock modes and concurrency and is reproduced; durations depend on data size and hardware and are not predictive of production. |

### Remediation

- **OBS002** (migrations/0042_add_note.sql:2): Run the statement with a short lock_timeout and retry with backoff, and avoid deploying while long transactions are open. [PostgreSQL docs](https://www.postgresql.org/docs/current/runtime-config-client.html#GUC-LOCK-TIMEOUT)

## Scenario comparison

Medians across 3 repeat(s). *During migration* covers only the time the migration ran.

| Scenario | Mode | p95 baseline → candidate | p95 ratio | Max during migration | Error rate Δ | Throughput Δ | Baseline noise | Ops (B / C) |
|---|---|---|---|---|---|---|---|---|
| read_order | read | 1.0 ms → 3.0 ms | 3.00× | 1500 ms | +0.00 pp | +0.0% | 0% | 3,000 / 3,000 |

## Migration

Executed with `lock_timeout = 1500 ms`, `statement_timeout = 60000 ms`, transaction mode `auto`, as `owner`.

| ID | Location | Statement | Predicted locks | Outcome per repeat | Duration (max) |
|---|---|---|---|---|---|
| 1.1 | migrations/0042_add_note.sql:2 | ALTER TABLE | ACCESS EXCLUSIVE on orders | failed 55P03 ×3 | 1500 ms |

## Lock telemetry (candidate phases)

Sampled every 100 ms from `pg_stat_activity` and `pg_locks`; waits shorter than the interval can be missed.

| Repeat | Migration waited | Migration blocked by | Sessions queued behind migration | Longest workload lock wait | Samples |
|---|---|---|---|---|---|
| 1 | 1490 ms | prodshadow/background/report/0 | 4 | 1450 ms | 50 |
| 2 | 1490 ms | prodshadow/background/report/0 | 4 | 1450 ms | 50 |
| 3 | 1490 ms | prodshadow/background/report/0 | 4 | 1450 ms | 50 |

## Timeline (candidate repeat 1)

Foreground operations per second; `M` marks seconds in which the migration was running.

```text
  sec      ops  errors      p95       max
     0      100       0   1.0 ms    4.0 ms
M    1        4       0  1400 ms   1500 ms
```

## Experiment quality

- Representativeness: **manual-fixture**
- Pairs completed: 3 of 3
- Minimum successful operations per scenario and phase: 1,000 (required 10)
- Largest baseline noise (p95 coefficient of variation): 0%
- Telemetry: interval 100 ms, largest gap 0.00 ms, failed samples 0

**Limitations**

- Lock waits shorter than the sampling interval may be missed by server-side sampling; client-side latency covers them.
- Durations of rewrites and index builds scale with data size and hardware; a small fixture understates them.
- Passing this experiment does not guarantee a safe production migration.
- The fixture is manual-fixture; it is not claimed to match production data distribution or scale.

## Reproduction

| Item | Value |
|---|---|
| Tool | prodshadow v0.1.0-test (commit test, go1.26, linux/amd64) |
| Report schema | 1.0.0 |
| Git commit | `0123abcd` |
| Image | `postgres:17-alpine` |
| Server | PostgreSQL 17.12 on x86_64 |
| Settings hash | `sha256:abc` |
| Resource limits | 2 CPUs, 2048 MiB memory, 4096 MiB disk |
| Fixture hash | `sha256:fx` |
| Workload hash | `sha256:wl` |
| Workload seed | 1 |
| Parser | PostgreSQL 17 (libpg_query via pg_query_go v6) |

