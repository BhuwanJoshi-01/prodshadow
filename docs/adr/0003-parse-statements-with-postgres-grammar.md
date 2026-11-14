# ADR 0003: Split statements ourselves, parse each with PostgreSQL's grammar

**Status:** accepted

## Context

Static rules and the migration executor both need statements with exact
file and line positions. Regular-expression linters misread SQL with dollar
quoting, nested comments or `BEGIN ATOMIC` bodies, and a single statement
the parser rejects should not hide the analysis of every other statement in
the file.

## Decision

* `sqlsplit` splits files into statements, tracking quotes, dollar quotes,
  nested block comments and `BEGIN ATOMIC` bodies, and records byte offsets
  and lines. It is fuzzed in CI.
* Each statement is parsed separately with libpg_query, PostgreSQL's own
  parser, through pg_query_go. The bundled grammar is PostgreSQL 17.
* A statement that does not parse is reported (`PS116`, inconclusive by
  default) and still executed in the experiment, where the real server is
  the judge.
* Lock modes are derived from the parse tree and the documented lock
  levels, not from keywords.

## Consequences

* Building from source needs a C compiler (cgo); release binaries hide this
  from users.
* Syntax newer than the bundled grammar is reported as not analysed until
  the binding is updated. Users who rely on it can set `PS116: info`.
* The executor runs exactly the statements the analysis saw, so a finding's
  location matches what ran.
