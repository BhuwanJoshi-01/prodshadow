# FAQ

**Does ProdShadow ever connect to production?**
No. There is no setting for a database address. ProdShadow creates every
database it uses, connects only through a private unix socket or the
loopback interface, and verifies a random per-run token on every connection.
See the [threat model](threat-model.md).

**Why not just use a migration linter?**
Linters tell you which lock a statement takes. They cannot show you the lock
*queue*: an instant `ALTER TABLE` behind a long report transaction stalls
every query on the table. ProdShadow reproduces that situation with
background sessions and measures it. Keep your linter; ProdShadow's static
rules are deliberately few and exist to explain what was measured.

**My migration passed. Is it safe?**
It means no configured threshold was exceeded *in this experiment* — with
your fixture, your workload and a small server. Durations on production-size
tables can be far longer, and traffic you did not model is not represented.
See [limitations](limitations.md).

**Why is my run inconclusive?**
The report lists the reasons: no workload or fixture (`QUAL007`), too few
operations per scenario (`QUAL001`), a baseline that varies too much between
repeats (`QUAL002`), a latency change within the noise band, or a statement
the parser cannot read (`PS116`). Increase `measurement_seconds` or
`repeats`, use a quieter runner, or adjust the policy. ProdShadow does not
turn uncertainty into a pass.

**How do I model the long transaction that caused our last incident?**
Add a background session to the workload with a `transaction` and a
`hold_ms` that matches the real job's duration. See
[examples/long-running-transaction](../examples/long-running-transaction).

**Can I use it with Rails, Django, Prisma or Alembic?**
Yes, after rendering migrations to SQL (for example `rails db:migrate` with
SQL schema dumps, `manage.py sqlmigrate`, `prisma migrate diff --script`,
`alembic upgrade --sql`). Put previously applied migrations into the fixture
schema and pass the new one with `--migration`.

**How long does a run take?**
Roughly start-up + seeding + `repeats × 2 × (warmup + measurement + ~3 s)`.
The examples take about two minutes on a 4-CPU machine.

**Does it work on macOS?**
Yes with `network: loopback` (Docker Desktop cannot share unix sockets with
containers). Tested platforms are Linux CI runners.

**Why does building need a C compiler?**
The SQL parser is libpg_query, PostgreSQL's own parser, compiled through
cgo. Release binaries avoid this for users.

**Why `ps_app` and not my application role?**
Fixture, workload and migration run as an unprivileged per-run role so that
user SQL cannot read server files or start programs even inside the
container. Use `migration.run_as: superuser` only if the migration genuinely
needs it (for example to create an extension that is not trusted).

**Can ProdShadow import production statistics?**
Not yet. It is planned as a separate, read-only, opt-in command after a
security review; see the [roadmap](roadmap.md).
