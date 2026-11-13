# ProdShadow

**Rehearse PostgreSQL migrations under concurrent load — in a disposable database, never in production.**

ProdShadow runs your migration in an isolated, throwaway PostgreSQL server
while a workload you define keeps querying it, compares the result with an
identical run *without* the migration, and reports what it measured: which
sessions waited, for how long, behind which lock, and what that did to
latency, errors and throughput. The result is a JSON/Markdown/HTML report
and a CI exit code.

> ProdShadow produces evidence and uncertainty, never a guarantee. A passing
> result means *no configured threshold was exceeded in this experiment*,
> not that the migration is safe in production.

## Why

Most migration outages are not caused by slow SQL. A statement that takes a
millisecond — `ALTER TABLE ... ADD COLUMN`, say — needs an `ACCESS EXCLUSIVE`
lock. If a long-running transaction holds any lock on the table, the
`ALTER TABLE` waits, and **every query that arrives after it waits behind
it**. A linter can tell you the lock mode; it cannot show you the queue.

ProdShadow reproduces the queue deliberately: workloads can declare
*background sessions* (a report that keeps a transaction open, for
example), and the experiment measures what happens to everyone else.

```text
  sec      ops  errors      p95       max        <- candidate run, 8 sessions
     3    15538       0   1.2 ms    6.6 ms
     4    16050       0   1.2 ms    5.8 ms
M    5       63       0  1504 ms   1504 ms        <- ALTER TABLE waiting for its lock
M    6     7232       0   1.2 ms    9.4 ms
     7    14844       0   1.3 ms    7.1 ms
```

See [docs/research/landscape.md](docs/research/landscape.md) for public
incidents and how ProdShadow relates to linters such as squawk, Atlas and
strong_migrations, and to clone-based checkers such as DBLab.

## How it works

1. **Static analysis** – every migration statement is parsed with the
   PostgreSQL grammar; ProdShadow records the locks it takes, where in the
   file it is, and a small set of hazards (`prodshadow rules`).
2. **Disposable server** – a PostgreSQL container with no network, a
   read-only root file system, a size-capped tmpfs data directory, all
   capabilities dropped except those the image needs, and CPU, memory and
   process limits. Every connection verifies a per-run ownership token.
3. **Fixture** – schema and seed SQL, and optionally deterministic
   generated rows, are loaded once into a template database by an
   unprivileged role.
4. **Paired experiment** – for each repeat a *baseline* (workload only) and
   a *candidate* (workload while the migration runs in its own session with
   `lock_timeout` and `statement_timeout`) run on fresh copies of the
   template, in alternating order.
5. **Evidence** – every operation is timed on the client; `pg_stat_activity`
   and `pg_locks` are sampled to attribute waits (*who blocked whom*);
   counters record WAL and deadlocks.
6. **Decision** – transparent rules compare candidate with baseline against
   your policy. Unstable baselines, too few samples or missing inputs give
   `inconclusive` — never `pass`.

## Quick start

Requirements: Go 1.26+, a C compiler (the SQL parser uses cgo), and Docker or
Podman on Linux. On macOS use `network: loopback`. Without a container
engine, the `local` provider runs installed PostgreSQL binaries (without
resource limits).

```bash
go install github.com/BhuwanJoshi-01/Open-Source-1/cmd/prodshadow@latest

prodshadow init            # starter config, fixture, workload and migration
prodshadow doctor          # check Docker and the configuration
prodshadow inspect --config prodshadow.yaml   # static analysis, no database
prodshadow verify  --config prodshadow.yaml   # the full experiment
```

`prodshadow plan` shows what `verify` will do and roughly how long it takes.
`verify` writes `prodshadow-results/report.{json,md,html}` (plus SARIF and
JUnit XML on request) and exits with:

| Code | Meaning |
|---|---|
| 0 | Completed; no blocking finding (or inconclusive with `policy.inconclusive: pass`) |
| 2 | Completed with blocking findings |
| 3 | Invalid configuration or input |
| 4 | Infrastructure failure (container engine, cleanup) |
| 5 | Inconclusive: the experiment could not decide |

## Examples

| Example | Demonstrates | Expected |
|---|---|---|
| [safe-add-column](examples/safe-add-column) | Metadata-only change, short transactions | pass |
| [long-running-transaction](examples/long-running-transaction) | Lock queue behind a reporting transaction | block (OBS002) |
| [blocking-index](examples/blocking-index) | `CREATE INDEX` without `CONCURRENTLY` stalls writers | block (OBS001) |
| [backfill-heavy](examples/backfill-heavy) | Unbatched `UPDATE` holds row locks | block (OBS001) |
| [rolling-deploy](examples/rolling-deploy) | Dropping a column the old app version still reads | block (OBS007) |

## In CI

```yaml
- uses: actions/checkout@v4
- uses: BhuwanJoshi-01/Open-Source-1@main
  with:
    config: prodshadow.yaml
```

The action writes the Markdown report to the job summary, annotates the
migration lines of the pull request, and uploads the report directory as an
artifact. SARIF output feeds GitHub code scanning; JUnit XML feeds GitLab and
Jenkins test reports. See [docs/ci.md](docs/ci.md).

## Safety model

* There is **no configuration field or flag that accepts a database
  address**. Servers are created by the isolation manager, which hands out
  connections only to unix sockets or loopback addresses and verifies an
  ownership token on every connection.
* Fixture, workload and (by default) migration SQL run as an unprivileged
  role, so server-side file and program access is unavailable.
* Extensions and server settings that reach the network or execute shell
  commands are refused.
* Artifacts never contain passwords, connection strings or raw SQL text
  unless you opt in; known secret formats are redacted.

Details: [docs/threat-model.md](docs/threat-model.md).

## Documentation

* [Configuration reference](docs/configuration.md)
* [Workload reference](docs/workload.md)
* [Rules](docs/rules.md) · [Report format](docs/report.md) · [CLI](docs/cli.md)
* [Architecture](docs/architecture.md) · [Threat model](docs/threat-model.md)
* [Limitations](docs/limitations.md) · [Measured results](docs/benchmarks.md) · [FAQ](docs/faq.md) · [Roadmap](docs/roadmap.md)
* [CI guide](docs/ci.md) · [Landscape and gap analysis](docs/research/landscape.md)
* [Contributing](CONTRIBUTING.md) · [Security policy](SECURITY.md)

## Status

Pre-release (0.1 development). Interfaces may change before 1.0; the report
schema is versioned independently (`schema_version`).

* The integration suite runs against real PostgreSQL 15, 16, 17 and 18
  containers in CI.
* Every example reproduces its documented decision; measured results are in
  [docs/benchmarks.md](docs/benchmarks.md).
* Not yet validated with external teams — see the validation plan in
  [docs/roadmap.md](docs/roadmap.md). Reports of false findings are very
  welcome.

## License

Apache License 2.0 — see [LICENSE](LICENSE).
