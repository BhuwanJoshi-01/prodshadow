# ADR 0002: Never connect to a database ProdShadow did not create

**Status:** accepted

## Context

A tool that runs migrations and load against PostgreSQL is one setting away
from running them against production. Users under deadline pressure copy
connection strings; CI secrets end up in the wrong job. The blast radius of
that mistake is a production outage or data loss caused by a safety tool.

## Decision

* The configuration has **no field for a database address, user or
  password**, and the CLI has no flag for one.
* ProdShadow starts its own server for every run: a container on the
  `none` network reached through a unix socket in a private directory, or
  a port published on `127.0.0.1` only, or local binaries in a temporary
  directory.
* Before dialing, the target address is checked against what the provider
  reported; anything else is refused.
* Every connection verifies a random per-run ownership token stored in the
  server (`prodshadow.run_token`), so a different server that happens to
  listen on the same port is refused.
* Cleanup only removes containers that carry the
  `io.prodshadow.managed=true` label.

## Consequences

* Production statistics cannot be read directly. A future statistics import
  will be a separate, read-only, opt-in command with its own credentials
  (see the [roadmap](../roadmap.md)).
* Users must describe production with a fixture and a workload, which is
  more work than pointing at a replica, and less representative.
* The negative tests in the [threat model](../threat-model.md) guard these
  invariants.
