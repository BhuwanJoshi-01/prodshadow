# Using ProdShadow with golang-migrate and goose

## golang-migrate

Each migration is a pair of files, `000042_add_order_region.up.sql` and
`.down.sql`. Pass the `up` file:

```yaml
migration:
  files:
    - ./migrations/000042_add_order_region.up.sql
```

Build the fixture schema from the earlier `up` files. Their names start
with a zero-padded version, so lexical order is application order:

```bash
ls migrations/*.up.sql | sort | grep -v '/000042_' | xargs cat > prodshadow/schema.sql
```

The PostgreSQL driver sends a file to the server as one multi-statement
query, which PostgreSQL runs as a single implicit transaction. That is what
`migration.transaction: per-file` reproduces; with `auto`, ProdShadow does
the same unless the file contains a statement that cannot run in a
transaction block, in which case golang-migrate fails while ProdShadow runs
it unwrapped. PS102 reports such files. Put `CREATE INDEX CONCURRENTLY` in a
migration of its own.

## goose

goose keeps the up and down steps in one file, separated by annotations:

```sql
-- +goose Up
ALTER TABLE orders ADD COLUMN region text;

-- +goose Down
ALTER TABLE orders DROP COLUMN region;
```

ProdShadow would execute both halves, so extract the up half first:

```bash
awk '/^-- \+goose Down/ { exit } { print }' migrations/00042_add_order_region.sql \
  > prodshadow/migration.sql
```

The annotations are SQL comments and need no further changes. goose runs
each migration in a transaction unless it contains
`-- +goose NO TRANSACTION`; use `migration.transaction: auto`, which leaves
files with `CREATE INDEX CONCURRENTLY` unwrapped. For the fixture, extract
the up halves of the earlier migrations the same way and concatenate them.
