# Example: partition maintenance

Creating next year's partition is routine maintenance, and the usual
statement for it locks the whole partitioned table.

* `events` is partitioned by `RANGE (at)` with one partition per year and
  200,000 seeded rows.
* The API reads recent events per account and inserts new ones.
* A background `nightly_export` session aggregates the table in a
  transaction that stays open for six seconds. It only holds
  `ACCESS SHARE`, which never blocks the API on its own.
* `0031_events_2027.sql` runs `CREATE TABLE events_2027 PARTITION OF events`,
  which needs `ACCESS EXCLUSIVE` on `events`.

```bash
prodshadow verify --config examples/partition-maintenance/prodshadow.yaml
```

**Expected result:** `block` (exit code 2) with

* `OBS002 migration-lock-timeout` – the `CREATE TABLE` queued behind the
  export and gave up after `lock_timeout` (2 s);
* `OBS011 sessions-queued-behind-migration` – every workload session queued
  behind the lock request while it waited, including reads of partitions the
  migration never touches;
* `PS141 create-partition-locks-parent` – the static rule that predicts it.

In our runs (PostgreSQL 17.12, three repeats) the migration timed out after
2.0 s in every repeat with all 8 workload sessions queued behind it. Without
a `lock_timeout` it would have waited for the rest of the export.

## The alternative

`alternatives/0031_events_2027.sql` creates the table on its own, adds a
`CHECK` constraint matching the partition bounds and then attaches it.
`ATTACH PARTITION` only needs `SHARE UPDATE EXCLUSIVE` on `events`, which
does not conflict with readers, writers or the export, and the matching
constraint lets PostgreSQL skip the scan that would otherwise prove every
row fits the bounds (`PS123`).

```bash
prodshadow verify --config examples/partition-maintenance/prodshadow.yaml \
  --migration examples/partition-maintenance/alternatives/0031_events_2027.sql
```

**Expected result:** `pass`. In our runs the migration finished in 8–35 ms
with no session queued behind it, while the export was still running.

> If the partitioned table has a `DEFAULT` partition, attaching takes
> `ACCESS EXCLUSIVE` on the default partition and scans it unless it has a
> constraint excluding the new range. Rehearse with your real layout.
