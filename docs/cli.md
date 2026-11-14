# Command line reference

```text
prodshadow <command> [flags]
```

Progress and warnings go to standard error; results go to standard output.

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Success. For `verify`: no blocking finding, or inconclusive with `policy.inconclusive: pass`. |
| `1` | Unexpected internal error. |
| `2` | Blocking findings (`verify`; `inspect --fail-on`). |
| `3` | Invalid configuration, input or usage. |
| `4` | Infrastructure failure: container engine unavailable, server start-up or cleanup failed, reports could not be written. |
| `5` | Inconclusive: missing workload or fixture, too few samples, unstable baseline, unparsable statements (by default). |

Whether code 5 fails your build is your choice (`policy.inconclusive`), but
it is never reported as a pass.

## `prodshadow init`

Creates `prodshadow.yaml`, `fixtures/schema.sql`, `fixtures/seed.sql`,
`workloads/workload.yaml` and an example migration.

| Flag | Default | |
|---|---|---|
| `--dir` | `.` | Target directory. |
| `--project` | directory name | Project name. |
| `--force` | `false` | Overwrite existing files. |

## `prodshadow validate`

Validates the configuration and parses every referenced file (workload,
generator, suppressions, migrations) without starting a server.

| Flag | Default | |
|---|---|---|
| `--config` | `prodshadow.yaml` | Configuration file. |
| `--strict` | `false` | Treat warnings (unknown keys, unpinned image, …) as errors. |
| `--json` | `false` | Machine-readable output. |

## `prodshadow inspect`

Static analysis only; needs no database or container engine.

| Flag | Default | |
|---|---|---|
| `--migration` | — | Migration file; repeat for several, executed in order. `--migration -` reads a single migration from standard input, reported as `<stdin>`. |
| `--config` | — | Use rule overrides, transaction mode and `migration.files` from a configuration. |
| `--transaction` | `auto` | `auto`, `per-file` or `none`. |
| `--format` | `text` | `text`, `json`, `markdown` or `sarif`. |
| `--fail-on` | — | Exit 2 if an unsuppressed finding is at least this severe (`info`, `warning`, `critical`; inconclusive findings count as more severe than critical). |

## `prodshadow plan`

Shows what `verify` would run — server, isolation limits, fixture, workload
sessions, the alternating phase schedule, transaction groups with predicted
locks, and an estimated duration — without starting anything. Capacity
problems (too many sessions for `max_connections`) are listed as warnings.

| Flag | Default | |
|---|---|---|
| `--config` | `prodshadow.yaml` | Configuration file. |
| `--migration` | `migration.files` | Override the migration files (repeatable). |
| `--json` | `false` | Machine-readable output. |

## `prodshadow verify`

Runs the experiment and writes reports.

| Flag | Default | |
|---|---|---|
| `--config` | `prodshadow.yaml` | Configuration file. |
| `--migration` | `migration.files` | Override the migration files (repeatable). |
| `--output` | `report.directory` | Report directory. |
| `--format` | `report.formats` | Comma-separated: `json`, `markdown`, `html`, `sarif`, `junit`. |
| `--repeats` | `workload.repeats` | Number of baseline/candidate pairs. |
| `--keep` | `false` | Keep the server after the run for local debugging. Never use in CI. |
| `--quiet` | `false` | No progress output. |

In GitHub Actions the Markdown report is appended to `$GITHUB_STEP_SUMMARY`.
Interrupting the command (Ctrl-C, SIGTERM) stops the run and removes the
server.

## `prodshadow report`

Re-renders a JSON report written by `verify`.

| Flag | Default | |
|---|---|---|
| `--input` | `prodshadow-results/report.json` | Report to read. Reports with a newer major schema version are rejected. |
| `--format` | `markdown` | `markdown`, `html`, `json`, `sarif` or `junit`. |
| `--output` | stdout | Output file. |

## `prodshadow diff`

Compares two JSON reports — typically a migration and its rewritten
alternative — and prints the decision change, findings resolved, introduced
and persisting, and each scenario's candidate p95, stall and error rate.
Runs with different fixtures, workloads or PostgreSQL versions are flagged as
not directly comparable.

```bash
prodshadow verify --output out/original
prodshadow verify --migration alternatives/0012_concurrently.sql --output out/alternative
prodshadow diff --base out/original/report.json --head out/alternative/report.json
```

| Flag | Default | |
|---|---|---|
| `--base` | — | Report of the original migration. |
| `--head` | — | Report of the alternative. |
| `--json` | `false` | Machine-readable output. |

## `prodshadow history`

Summarises stored JSON reports as a timeline, oldest first: decision,
longest candidate migration, longest lock wait, most sessions blocked by the
migration, and unsuppressed findings by severity. Name report files or
directories; a directory contributes every `*.json` file directly inside it,
and files there that are not reports are skipped with a note. A run whose
fixture, workload or PostgreSQL version changed since the previous one is
marked with `*`.

```bash
# keep each CI run's report, e.g. as reports/<run-id>.json, then:
prodshadow history reports/ --project shop --last 20
```

| Flag | Default | |
|---|---|---|
| `--project` | — | Only runs of this project. |
| `--last` | `0` | Only the N most recent runs (0 shows all). |
| `--json` | `false` | Machine-readable output. |

## `prodshadow doctor`

Checks the platform, the configuration (if present), the container engine
and leftovers from interrupted runs. Exits 4 if a check fails.

## `prodshadow cleanup`

Removes containers labelled `io.prodshadow.managed=true`. It refuses to
remove anything else.

| Flag | Default | |
|---|---|---|
| `--provider` | `docker` | `docker` or `podman`. |
| `--run` | — | Only this run ID. |
| `--dry-run` | `false` | List without removing. |

## `prodshadow rules`, `locks`, `schema`, `completion`, `version`

* `prodshadow rules [--kind static|observed|quality] [--json] [RULE ...]`
  lists every rule with its default severity. Name one or more rules, by ID
  or name, to see their details and a link to their documentation page,
  for example `prodshadow rules PS103 OBS002`.
* `prodshadow locks [--json] [MODE]` explains PostgreSQL's table lock modes:
  which statements take each one, which modes it conflicts with and which
  statements wait while it is held or queued. Name a mode in any spelling
  (`access exclusive`, `ACCESS_EXCLUSIVE`, `AccessExclusiveLock`) for its
  details.
* `prodshadow schema config|workload|report` prints a JSON Schema.
* `prodshadow completion bash|zsh|fish` prints a shell completion script for
  commands, flags and schema names, e.g. `source <(prodshadow completion bash)`
  in `~/.bashrc`, or
  `prodshadow completion fish > ~/.config/fish/completions/prodshadow.fish`.
* `prodshadow version [--json | --short]` prints the version, commit, report
  schema version and parser grammar. `--short` prints only the version
  number, for scripts that pin or compare it.
