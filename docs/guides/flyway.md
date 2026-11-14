# Using ProdShadow with Flyway

Flyway SQL migrations (`V<version>__<description>.sql`) are plain SQL and
can be passed to ProdShadow unchanged. Java-based migrations are not
covered.

## 1. The new migration

```yaml
migration:
  files:
    - ../src/main/resources/db/migration/V42__add_order_region.sql
```

## 2. The schema before it

Concatenate the earlier versioned migrations in version order. Flyway
compares versions numerically (`V9` before `V10`), so sort with `sort -V`
rather than lexically:

```bash
ls src/main/resources/db/migration/V*.sql | sort -V | grep -v '/V42__' \
  | xargs cat > prodshadow/schema.sql
```

Repeatable migrations (`R__*.sql`), such as views, run after the versioned
ones; append them if the new migration depends on them. Placeholders
(`${schema}`) must be replaced first, for example with `sed`, because
ProdShadow executes the SQL as written.

## 3. Transactions

On PostgreSQL, Flyway runs each migration in its own transaction unless the
script's configuration says otherwise (`executeInTransaction=false` in a
`V42__add_order_region.sql.conf` file), which is needed for
`CREATE INDEX CONCURRENTLY`. With `migration.transaction: auto`, ProdShadow
also wraps each file and leaves files with such statements unwrapped, which
matches.

# Using ProdShadow with Liquibase

Liquibase can print the SQL it would run without applying it:

```bash
liquibase update-sql --changelog-file=db/changelog.xml > prodshadow/migration.sql
```

The output includes statements that maintain Liquibase's own
`DATABASECHANGELOG` and `DATABASECHANGELOGLOCK` tables. Either create those
tables in the fixture (dump them from a development database together with
the schema) or delete those statements from the rendered file. Point
`update-sql` at a development database that has every earlier changeset
applied, so only the new changesets are rendered.
