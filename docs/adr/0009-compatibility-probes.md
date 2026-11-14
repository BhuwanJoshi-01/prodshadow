# ADR 0009: Check application versions with probes, not with the traffic mix

**Status:** accepted

## Context

During a rolling deploy the previous release keeps running against the new
schema. Modelling the old release as weighted scenarios works (the
rolling-deploy example did) but has two problems: only frequent queries are
exercised, so a rarely used query of the old release goes unchecked, and a
broken old release shows up indirectly, as an error-rate increase whose
cause a reviewer has to work out.

## Decision

* The workload gains a `compatibility` section listing, per release, the
  queries it depends on ("probes").
* In every candidate phase each probe runs once on the fresh database before
  the workload starts and once after the migration has finished, as the
  application role, inside a transaction that is rolled back, with a short
  statement timeout. The same parameter values are used before and after.
* A probe that works before and fails after is reported per release as
  `OBS013`; one that already fails before is `QUAL009`, because that release
  was not checked. Findings carry the release in `app_version`, so diffs and
  suppressions can tell releases apart.

## Consequences

* Write probes are safe because they are rolled back; probes cannot detect
  problems that only appear when their writes commit.
* Probes run outside the measurement window and do not change latency
  numbers.
* Teams have to maintain the list of queries per release. Static rules
  nudge them: `QUAL012` suggests probes when a migration drops or renames
  something.
