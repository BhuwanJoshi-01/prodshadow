# ADR 0001: Measure the migration as a paired, alternating experiment

**Status:** accepted

## Context

The question ProdShadow answers is "what did this migration do to the
traffic around it?". A single run with the migration cannot answer it:
latency on a small CI runner varies by tens of percent between identical
runs, warms up as caches fill, and drifts as the runner's neighbours change.
Any absolute threshold would either miss real regressions or flag noise.

## Decision

Each repeat runs a **baseline phase** (workload only) and a **candidate
phase** (workload plus migration) on the same server, each in a fresh copy
of the same template database. Findings compare the candidate with the
baseline, using medians across repeats. The phase order alternates between
repeats (baseline first, then candidate first) so that drift and warm-up
affect both sides equally. The variation between baseline repeats is
measured and used as the noise band: changes inside it are not attributed
to the migration.

## Consequences

* A run takes `repeats × 2` measurement windows; the default of three
  repeats roughly doubles the cost compared with a single candidate run.
* Results are ratios and differences, so policies are written as
  "p95 may grow by 1.5×" rather than "p95 under 20 ms", and carry over
  between runners of different speed.
* A noisy baseline produces an inconclusive result instead of a guess
  (see [ADR 0004](0004-inconclusive-is-not-a-pass.md)).
* Absolute durations, such as how long the migration took, are still
  reported, but are labelled as fixture-dependent.
