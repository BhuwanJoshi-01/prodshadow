# ADR 0006: In open-loop mode, measure latency from the intended start

**Status:** accepted

## Context

In a closed-loop workload each session waits for its previous operation
before starting the next one. When the database blocks, sessions stop
issuing work, so fewer slow operations are recorded and the latency
distribution looks better than what users experience. This is coordinated
omission, and it hides exactly the lock-queue stalls ProdShadow is meant to
show. Production traffic does not slow down because the database did.

## Decision

* With `workload.arrival_rate`, operations arrive as a Poisson process and
  are dispatched to a fixed pool of sessions.
* Latency is measured from each operation's **intended** start time, not
  from when a session became free, so time spent queued in the client
  during a stall is counted.
* Closed loop remains the default because it needs no rate to be chosen and
  shows the throughput drop directly (`OBS008`); the workload docs explain
  when to use each.

## Consequences

* Open-loop runs report higher, more honest tail latencies during blocking.
* If the arrival rate exceeds what the pool can serve even without a
  migration, the baseline itself queues; the noise band and `QUAL` rules
  make that visible instead of attributing it to the migration.
