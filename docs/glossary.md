# Glossary

**Background session** — A workload session outside the weighted mix that
runs a transaction on a schedule, often holding it open (`hold_ms`). It
models the long report, batch job or idle-in-transaction connection that a
migration ends up waiting for. See [workload](workload.md#background-sessions).

**Baseline phase** — A measurement window with the workload running and no
migration. Its latencies are what the candidate is compared with.

**Blocker** — A session holding a lock that another session is waiting for.
Reports name blockers by `application_name`, as reported by
`pg_blocking_pids()`.

**Candidate phase** — A measurement window in which the migration starts
`migration.start_after_seconds` after the workload.

**Disposable server** — The PostgreSQL server ProdShadow starts for a run
and destroys afterwards. It is labelled `io.prodshadow.managed=true` and
verified with a per-run ownership token on every connection.

**Evidence kind** — How a finding is known: `static` (from the SQL and
PostgreSQL's locking rules), `heuristic` (a rule of thumb), `observed`
(measured in the experiment) or `quality` (about the experiment itself).

**Fixture** — The schema and data the experiment runs against: a schema
file, optional seed SQL and an optional deterministic generator. Its hash
identifies it in reports.

**Implied lock** — A lock a statement takes on a relation it does not name:
`DROP INDEX` on the index's table, `INSERT` on the table a foreign key
references, DDL on a partitioned table on its partitions, a query on a view
on the view's tables, a trigger's statements on the tables they write.
Predicted from the [schema catalog](#schema-catalog) and shown as
`via ...`. See [Locks beyond the named table](guides/locks-beyond-the-named-table.md).

**Inconclusive** — A result that cannot support a decision, for example too
few samples or a noisy baseline. It is never reported as a pass; by default
it exits with code 5.

**Lock queue** — PostgreSQL grants table locks in request order. A migration
waiting for `ACCESS EXCLUSIVE` makes every later request on the table wait
behind it, even a plain `SELECT`, which is how an instant `ALTER TABLE`
causes an outage.

**Lock oracle** — The integration test that runs each classified statement
inside a transaction on a real server and compares its locks in
`pg_locks` with the prediction, on every supported PostgreSQL version
([ADR 0012](adr/0012-lock-oracle.md)).

**Noise band** — The variation between baseline repeats. Latency changes
inside it are not attributed to the migration.

**Predicted conflict** — A workload session whose locks conflict with a
migration statement's, according to the lock conflict matrix. `plan` lists
them before a run; reports show them next to each scenario's measured
stall. A conflict means whichever side comes second waits; the experiment
measures for how long.

**Repeat** — One baseline phase and one candidate phase, in alternating
order. Several repeats separate the migration's effect from run-to-run
noise.

<a id="schema-catalog"></a>**Schema catalog** — What the static analysis learns from the fixture schema
(or `inspect --schema`): indexes, partitions, views, sequences, keys,
foreign keys, functions, triggers and rules. It follows the migration's own
creates, renames and drops ([ADR 0011](adr/0011-schema-catalog.md)).

**Stall** — A client-side measurement: an operation that started during the
migration and took longer than the scenario's baseline p99.

**Suppression** — A reviewed exception for a finding, inline
(`-- prodshadow:ignore`) or in a suppression file, with a reason and
optional expiry. Expired suppressions are reported.

**Template database** — The fixture is loaded once into a template, and each
phase gets a fresh copy, so every phase starts from the same data.

**Workload** — The concurrent traffic: weighted scenarios run by a pool of
sessions (closed loop) or at a fixed arrival rate (open loop), plus
background sessions.
