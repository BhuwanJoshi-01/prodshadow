# ADR 0010: Rehearse the deploy tool's lock retries

**Status:** accepted

## Context

The standard advice for blocking DDL is a short `lock_timeout` plus
retries: an attempt that cannot get its lock gives up before the queue
behind it grows, and the deploy tries again. Rehearsing a single attempt
reports `OBS002` (the lock was not granted) and blocks, even when the
deploy as it really runs would have succeeded on the second attempt with a
one-second stall.

## Decision

* `migration.lock_retries` and `migration.retry_backoff_ms` make the
  executor behave like such a deploy tool: a transaction group (or an
  autocommitted statement) that fails with `lock_not_available` is rolled
  back and run again after a backoff that grows with the attempt number.
* Every retried attempt is recorded with the statement, attempt number and
  how long it waited (`phases[].migration.retries`); only the last
  attempt's statement outcomes are kept.
* A migration that succeeds after retries is reported as `OBS014` (info);
  one that runs out of retries is still `OBS002`, with the retry count.
* The candidate measurement window extends by the longest time the retries
  can take, so the whole sequence is observed.
* Retries are off by default, so a configuration describes the deploy as
  it is.

## Consequences

* Each failed attempt still stalls sessions that arrive while it waits;
  `OBS001` and `OBS011` measure that, so retries do not hide the cost.
* The rehearsal matches production only if the configured retries match
  the deploy tool's.
