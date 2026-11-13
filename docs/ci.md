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
      - uses: BhuwanJoshi-01/Open-Source-1@main   # pin a release tag or commit SHA
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
      - id: changed
        run: |
          {
            echo 'files<<EOF'
            git diff --name-only --diff-filter=A "origin/${GITHUB_BASE_REF}...HEAD" -- 'migrations/*.sql'
            echo EOF
          } >> "$GITHUB_OUTPUT"
      - if: steps.changed.outputs.files != ''
        uses: BhuwanJoshi-01/Open-Source-1@main
        with:
          migrations: ${{ steps.changed.outputs.files }}
```

Older migrations should already be part of `fixture.schema`, so that the new
migration runs against the schema it will meet in production.

### Inline annotations

When `GITHUB_ACTIONS=true`, `verify` and `inspect` also print workflow
commands, so critical findings appear as errors and warnings or inconclusive
findings as warnings directly on the migration lines of the pull request.
No extra permissions are needed. Informational and suppressed findings are
not annotated.

### Code scanning annotations

Add `sarif` to `report.formats` and upload the file:

```yaml
      - uses: BhuwanJoshi-01/Open-Source-1@main
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
CGO_ENABLED=1 go install github.com/BhuwanJoshi-01/Open-Source-1/cmd/prodshadow@main
prodshadow verify --config prodshadow.yaml
status=$?
# publish prodshadow-results/ as an artifact, then:
exit $status
```

GitLab CI with Docker-in-Docker needs `network: loopback` unless the job can
bind-mount a host directory into the service container.

## Runtime budget

A run takes roughly

```text
start-up + seeding + repeats × 2 × (warmup + measurement + ~3 s)
```

The examples (3 repeats, 3 s warm-up, 15 s measurement) take about two
minutes on a 4-CPU runner. Measure your own fixtures rather than relying on
these numbers.
