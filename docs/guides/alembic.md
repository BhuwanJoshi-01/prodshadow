# Using ProdShadow with Alembic

Alembic migrations are Python, but Alembic can render them to SQL without a
database ("offline mode"). ProdShadow needs two SQL files: the schema as it
is before the new migration (the fixture), and the new migration itself.

## 1. Render the new migration

Find the revision before the new one with `alembic history`, then render the
range from it to the new revision:

```bash
alembic upgrade 3f1c2a9b7d10:8a4e0c6f2b51 --sql > prodshadow/migration.sql
```

Offline output wraps the statements in `BEGIN;` / `COMMIT;` and ends with
`UPDATE alembic_version SET version_num='...'`. ProdShadow honours the
explicit transaction (with `migration.transaction: auto`) and the update
needs an `alembic_version` table in the fixture (step 2).

Operations written with `op.execute()` appear as written. Data migrations
that run Python code against a connection cannot be rendered offline; they
fail in offline mode and are not covered.

## 2. Render the schema before it

```bash
alembic upgrade base:3f1c2a9b7d10 --sql > prodshadow/schema.sql
```

This creates `alembic_version` and every table up to the revision before
the new migration. Alternatively, dump a development database that is at
that revision:

```bash
pg_dump --schema-only --no-owner --no-privileges "$DEV_DATABASE_URL" > prodshadow/schema.sql
```

Remove statements the fixture runner refuses (`CREATE EXTENSION` goes to
`postgres.extensions`; ownership and grants are dropped by the flags above).

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

Concurrent index builds need `op.create_index(..., postgresql_concurrently=True)`
inside `with op.get_context().autocommit_block():`. Offline mode then emits
`COMMIT;` before and `BEGIN;` after the statement, which ProdShadow follows
(PS102 checks that it really runs outside a transaction).

## 4. In CI

Render both files in the job before `prodshadow verify`, so they always
match the branch under test. See the [CI guide](../ci.md).
