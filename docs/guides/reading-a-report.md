# Reading a report

`prodshadow verify` writes the same facts in several formats: `report.json`
(the contract, see [report format](../report.md)), `report.md` (also the
GitHub job summary) and `report.html`. This guide walks through the
Markdown report from top to bottom and says what to look at first.

## 1. The decision

```text
**Decision: BLOCK** · exit code 2 · status `completed_with_findings`
Blocking findings: OBS002, OBS005.
```

* `pass` (exit 0): no finding reached a severity in `policy.fail_on`.
  It does not mean the migration is safe in production; the disclaimer under
  the decision says so on every report.
* `block` (exit 2): the listed rules failed. Start with these.
* `inconclusive` (exit 5): the experiment could not decide, for example too
  few samples or a noisy baseline. The quality section says why; see
  [troubleshooting](../troubleshooting.md#the-run-is-inconclusive).
* `error` (exit 4): the infrastructure failed; the errors section explains.

The table under it identifies the run: PostgreSQL version, isolation,
fixture size, workload shape and duration. Compare runs only when the
fixture and workload match (`prodshadow history` marks runs where they
changed).

## 2. Findings

One row per finding, most severe first, with the rule (linked to its page),
confidence and the reason for it, the evidence kind, and where it applies:
a migration file and line, a workload scenario, or an application version.

* **Observed** findings (`OBS…`) are measurements from the experiment.
  `OBS001` (lock wait budget) and `OBS002` (lock timeout) are the classic
  lock-queue outage; `OBS005`/`OBS006` are latency regressions; `OBS013`
  names an application release that breaks.
* **Static** findings (`PS…`) come from reading the SQL and explain *why*
  something can block or break; heuristic ones say what they cannot see.
* **Quality** findings (`QUAL…`) are about the experiment itself: what it
  did not model or measure.

The remediation list below the table repeats the fixes for warnings and
critical findings.

## 3. Scenario comparison

Per scenario, the median across repeats of baseline and candidate: p95 and
its ratio, the longest operation while the migration ran, error-rate and
throughput change, and the baseline noise. A ratio inside the noise band is
not attributed to the migration. *Max during migration* is often the most
telling number: it is the stall a user would have seen.

## 4. Migration

Every statement with its predicted locks, what happened to it in each
candidate repeat (`succeeded ×3`, `failed 55P03 ×3`) and its longest
duration. Lock retries, if configured, are listed under the table.

## 5. Lock telemetry

Per candidate repeat: how long the migration waited and for whom
(`Migration blocked by` names the session by `application_name`), how many
workload sessions queued behind it, and the longest open transaction. This
is the *who blocked whom* evidence behind `OBS001`, `OBS002` and `OBS011`.

## 6. Application compatibility

If the workload declares probes, one row per probe with its result before
and after the migration and in how many repeats it broke.

## 7. Timeline

Operations per second, errors and latency for each second of the worst
candidate repeat; `M` marks the seconds the migration ran. A drop in
operations and a spike in `max` under `M` is the lock queue in action.

## 8. Experiment quality and reproduction

What the experiment can and cannot claim: samples per scenario, baseline
noise, sampling gaps, the fixture's representativeness and the standing
limitations, followed by everything needed to reproduce the run (tool
version, image digest, settings, seeds).
