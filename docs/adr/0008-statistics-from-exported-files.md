# ADR 0008: Import production statistics from exported files, not connections

**Status:** accepted

## Context

A workload and fixture that resemble production make the experiment far
more useful, and `pg_stat_statements` and `pg_class` contain most of what is
needed: which statements run, how often, and how big the tables are. The
roadmap listed a read-only statistics import, gated on a security review,
because it would be the first feature to connect to a database ProdShadow
did not create, contradicting [ADR 0002](0002-no-production-connections.md).

## Decision

* ProdShadow does not connect to production for statistics either. Users
  export what is needed themselves, with a query they can read, on a
  replica if they prefer, as CSV (`\copy (...) TO 'file' CSV HEADER`).
* `prodshadow draft-workload` turns a `pg_stat_statements` export into a
  draft workload, and `prodshadow draft-fixture` turns the fixture schema
  plus a `pg_class` export into a draft generator file.
* The output is a draft with `TODO` markers, meant to be reviewed and
  committed, not an input used directly by `verify`: query text can contain
  literals, and parameter ranges need human judgement.

## Consequences

* No credentials, network access or new attack surface; ADR 0002 stands
  without exceptions.
* Users run one or two queries by hand and keep the drafts current
  themselves. The [modelling guide](../guides/modelling-traffic.md) lists
  the queries.
* Exported query text is user data: the drafter treats it as untrusted
  input (fuzzed in CI) and the guide asks users to review it before
  committing.
