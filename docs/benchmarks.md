# Measured results

These are the results of running every example with its committed
configuration (3 repeats, 3 s warm-up, 15 s measurement, 8 workload
sessions). They are published so that you can compare your own runs; they
are **not** limits or guarantees.

## Environment

| | |
|---|---|
| Host | 4 vCPU (Intel Xeon @ 2.1 GHz), 15 GiB RAM, Linux |
| Container engine | Docker 29.9.2 |
| Server | PostgreSQL 17.12, `postgres:17-alpine`, 2 CPUs, 1–2 GiB memory, tmpfs data |
| ProdShadow | development build, November 2026 |

## Results

| Example | Rows | Decision | Wall time | Migration duration per repeat | Sessions queued behind migration | Longest stall beyond baseline p99 | Baseline noise (p95 CV) |
|---|---|---|---|---|---|---|---|
| safe-add-column | 50,000 | pass | 125 s | 2, 4, 2 ms | 0 | 4.5 ms | 1.2 % |
| long-running-transaction | 100,000 | block (OBS002) | 125 s | 2002, 2001, 2001 ms (lock timeout) | 8 | 2001 ms | 3.0 % |
| blocking-index | 800,000 | block (OBS001) | 128 s | 691, 819, 677 ms | 8 | 820 ms | 5.0 % |
| blocking-index, `CONCURRENTLY` | 800,000 | pass | 128 s | 1274, 992, 1173 ms | 0 | 40 ms | 1.5 % |
| backfill-heavy | 400,000 | block (OBS001) | 126 s | 1671, 1544, 1619 ms | 8 | 1656 ms | 10 % |
| rolling-deploy | 20,000 | block (OBS007, OBS005) | 125 s | 3, 3, 6 ms | 0 | 8 ms | 4.5 % |

Observations:

* **Lock queues are deterministic.** In every blocking example all eight
  workload sessions queued behind the migration in all three repeats, which
  is why those findings carry *high* confidence.
* **Reads were unaffected by `SHARE` and row locks.** In `blocking-index`
  and `backfill-heavy` only write scenarios stalled, matching the lock
  conflict matrix.
* **`CONCURRENTLY` traded blocking for duration.** The concurrent build took
  roughly 50 % longer but no session queued; the 40 ms stall is CPU
  contention on the 2-CPU server, not lock waiting.
* **Equivalent phases agree.** An experiment with a no-op migration
  measured a p95 ratio of 1.04 between baseline and candidate (integration
  test `TestEqualPhasesStayWithinNoise`).
* **Telemetry kept up.** The largest gap between lock samples was 130 ms
  for a 100 ms interval.
* **Error storms are visible.** In `rolling-deploy`, failing old-version
  queries raised the new version's p95 by about 1.7×.

## Overhead

Each phase costs roughly warm-up + measurement + 3 s (database copy,
checkpoint, statistics flush). Server start-up and seeding took 2–4 s for
these fixtures; generating 800,000 rows through `COPY` took 1.7 s.

### Client-side cost per operation

Measured with `go test -bench` on the same host:

| Operation | Cost |
|---|---|
| Record one latency sample (histogram) | ~18 ns |
| Summarise a histogram (percentiles) | ~2 µs |
| Draw an `int_range` / `zipf` / `uuid` parameter | ~17 / ~66 / ~314 ns |
| Split SQL | ~180 MB/s |

The measurement machinery is therefore negligible next to a round trip to
the database (0.1–1 ms in these examples).

## Reproducing

```bash
make build
for e in safe-add-column long-running-transaction blocking-index backfill-heavy rolling-deploy; do
  ./bin/prodshadow verify --config examples/$e/prodshadow.yaml --output /tmp/ps-$e
done
```

Please share results from other hardware in an issue; they help calibrate
default thresholds.
