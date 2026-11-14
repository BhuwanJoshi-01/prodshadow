# Using ProdShadow with Django

Django renders a migration's SQL with `sqlmigrate`. ProdShadow needs that
SQL plus the schema as it is before the migration.

## 1. Render the new migration

```bash
python manage.py sqlmigrate orders 0042_order_region > prodshadow/migration.sql
```

`sqlmigrate` connects to the database in your settings to generate some
statements (for example constraint names it has to look up), so point it at
a development database that has the earlier migrations applied.

For an atomic migration (the default on PostgreSQL) the output is wrapped in
`BEGIN;` / `COMMIT;`, which ProdShadow follows. Migrations with
`atomic = False`, such as those using `AddIndexConcurrently` from
`django.contrib.postgres.operations`, are rendered without a transaction;
keep `migration.transaction: auto` so ProdShadow does not add one.

`RunPython` operations cannot be rendered; `sqlmigrate` prints a comment
saying so. They are not executed and not covered. `RunSQL` appears as
written.

## 2. Dump the schema before it

Migrate a development database to the previous migration and dump it:

```bash
python manage.py migrate orders 0041_order_notes
pg_dump --schema-only --no-owner --no-privileges "$DEV_DATABASE_URL" > prodshadow/schema.sql
```

Remove `CREATE EXTENSION` statements from the dump and list the extensions in
`postgres.extensions` instead; the fixture runner refuses them.

## 3. Configure

```yaml
fixture:
  schema: ./prodshadow/schema.sql
  generator:
    path: ./prodshadow/generator.yaml
migration:
  files:
    - ./prodshadow/migration.sql
  transaction: auto
```

Table names are `<app>_<model>` (`orders_order`), which is also what the
workload's SQL must use.

## 4. Several migrations in one deploy

Render each one into its own file in order (`0042.sql`, `0043.sql`) and list
them, or use a glob such as `prodshadow/migrations/*.sql`. Each file keeps
its own transaction, as when Django applies them.
