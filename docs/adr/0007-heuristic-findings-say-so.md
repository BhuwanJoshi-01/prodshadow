# ADR 0007: Static rules mark what they cannot see as heuristic

**Status:** accepted

## Context

A migration file shows statements, not the database they run against.
Whether `ALTER COLUMN TYPE` rewrites the table depends on the old type;
whether a foreign key already has an index, whether a table is in a
publication, whether a server preloads an extension, all live outside the
file. A rule that reports these as certainties trains reviewers to ignore
it; a rule that stays silent misses real hazards.

## Decision

* Each finding has an evidence kind. `static` means the hazard follows from
  the SQL and PostgreSQL's documented behaviour alone. `heuristic` means it
  depends on something the migration does not show.
* Heuristic findings carry a lowered confidence and a message that names
  what could make them wrong (for example "unless an index with these
  leading columns already exists").
* Rules use what the migration *does* show to avoid false positives:
  relations created earlier in the same migration carry no traffic; a
  constraint validated in an earlier transaction lets `SET NOT NULL` skip
  its scan; a `USING` clause makes a rewrite certain.
* The experiment, not the static rule, is the arbiter: findings say how the
  candidate run verifies them, and observed rules report what actually
  happened.

## Consequences

* Some heuristic findings are noise for a given database. Policies can
  lower or disable individual rules, and suppressions document reviewed
  exceptions with a reason and expiry.
* Rule pages state the evidence kind and what the rule cannot see, so a
  reviewer can judge a finding without reading the code.
