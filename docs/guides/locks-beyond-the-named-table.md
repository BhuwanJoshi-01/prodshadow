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
partition they name. `REINDEX TABLE` and `CLUSTER` on a partitioned table
process each partition in its own transaction and cannot run inside a
transaction block. See the
[partition maintenance example](../../examples/partition-maintenance).

## Materialized views and views

`REFRESH MATERIALIZED VIEW` reads every table of the view's query
(`ACCESS SHARE`), so it waits behind an `ACCESS EXCLUSIVE` request on any of
them. Without `CONCURRENTLY` it holds `ACCESS EXCLUSIVE` on the view itself
for the whole refresh, blocking every reader ([PS113](../rules/PS113.md)).
`CREATE OR REPLACE VIEW` takes `ACCESS EXCLUSIVE` on the view it replaces.

## Sequences

`nextval()` takes `ROW EXCLUSIVE` on the sequence and holds it until the
transaction ends, including the implicit `nextval()` of serial and identity
columns. `ALTER SEQUENCE`, in any form, takes `SHARE ROW EXCLUSIVE`, so it
waits for every in-flight transaction that drew a value and blocks new
inserts while it waits ([PS156](../rules/PS156.md)).

## What is not modelled

* Locks taken inside functions, triggers you wrote, and `DO` blocks.
* Which partition an `INSERT` or a pruned query reaches at run time.
* Objects the fixture schema does not contain. Keep it close to
  production; `validate` warns about tables the workload or the migration
  uses that the schema does not create.

The experiment is the arbiter: `verify` runs the real statements against
the real schema and measures every wait, whether or not a prediction
covered it.
