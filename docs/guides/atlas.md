# Using ProdShadow with Atlas

Atlas writes versioned migrations as plain SQL files in the migration
directory (`migrations/20261102093000_add_region.sql`, with an `atlas.sum`
checksum file), so they can be passed to ProdShadow unchanged.

## 1. The new migration

```yaml
migration:
  files:
    - ../migrations/20261102093000_add_region.sql
```

To rehearse a change before generating the file, let Atlas write it with
`atlas migrate diff` against a dev database, then point ProdShadow at the
new file. ProdShadow complements Atlas's own lint analyzers: they read the
SQL, ProdShadow also measures the lock queue under load.

## 2. The schema before it

File names start with a version that sorts lexically, so concatenating the
earlier files in order rebuilds the schema:

```bash
ls migrations/*.sql | sort | grep -v '/20261102093000_' | xargs cat > prodshadow/schema.sql
```

## 3. Transactions

`atlas migrate apply` runs each file in its own transaction by default
(`--tx-mode file`). A file that needs to run outside one, for
`CREATE INDEX CONCURRENTLY`, starts with the `-- atlas:txmode none`
directive. Keep `migration.transaction: auto`: ProdShadow wraps each file
in a transaction and leaves files containing such statements unwrapped,
which matches. The directive is an SQL comment and needs no change.
