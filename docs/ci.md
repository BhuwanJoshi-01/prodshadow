# Using ProdShadow in CI

## GitHub Actions

```yaml
name: Migration rehearsal
on:
  pull_request:
    paths: ["migrations/**", "prodshadow.yaml", "fixtures/**", "workloads/**"]

permissions:
  contents: read

jobs:
  prodshadow:
    runs-on: ubuntu-24.04        # Docker is preinstalled; network: none needs Linux
    timeout-minutes: 30
    steps:
      - uses: actions/checkout@v7
      - uses: BhuwanJoshi-01/prodshadow@main   # pin a release tag or commit SHA
        with:
          config: prodshadow.yaml
```

The action builds ProdShadow, runs `prodshadow verify`, appends the Markdown
report to the job summary, uploads `report.json`, `report.md` and
`report.html` as the `prodshadow-report` artifact, and fails the step with
ProdShadow's exit code.

| Input | Default | |
|---|---|---|
| `config` | `prodshadow.yaml` | Configuration, relative to `working-directory`. |
| `migrations` | — | Newline-separated files overriding `migration.files`. |
| `changed-since` | — | Only migrations added or changed since this git ref, e.g. `origin/${{ github.base_ref }}`; needs `fetch-depth: 0`. |
| `args` | — | Extra `prodshadow verify` flags, one flag or value per line, e.g. `--repeats` and `5`. |
| `working-directory` | `.` | |
| `report-directory` | `prodshadow-results` | |
| `version` | the action's ref | ProdShadow version to build. |
| `go-version` | `1.27` | |
| `upload-artifact` | `true` | |
| `fail` | `true` | Set `false` to report without failing the job. |

Outputs: `exit-code`, `decision` (`pass`, `block`, `inconclusive`,
`error`) and `report-directory`.

### Testing only the migrations a pull request adds

```yaml
      - uses: actions/checkout@v7
        with:
          fetch-depth: 0
      - uses: BhuwanJoshi-01/prodshadow@main
        with:
          changed-since: origin/${{ github.base_ref }}
```

`changed-since` keeps the files of `migration.files` that git reports as
added, modified or renamed since the merge base with the pull request's
base branch; when none changed, the step passes without starting a server.
Outside the action, use `prodshadow verify --changed-since origin/main` (or
the same flag on `inspect` and `plan`).

Older migrations should already be part of `fixture.schema`, so that the new
migration runs against the schema it will meet in production.

### Showing the prediction before the experiment

`prodshadow plan` needs no container engine and finishes in a second. Its
Markdown output lists what each statement locks and which workload
sessions it is predicted to conflict with; appending it to the job summary
puts that in front of the reviewer even when the experiment is skipped or
still running:

```yaml
- name: Predicted locks and conflicts
  run: prodshadow plan --config prodshadow.yaml --markdown >> "$GITHUB_STEP_SUMMARY"
```

The ProdShadow action does this by default before running `verify`
(input `plan-summary`, set it to `"false"` to skip).

`verify` then measures the same conflicts; its report shows each
scenario's prediction next to its measured stall.

### Inline annotations

When `GITHUB_ACTIONS=true`, `verify` and `inspect` also print workflow
commands, so critical findings appear as errors and warnings or inconclusive
findings as warnings directly on the migration lines of the pull request.
No extra permissions are needed. Informational and suppressed findings are
not annotated.

### Code scanning annotations

Add `sarif` to `report.formats` and upload the file:

```yaml
      - uses: BhuwanJoshi-01/prodshadow@main
        id: prodshadow
        with:
          fail: "false"
      - uses: github/codeql-action/upload-sarif@v4
        with:
          sarif_file: ${{ steps.prodshadow.outputs.report-directory }}/report.sarif
      - run: exit ${{ steps.prodshadow.outputs.exit-code }}
```

The upload step needs `permissions: security-events: write`.

### Inconclusive results

Exit code 5 means the experiment could not decide (missing inputs, too few
samples, noisy baseline). It fails the job by default. To report without
failing, set `policy.inconclusive: pass` — the report still says
"inconclusive".

## Security in CI

* Run ProdShadow on `pull_request`, **not** `pull_request_target`: the job
  executes SQL from the pull request and must not see deployment secrets.
* ProdShadow never needs database credentials, cloud credentials or a
  token. Give the job `permissions: contents: read`.
* Pin the container image by digest (`postgres:17-alpine@sha256:…`) and the
  action by commit SHA for reproducible, tamper-evident runs.

## Other CI systems

Any Linux runner with Docker or Podman works:

```bash
CGO_ENABLED=1 go install github.com/BhuwanJoshi-01/prodshadow/cmd/prodshadow@main
prodshadow verify --config prodshadow.yaml
status=$?
# publish prodshadow-results/ as an artifact, then:
exit $status
```

GitLab CI with Docker-in-Docker needs `network: loopback` unless the job can
bind-mount a host directory into the service container. Add `junit` to
`report.formats` and declare the file as a test report:

```yaml
prodshadow:
  script:
    - prodshadow verify --config prodshadow.yaml --format json,markdown,junit
  artifacts:
    when: always
    paths: [prodshadow-results/]
    reports:
      junit: prodshadow-results/report.junit.xml
```

## Runtime budget

A run takes roughly

```text
start-up + seeding + repeats × 2 × (warmup + measurement + ~3 s)
```

The examples (3 repeats, 3 s warm-up, 15 s measurement) take about two
minutes on a 4-CPU runner. Measure your own fixtures rather than relying on
these numbers.

## This repository's own CI

The project's workflows are tuned for a private repository, where Actions
minutes are billed and macOS minutes cost ten times the Linux rate:

| Workflow | Pull requests | Scheduled |
|---|---|---|
| CI (lint, tidy, unit tests with `-race`, build) | every change except documentation | nightly, plus macOS and fuzzing |
| Integration | PostgreSQL 17 and the local provider, when database-facing code changes | nightly, PostgreSQL 15–18 |
| Examples | — | weekly |
| CodeQL | — | weekly; results kept as an artifact on private repositories |

Pull requests are merged by fast-forward, so the commit tested on the pull
request is the commit that lands on `main`. Scheduled workflows run on the
repository's **default branch** only. Set Docker Hub credentials as the
`DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN` secrets to avoid anonymous pull
limits. All workflows can be started manually from the Actions tab.
