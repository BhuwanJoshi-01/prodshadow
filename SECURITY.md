# Security policy

ProdShadow executes user-supplied SQL. Its central promise is that this SQL
only ever runs inside a disposable database that ProdShadow created, and that
no artifact leaks credentials. Reports that break either promise are treated
as security issues.

## Reporting a vulnerability

Please **do not open a public issue**. Use GitHub's private vulnerability
reporting ("Report a vulnerability" on the repository's Security tab). Include:

* the ProdShadow version (`prodshadow version --json`),
* the configuration and SQL needed to reproduce (with secrets removed),
* what you expected and what happened.

You will receive an acknowledgement within five working days. Fixes are
released as patch versions and credited in the changelog unless you prefer
otherwise.

## In scope

* Any path by which migration, fixture or workload SQL reaches a database not
  created by the isolation manager.
* Container escapes or host access made possible by ProdShadow's container
  configuration.
* Secrets, connection strings or raw SQL appearing in default artifacts.
* Command injection through configuration values.
* Cleanup failures that leave servers running without being reported.

## Out of scope

* Vulnerabilities in PostgreSQL, Docker, Podman or the container image
  themselves (report them upstream).
* Running ProdShadow with `--keep`, `migration.run_as: superuser`,
  `privacy.persist_raw_query_text: true` or the `local` provider, which
  deliberately relax a guardrail; these are documented trade-offs.
* Denial of service by a configuration that requests very large fixtures
  within the configured limits.

## Supported versions

Until 1.0, only the latest release receives security fixes.
