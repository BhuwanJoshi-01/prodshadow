# ADR 0005: The JSON report is a versioned contract

**Status:** accepted

## Context

Reports are consumed by CI scripts, dashboards, `prodshadow diff` and
`prodshadow history`, often months after they were written and by a
different version of the tool. Markdown and HTML are for people; machines
need something stable.

## Decision

* The JSON report is the primary output and integration boundary. Its JSON
  Schema lives in `schemas/report.v1.schema.json` and is printed by
  `prodshadow schema report`.
* `schema_version` follows semantic versioning, independent of the CLI
  version: additive fields bump the minor version; removing a field or
  changing its meaning bumps the major version.
* Readers accept any report with the same major version and reject other
  majors instead of misreading them.
* Markdown, HTML, SARIF and JUnit are rendered from the JSON report
  (`prodshadow report`), so every format shows the same facts.
* Raw SQL text and query parameters are left out unless
  `privacy.persist_raw_query_text` / `persist_query_parameters` allow them;
  statements are identified by fingerprints.

## Consequences

* Field names are hard to change once released; new information is added
  beside old fields rather than replacing them.
* Golden-file and schema-validation tests guard the format.
