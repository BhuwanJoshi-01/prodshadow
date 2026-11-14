# Using ProdShadow with Prisma Migrate

Prisma Migrate already stores every migration as SQL in
`prisma/migrations/<timestamp>_<name>/migration.sql`, so no rendering step is
needed for committed migrations.

## 1. The new migration

Point ProdShadow at the newest directory:

```yaml
migration:
  files:
    - ../prisma/migrations/20261102093000_add_order_region/migration.sql
```

To check a schema change before creating the migration, let Prisma print the
SQL it would generate:

```bash
npx prisma migrate diff \
  --from-migrations prisma/migrations \
  --to-schema-datamodel prisma/schema.prisma \
  --shadow-database-url "$SHADOW_DATABASE_URL" \
  --script > prodshadow/migration.sql
```

`--from-migrations` needs a shadow database to replay the existing
migrations; use a local development server, never production.

## 2. The schema before it

Every earlier migration, in order, is the schema the new one runs against.
Directory names start with a timestamp, so lexical order is application
order:

```bash
ls -d prisma/migrations/*/ | sort | head -n -1 | while read -r d; do
  cat "$d/migration.sql"; echo
done > prodshadow/schema.sql
```

(`head -n -1` drops the newest migration, the one under test.)

## 3. Transactions

Keep `migration.transaction: auto`: ProdShadow wraps each file in a
transaction, and leaves a file unwrapped when it contains statements such
as `CREATE INDEX CONCURRENTLY` that cannot run in a transaction block.
Prisma does not generate those; if you add one by hand, put it in a
migration of its own so that it runs the same way under Prisma and under
ProdShadow.

## 4. Names

Prisma quotes model and field names, so tables and columns keep their case
(`"Order"`, `"createdAt"`). The workload's SQL must use the same quoted
names.
