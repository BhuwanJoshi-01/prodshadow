# Configuration reference (`prodshadow.yaml`)

The machine-readable definition is
[`schemas/config.v1.schema.json`](../schemas/config.v1.schema.json)
(`prodshadow schema config`). Relative paths are resolved against the
directory that contains the configuration file. Unknown keys produce a
warning with a suggestion (`--strict` turns warnings into errors).

There is deliberately **no setting for a database address or credentials**.

| Key | Default | Description |
|---|---|---|
| `version` | — | Required. Only `1` is supported. |
| `project` | — | Required. Letters, digits, `.`, `_` or `-`; shown in reports. |

## `postgres`

| Key | Default | Description |
|---|---|---|
| `version` | `"17"` | PostgreSQL major version. 15–18 are tested; others produce a warning, 13 and older are refused. |
| `image` | `postgres:<version>-alpine` | Container image. Pin a digest (`@sha256:…`) in CI. A tag whose major differs from `version` produces a warning. |
| `database` | `app_test` | Name of each phase's database. `postgres`, `template*` and `ps_*` are reserved. |
| `extensions` | `[]` | Created by the administrative role before the fixture. Extensions that reach the network, server files or untrusted languages (`dblink`, `postgres_fdw`, `file_fdw`, `plpython3u`, …) are refused. |
| `settings` | `{}` | Server parameters passed at start-up, e.g. `shared_buffers: 256MB`. Parameters that run commands, load libraries or move sockets are refused. |

### `postgres.isolation`

| Key | Default | Description |
|---|---|---|
| `provider` | `docker` | `docker`, `podman` or `local` (installed binaries; no resource limits; not as root). |
| `network` | `none` | `none`: no network, unix socket shared through a private directory (Linux only). `loopback`: port published on `127.0.0.1` only. |
| `memory_mb` | `2048` | Container memory and swap limit. |
| `cpus` | `2` | Container CPU limit. |
| `ephemeral_disk_mb` | `4096` | Size of the tmpfs holding the data directory. |
| `pids_limit` | `512` | Maximum processes in the container. |
| `startup_timeout_seconds` | `120` | Time allowed for the server to become ready. |
| `binary` | — | Path of the container CLI, or the PostgreSQL `bin` directory for `local`. |

## `fixture`

| Key | Default | Description |
|---|---|---|
| `schema` | — | SQL file creating tables, indexes, constraints. |
| `seed_sql` | — | SQL file inserting data. |
| `generator.path` | — | Deterministic row generator ([format](workload.md#fixture-generator)). |
| `generator.seed` | `0` | Seed for generated rows. |
| `representation` | `manual-fixture` | `manual-fixture` or `synthetic`; shown in every report. |
| `max_rows` | `50000000` | Cap on generated rows. |

Fixture SQL runs as the unprivileged owner role. `ALTER SYSTEM`, role
management, `CREATE DATABASE`, `CREATE EXTENSION`, `LOAD` and `COPY` to or
from server files or programs are rejected. Without any fixture the run is
inconclusive.

## `workload`

| Key | Default | Description |
|---|---|---|
| `file` | — | Workload definition ([reference](workload.md)). Without it the run is inconclusive. |
| `warmup_seconds` | `10` | Discarded time before each measurement. |
| `measurement_seconds` | `45` | Length of each measurement window (candidate windows extend until the migration finishes). |
| `concurrency` | `16` | Client sessions running the weighted scenarios. |
| `arrival_rate` | `0` | Operations per second; when set, switches to open-loop scheduling. |
| `repeats` | `3` | Baseline/candidate pairs. Fewer than 3 cannot estimate noise. |
| `seed` | `1` | Seed for scenario choice and parameters. |
| `min_samples_per_scenario` | `200` | Fewer successful operations in any phase makes latency comparisons inconclusive. |

## `migration`

| Key | Default | Description |
|---|---|---|
| `files` | — | Migration files, executed in order. `--migration` overrides. |
| `lock_timeout_ms` | `1500` | Session `lock_timeout` for the migration. |
| `statement_timeout_ms` | `60000` | Session `statement_timeout` for the migration. |
| `start_after_seconds` | `5` | Delay into the measurement window, so pre-migration samples exist. Must be shorter than `measurement_seconds`. |
| `run_as` | `owner` | `owner` (unprivileged role that owns the fixture) or `superuser`. |
| `transaction` | `auto` | `auto`: wrap each file in a transaction unless it controls transactions itself or contains statements such as `CREATE INDEX CONCURRENTLY`; `per-file`: always wrap; `none`: autocommit each statement. |

## `policy`

| Key | Default | Description |
|---|---|---|
| `fail_on` | `[critical]` | Severities that block (exit 2). |
| `inconclusive` | `fail` | `fail` exits 5; `pass` exits 0 but the report still says inconclusive. |
| `max_lock_wait_ms` | `2000` | Longest wait any workload session may experience while the migration runs (`OBS001`). |
| `max_error_rate_increase_pct_points` | `1` | Per-scenario error-rate increase (`OBS007`). |
| `max_p95_latency_ratio` | `1.5` | Candidate/baseline p95 per scenario (`OBS005`). |
| `max_p99_latency_ratio` | — | Optional p99 ratio (`OBS006`). |
| `max_throughput_drop_pct` | — | Optional throughput drop (`OBS008`). |
| `max_migration_seconds` | — | Optional migration duration budget (`OBS009`). |
| `rules` | `{}` | Per-rule severity override or `off`, e.g. `{PS116: info}`. |
| `suppressions` | — | Audited suppression file ([format](rules.md#suppressions)). |

## `report`

| Key | Default | Description |
|---|---|---|
| `directory` | `./prodshadow-results` | Output directory (`--output` overrides). |
| `formats` | `[json, markdown]` | Any of `json`, `markdown`, `html`, `sarif`, `junit` (`--format` overrides). |

## `privacy`

| Key | Default | Description |
|---|---|---|
| `persist_raw_query_text` | `false` | Include migration and workload SQL in reports. |
| `persist_query_parameters` | `false` | Reserved; parameter values are never persisted in this version. |
| `redact_patterns` | `[]` | Extra regular expressions to redact from every message. |

## `telemetry`

| Key | Default | Description |
|---|---|---|
| `sample_interval_ms` | `100` | Polling interval for lock sampling (10–5000). |

## Environment overrides

Only operational settings can be overridden, never policy:

| Variable | Setting |
|---|---|
| `PRODSHADOW_POSTGRES_IMAGE` | `postgres.image` |
| `PRODSHADOW_ISOLATION_PROVIDER` | `postgres.isolation.provider` |
| `PRODSHADOW_ISOLATION_NETWORK` | `postgres.isolation.network` |
| `PRODSHADOW_ISOLATION_BINARY` | `postgres.isolation.binary` |
| `PRODSHADOW_WORKLOAD_REPEATS` | `workload.repeats` |
| `PRODSHADOW_WORKLOAD_CONCURRENCY` | `workload.concurrency` |
| `PRODSHADOW_WORKLOAD_MEASUREMENT_SECONDS` | `workload.measurement_seconds` |
| `PRODSHADOW_WORKLOAD_WARMUP_SECONDS` | `workload.warmup_seconds` |
| `PRODSHADOW_REPORT_DIRECTORY` | `report.directory` |
| `PRODSHADOW_REPORT_FORMATS` | `report.formats` (comma-separated) |

Applied overrides are recorded in the report (`environment.env_overrides`).
