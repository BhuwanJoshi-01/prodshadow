# Locks beyond the named table

Most lock advice is about the table a statement names: `ALTER TABLE orders`
locks `orders`. Many statements also lock tables they do not name, and those
locks queue behind long transactions exactly like the obvious ones. This
guide lists the cases ProdShadow knows about. Every lock mode below that a
statement takes inside a transaction block is checked against `pg_locks` on
a real server by the lock oracle integration test, on each PostgreSQL
version in the nightly matrix.

ProdShadow learns the objects involved from the fixture schema
(`fixture.schema`). `plan`, `verify` and `inspect --config` list such locks
with their origin, for example
`ACCESS EXCLUSIVE on orders (via index orders_status_idx)`.

## Foreign keys

A foreign key from `invoices.customer_id` to `customers.id` installs
triggers on both tables.

| Statement | Also locks | Mode |
|---|---|---|
| `INSERT INTO invoices ...` | `customers` (checks the referenced row) | `ROW SHARE` |
| `UPDATE invoices SET customer_id = ...` | `customers` | `ROW SHARE` |
| `DELETE FROM customers ...`, or changing `customers.id` | `invoices` (checks for referencing rows) | `ROW SHARE` |
| the same with `ON DELETE CASCADE` / `SET NULL` / `SET DEFAULT` | `invoices` (changes the referencing rows), and onwards for cascades | `ROW EXCLUSIVE` |
| `ALTER TABLE invoices VALIDATE CONSTRAINT invoices_customer_fk` | `customers` | `ROW SHARE` |
| `ALTER TABLE invoices DROP CONSTRAINT invoices_customer_fk`, `DROP COLUMN customer_id`, `DROP TABLE invoices` | `customers` (its triggers are removed) | `ACCESS EXCLUSIVE` |
| `ALTER TABLE invoices ALTER COLUMN customer_id TYPE ...` or `ALTER TABLE customers ALTER COLUMN id TYPE ...` | the other table (the foreign key is re-created) | `ACCESS EXCLUSIVE` |
| `ALTER TABLE customers DROP CONSTRAINT customers_pkey CASCADE`, `DROP TABLE customers CASCADE` | `invoices` | `ACCESS EXCLUSIVE` |
| `TRUNCATE customers CASCADE` | `invoices`, and every table referencing it in turn | `ACCESS EXCLUSIVE` |

Two consequences are easy to miss:

* **A migration on the referenced table stalls inserts into the
  referencing one.** While `ALTER TABLE customers ...` waits for its
  `ACCESS EXCLUSIVE` lock, every `INSERT INTO invoices` queues behind it,
  because the foreign key check needs `ROW SHARE` on `customers`. `plan`
  lists such conflicts per scenario.
* **Dropping a foreign key is not a change to one table.** It takes
  `ACCESS EXCLUSIVE` on the referenced table as well, so it needs the same
  `lock_timeout` and retries as any other blocking DDL there.

The referential checks run only for rows a statement actually inserts,
changes or deletes, so they are the locks a statement *may* take. Without
`CASCADE`, `DROP TABLE` and `TRUNCATE` fail while another table's foreign
key still references the table ([PS157](../rules/PS157.md)).

## Indexes

`DROP INDEX` and `REINDEX INDEX` name an index but lock its table:
`ACCESS EXCLUSIVE` and `SHARE` respectively (`SHARE UPDATE EXCLUSIVE` with
`CONCURRENTLY`). [PS122](../rules/PS122.md) names the table when the schema
shows it.

## Partitions

DDL on a partitioned table recurses to every partition, with the same lock:
`ALTER TABLE events ...`, `CREATE INDEX ... ON events`, `LOCK TABLE events`,
`TRUNCATE events` and `DROP TABLE events`. `ONLY` stops the recursion.
`ATTACH PARTITION` and `DETACH PARTITION` lock the parent and only the
partition they name, plus the `DEFAULT` partition if there is one: it, and
`CREATE TABLE ... PARTITION OF`, take `ACCESS EXCLUSIVE` on the default
partition, whose implicit range changes. With a default partition,
`DETACH ... CONCURRENTLY` is not allowed at all ([PS161](../rules/PS161.md)). `REINDEX TABLE` and `CLUSTER` on a partitioned table
process each partition in its own transaction and cannot run inside a
transaction block.

Queries and DML on a partitioned table lock the partitions they reach, with
the same mode as the parent. Which ones depends on partition pruning at run
time, so ProdShadow lists every partition as a lock the statement may take.
A workload that queries the parent therefore conflicts with DDL on any one
partition, unless pruning keeps it away. See the
[partition maintenance example](../../examples/partition-maintenance).

## Materialized views and views

`REFRESH MATERIALIZED VIEW` reads every table of the view's query
(`ACCESS SHARE`), so it waits behind an `ACCESS EXCLUSIVE` request on any of
them. Without `CONCURRENTLY` it holds `ACCESS EXCLUSIVE` on the view itself
for the whole refresh, blocking every reader ([PS113](../rules/PS113.md)).
`CREATE OR REPLACE VIEW` takes `ACCESS EXCLUSIVE` on the view it replaces.

Queries on a view are rewritten into queries on its tables, so they lock
those: `SELECT` from a view takes `ACCESS SHARE` on every table behind it,
an `UPDATE` through a simple view takes `ROW EXCLUSIVE` on its table, and
`LOCK TABLE` on a view locks its tables in the requested mode. A workload
that reads through views therefore conflicts with a migration on the
tables underneath, and `plan` predicts it.

## Sequences

`nextval()` takes `ROW EXCLUSIVE` on the sequence and holds it until the
transaction ends, including the implicit `nextval()` of serial and identity
columns. `ALTER SEQUENCE`, in any form, takes `SHARE ROW EXCLUSIVE`, so it
waits for every in-flight transaction that drew a value and blocks new
inserts while it waits ([PS156](../rules/PS156.md)).

## Triggers

A trigger runs its function inside the statement that fired it, so an
`INSERT INTO orders` with an audit trigger also takes `ROW EXCLUSIVE` on
the audit table. ProdShadow reads the static SQL of PL/pgSQL and SQL
functions in the schema (including `PERFORM` and `BEGIN ATOMIC` bodies) and
adds those locks to `INSERT`, `UPDATE`, `DELETE`, `MERGE` and `TRUNCATE`
statements that fire the trigger, marked `via trigger <name>`. Row-level
triggers run only for affected rows, so these are locks the statement may
take. When a trigger's function writes a table that has triggers of its
own, those are followed too.

Rewrite rules (`CREATE RULE ... DO ALSO/INSTEAD ...`) add their actions to
every matching `INSERT`, `UPDATE` or `DELETE`, so their locks appear as
`via rule <name>`. Functions and procedures defined in the schema work the
same way: a query,
DML statement or `CALL` that runs one also takes the locks of its static
SQL, marked `via function <name>`.

## What is not modelled

* Dynamic SQL (`EXECUTE`) in functions and `DO` blocks, and functions or
  `DO` blocks in other languages. The static SQL of PL/pgSQL `DO` blocks is
  analysed, and triggers fired by a trigger function's own writes are
  followed.
* Which partition an `INSERT` or a pruned query reaches at run time.
* Objects the fixture schema does not contain. Keep it close to
  production; `validate` warns about tables the workload or the migration
  uses that the schema does not create.

The experiment is the arbiter: `verify` runs the real statements against
the real schema and measures every wait, whether or not a prediction
covered it.
