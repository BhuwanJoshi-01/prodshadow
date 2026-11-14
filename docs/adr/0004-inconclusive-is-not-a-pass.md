# ADR 0004: An inconclusive result is never a pass

**Status:** accepted

## Context

An experiment can fail to produce evidence: the workload ran too few
operations, the baseline varied more than the effect being measured, the
parser could not read a statement, or no workload was configured at all. A
tool that reports "no findings" in those cases tells reviewers a migration
is fine when nothing was actually checked, and that is worse than no tool.

## Decision

* Quality rules (`QUAL*`) describe the experiment itself. When one fires,
  the decision is `inconclusive`, with the reasons listed.
* `inconclusive` has its own exit code (5) and, by default
  (`policy.inconclusive: fail`), fails CI.
* Teams that prefer not to block can set `policy.inconclusive: pass`; the
  run then exits 0, but the report and job summary still say inconclusive,
  and `validate` warns about the setting.
* Latency changes inside the measured noise band are not reported as
  regressions, and are not reported as "no change" either.

## Consequences

* New users see inconclusive results until their fixture and workload are
  good enough, which is friction by design; the troubleshooting guide lists
  what to change for each reason.
* A pass means "no threshold was exceeded in an experiment that could have
  detected it", which is the only claim the tool can support.
