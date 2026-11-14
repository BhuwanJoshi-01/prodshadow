# Examples

Each directory is a complete ProdShadow project: configuration, fixture,
workload and migration, plus a README with the expected decision and what
to change. Run any of them from the repository root:

```bash
prodshadow verify --config examples/<name>/prodshadow.yaml
```

| Example | Migration | What happens | Expected |
|---|---|---|---|
| [safe-add-column](safe-add-column) | `ADD COLUMN display_name text` with `lock_timeout` | The brief `ACCESS EXCLUSIVE` lock is granted at once; nothing queues. | `pass` |
| [long-running-transaction](long-running-transaction) | `ADD COLUMN external_reference text` | A report holds `ACCESS SHARE` for 6 s; the `ALTER TABLE` queues behind it and all API queries queue behind the `ALTER TABLE` until `lock_timeout`. | `block` — OBS002 |
| [blocking-index](blocking-index) | `CREATE INDEX` on 800k generated rows | Writers wait for the whole build (`SHARE` lock); reads continue. The `CONCURRENTLY` alternative does not block. | `block` — OBS001 |
| [backfill-heavy](backfill-heavy) | `UPDATE subscribers SET … ` without `WHERE` | Every row stays locked until the backfill commits; concurrent writers wait. | `block` — OBS001 |
| [rolling-deploy](rolling-deploy) | `DROP COLUMN legacy_sku` | The previous application version still selects the column and fails. | `block` — OBS007, OBS013 |
| [partition-maintenance](partition-maintenance) | `CREATE TABLE … PARTITION OF events` | The new partition needs `ACCESS EXCLUSIVE` on the partitioned table; it queues behind an export and every query on every partition queues behind it. Attaching a prepared table does not block. | `block` — OBS002 |

`go test -tags integration -run TestExamples ./tests/integration/` runs all
of them and checks these outcomes; CI does the same weekly.

Durations in these examples are small because the fixtures are small. They
demonstrate *behaviour* — who blocks whom — not production timings.
