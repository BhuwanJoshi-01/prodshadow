# Safe migration patterns

Recipes for common schema changes that avoid long blocking locks, with the
rules that report the risky version. Each recipe assumes:

* every migration file starts with `SET lock_timeout = '2s';` and the deploy
  retries on `lock_not_available` ([PS114](../rules/PS114.md),
  [PS120](../rules/PS120.md));
* "separate transaction" means a separate migration file with
  `migration.transaction: per-file` (or `auto`), or an explicit
  `COMMIT`/`BEGIN` between the steps.

Run `prodshadow inspect` on each step and `prodshadow verify` with a
background session that holds a transaction open, so the lock queue is
exercised.

## Add a column

```sql
ALTER TABLE orders ADD COLUMN region text;                 -- instant
ALTER TABLE orders ADD COLUMN status text DEFAULT 'new';   -- instant: constant default
```

Volatile defaults (`gen_random_uuid()`, `clock_timestamp()`), `serial`,
identity and stored generated columns rewrite the table
([PS103](../rules/PS103.md)). Add the column without a default, set the
default for new rows, then backfill in batches.

## Make a column NOT NULL

```sql
-- 1
ALTER TABLE orders ADD CONSTRAINT orders_region_nn CHECK (region IS NOT NULL) NOT VALID;
-- 2, separate transaction
ALTER TABLE orders VALIDATE CONSTRAINT orders_region_nn;
-- 3, separate transaction
ALTER TABLE orders ALTER COLUMN region SET NOT NULL;      -- no scan: uses the validated check
ALTER TABLE orders DROP CONSTRAINT orders_region_nn;
```

Reported otherwise: [PS106](../rules/PS106.md), [PS136](../rules/PS136.md).

## Add a foreign key

```sql
-- 1
CREATE INDEX CONCURRENTLY refunds_order_id_idx ON refunds (order_id);
-- 2
ALTER TABLE refunds ADD CONSTRAINT refunds_order_fk
  FOREIGN KEY (order_id) REFERENCES orders (id) NOT VALID;
-- 3, separate transaction
ALTER TABLE refunds VALIDATE CONSTRAINT refunds_order_fk;
```

Reported otherwise: [PS107](../rules/PS107.md), [PS133](../rules/PS133.md),
[PS136](../rules/PS136.md), [PS137](../rules/PS137.md).

## Add a unique constraint or primary key

```sql
-- 1, outside a transaction block
CREATE UNIQUE INDEX CONCURRENTLY orders_reference_key ON orders (reference);
-- 2
ALTER TABLE orders ADD CONSTRAINT orders_reference_key UNIQUE USING INDEX orders_reference_key;
```

Reported otherwise: [PS101](../rules/PS101.md), [PS108](../rules/PS108.md).
Name concurrent indexes so a failed build can be dropped and retried
([PS132](../rules/PS132.md)).

## Change a column's type

Raising a `varchar` or `numeric` limit, or `varchar(n)` to `text`, does not
rewrite the table. Anything else does ([PS105](../rules/PS105.md)):

1. add a column of the new type;
2. write to both (trigger or application);
3. backfill in batches;
4. switch reads, then drop the old column in a later release.

## Rename a column or table

Running code still uses the old name ([PS110](../rules/PS110.md)). Add the
new column (or a view with the new table name), write to both, move readers,
then remove the old name in a later release. Check old releases with
[compatibility probes](../workload.md#compatibility-probes).

## Drop a column or table

Stop using it in a release that is fully deployed, then drop it
([PS109](../rules/PS109.md)). Avoid `CASCADE` ([PS124](../rules/PS124.md)).

## Backfill data

```sql
UPDATE orders SET region = 'eu' WHERE region IS NULL AND id BETWEEN 1 AND 10000;
-- commit, repeat for the next range
```

One unbounded `UPDATE` locks every row it touches until commit
([PS111](../rules/PS111.md)). Run batches outside the schema-change
transaction.

## Create or drop an index

Use `CONCURRENTLY` outside a transaction block
([PS101](../rules/PS101.md), [PS102](../rules/PS102.md),
[PS122](../rules/PS122.md)).

## Partitions

Create partitions with `LIKE` and attach them with a matching `CHECK`
constraint; detach with `CONCURRENTLY`
([PS123](../rules/PS123.md), [PS140](../rules/PS140.md),
[PS141](../rules/PS141.md)).

## Enum values

Add the value in one transaction and use it in the next
([PS128](../rules/PS128.md)). Renaming a value breaks running code
([PS110](../rules/PS110.md)).
