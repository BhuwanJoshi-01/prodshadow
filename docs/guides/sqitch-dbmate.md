# Using ProdShadow with sqitch and dbmate

Both tools keep migrations as plain SQL, so no rendering step is needed.

## sqitch

Each change has a deploy script, `deploy/<change>.sql`, listed in order in
`sqitch.plan`. `sqitch add` generates scripts that wrap the change in
`BEGIN;` / `COMMIT;`, which ProdShadow follows with
`migration.transaction: auto`.

```yaml
migration:
  files:
    - ../deploy/orders_region.sql
```

For the fixture, concatenate the deploy scripts of the earlier changes in
plan order. Change names in `sqitch.plan` are the first word of each line
that does not start with `%`, `@` or `#`:

```bash
awk '!/^(%|@|#|$)/ { print $1 }' sqitch.plan | grep -vx orders_region \
  | sed 's|.*|deploy/&.sql|' | xargs cat > prodshadow/schema.sql
```

Revert and verify scripts are not used. sqitch records deployments in its
own registry schema, which ProdShadow does not need.

## dbmate

dbmate keeps the up and down steps of a migration in one file, after
`-- migrate:up` and `-- migrate:down` markers:

```sql
-- migrate:up
ALTER TABLE orders ADD COLUMN region text;

-- migrate:down
ALTER TABLE orders DROP COLUMN region;
```

ProdShadow would run both halves, so extract the up half first:

```bash
awk '/^-- migrate:down/ { exit } { print }' db/migrations/20261102093000_add_region.sql \
  > prodshadow/migration.sql
```

dbmate runs each migration in a transaction unless its up marker says
`-- migrate:up transaction:false`; keep `migration.transaction: auto`,
which leaves files with `CREATE INDEX CONCURRENTLY` unwrapped. For the
fixture, `dbmate dump` writes the current schema to `db/schema.sql` from a
development database that has every earlier migration applied, or extract
and concatenate the earlier up halves as above.
