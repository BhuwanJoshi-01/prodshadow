# Contributing to ProdShadow

Thank you for helping. The most valuable contributions are the ones that
reduce false confidence: realistic fixtures, reproductions of migrations that
ProdShadow got wrong, and fixes to evidence or wording that overstates what
an experiment shows.

## Ground rules

* **Safety first.** Changes must keep the isolation invariants in
  [docs/threat-model.md](docs/threat-model.md). Never add a way to pass a
  database address to the executor, and never run shell commands built from
  configuration.
* **Evidence over labels.** A finding must say what was observed (or
  inferred), the threshold, the confidence and why. Prefer `inconclusive` to
  a guess.
* **Small, focused pull requests** with tests. SQL behaviour must be covered
  by integration tests against real PostgreSQL; mocks are not enough.

## Development setup

You need Go 1.26 or newer, a C compiler (cgo builds libpg_query), and Docker
for integration tests.

```bash
make build        # bin/prodshadow
make test         # unit tests, no database
make race         # unit tests with the race detector
make integration  # integration tests against PostgreSQL containers
make examples     # run every example and check its documented outcome
make lint         # gofmt, go vet and golangci-lint (if installed)
```

The first build compiles libpg_query and takes a minute; later builds are
cached.

Integration tests use `postgres:17-alpine` by default. Test another version
with:

```bash
PRODSHADOW_IT_IMAGE=postgres:16-alpine PRODSHADOW_IT_MAJOR=16 make integration
```

The `local` provider tests need PostgreSQL binaries and must run as a
non-root user (PostgreSQL refuses to start as root).

## Project layout

| Path | Purpose |
|---|---|
| `cmd/prodshadow` | Entry point |
| `internal/cli` | Commands, flags, exit codes |
| `internal/config` | `prodshadow.yaml` model, defaults, validation |
| `internal/sqlsplit` | Position-aware statement splitter |
| `internal/sqlinspect` | Parsing, lock classification, static rules |
| `internal/pglock` | Lock modes and conflict matrix |
| `internal/isolation` | Disposable servers, guarded connections |
| `internal/fixture` | Schema, seed and generated data |
| `internal/gen` | Deterministic value generators |
| `internal/workload` | Workload definition and concurrent runner |
| `internal/telemetry` | Lock sampling and database counters |
| `internal/migrate` | Migration executor |
| `internal/experiment` | Paired baseline/candidate orchestration |
| `internal/risk` | Comparisons, quality, findings, decision |
| `internal/report` | JSON, Markdown and HTML reports |
| `schemas` | Public JSON Schemas |
| `examples` | End-to-end example projects |

## Adding a static rule

1. Add the rule to the matching `internal/sqlinspect/rules_*.go` file with a
   new `PS1xx` ID, a kebab-case name, a summary and a documentation link.
2. Keep it conservative. If the hazard depends on data the parser cannot
   see, mark the finding `heuristic` with lowered confidence.
3. Add positive and negative cases to the rule's test file.
4. Document it in [docs/rules.md](docs/rules.md).

## Commit messages

Use the imperative mood and prefix the area, for example
`sqlinspect: detect volatile defaults in ADD COLUMN`. Explain *why* in the
body when it is not obvious.

A `Signed-off-by` line (`git commit -s`) is welcome but not required.

## Reporting false findings

Open an issue with the migration, the relevant part of the fixture and
workload, the PostgreSQL version and the JSON report. Anonymise anything
sensitive first. Reproductions that become examples or integration tests are
especially appreciated.

## Security issues

Please report vulnerabilities privately; see [SECURITY.md](SECURITY.md).
