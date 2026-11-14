# Report format

`verify` writes up to five files to the report directory:

* `report.json` – the machine-readable contract, validated against
  [`schemas/report.v1.schema.json`](../schemas/report.v1.schema.json);
* `report.md` – for pull request comments and CI job summaries;
* `report.html` – a single self-contained page (no external resources) with
  a timeline chart, suitable as a CI artifact;
* `report.sarif` – SARIF 2.1.0, so findings appear as code scanning alerts
  and pull request annotations on the migration lines. Findings without a
  line (for example a scenario's latency) are anchored to the first line of
  the first migration file. Paths are relative to the configuration
  directory; run from the repository root for correct annotations;
* `report.junit.xml` – JUnit XML for CI systems that display test reports
  (GitLab, Jenkins, Buildkite). Blocking findings fail, suppressed findings
  are skipped, and a first test case carries the overall decision.

`prodshadow report` re-renders a JSON report in any format.

## Versioning

`schema_version` follows semantic versioning independently of the CLI:

* additive, optional fields bump the **minor** version;
* removing or changing the meaning of a field bumps the **major** version.

| Version | Change |
|---|---|
| 1.4.0 | `phases[].scenarios[].sample_error`: the first (redacted) error message of a scenario that failed. |
| 1.3.0 | `phases[].migration.retries`: attempts that hit `lock_timeout` and were retried. |
| 1.2.0 | `findings[].app_version`: the application version of `OBS013` and `QUAL009` findings. |
| 1.1.0 | `phases[].compatibility`: compatibility probe outcomes. |
| 1.0.0 | First release. |

Consumers should ignore unknown fields. `prodshadow report` refuses reports
with a different major version instead of misreading them.

## Top-level fields

| Field | Description |
|---|---|
| `status` | `completed`, `completed_with_findings`, `inconclusive` or `failed`. |
| `ci_decision` | `pass`, `block`, `inconclusive` or `error` (same as `decision.outcome`). |
| `decision` | Outcome, exit code, message, failed / inconclusive / suppressed rule IDs, expired suppressions. |
| `findings` | Every finding, most severe first; suppressed findings last. |
| `comparisons` | Per scenario: baseline and candidate medians across repeats (`p50/p95/p99`, throughput, error rate, max observed), the during-migration window, ratios, error-rate delta in percentage points, throughput change, baseline noise (coefficient of variation) and stall. |
| `phases` | Every baseline and candidate phase: scenario statistics, per-second timeline, lock telemetry summary, database counters (WAL bytes, deadlocks, …), the migration outcome per statement and, in candidate phases, each compatibility probe's result before and after the migration (`compatibility`, since 1.1.0). |
| `migration` | Files, timeouts, transaction mode and every statement with location, kind, predicted locks, transaction requirement and fingerprint. |
| `quality` | Pairs completed, minimum samples, baseline noise, sampling interval and gaps, connection failures, limitations. |
| `representativeness` | Fixture level (`manual-fixture`, `synthetic`) and limitations. |
| `engine` | PostgreSQL version, runtime, image, network mode, resource limits, settings and their hash, parser grammar. |
| `fixture`, `workload` | Hashes, table sizes, scenario descriptions (template IDs instead of SQL by default), seeds. |
| `tool`, `git_sha`, `environment` | Reproduction metadata. |
| `effective_config` | The configuration after defaults and environment overrides (redaction patterns omitted). |
| `errors` | Infrastructure errors; any error makes the run `failed` (exit 4). |

## Finding

```json
{
  "rule_id": "OBS002",
  "rule": "migration-lock-timeout",
  "severity": "critical",
  "confidence": "high",
  "confidence_reason": "observed in 3 of 3 candidate repeats",
  "evidence_kind": "observed",
  "message": "The migration could not acquire its lock within lock_timeout (2000 ms) because prodshadow/background/revenue_report/0 held a conflicting lock. ...",
  "statement_id": "1.1",
  "file": "migrations/0007_add_invoice_reference.sql",
  "line": 4,
  "table": "invoices",
  "lock": "ACCESS EXCLUSIVE",
  "remediation": "Run the statement with a short lock_timeout and retry with backoff, ...",
  "docs_url": "https://www.postgresql.org/docs/current/runtime-config-client.html#GUC-LOCK-TIMEOUT"
}
```

`observed`, `threshold` and `unit` are present when a measurement was
compared with a policy value.

## Privacy

By default reports contain no SQL text, no parameter values, no passwords
and no connection strings. Statements and workload scenarios are identified
by fingerprints and template IDs. Server error messages are redacted using
the run's ephemeral secrets, built-in credential patterns and
`privacy.redact_patterns`. Set `privacy.persist_raw_query_text: true` to
include SQL (still redacted).

The built-in patterns cover passwords in connection URLs and `key=value`
strings, SQL `PASSWORD '...'` clauses (`CREATE ROLE`, `ALTER ROLE`, user
mapping options), `PGPASSWORD`, bearer tokens, AWS access key IDs, GitHub,
Slack, Stripe and Google API keys, JSON Web Tokens and PEM private keys.
Add `privacy.redact_patterns` for anything specific to your systems.
