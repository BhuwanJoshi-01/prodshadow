# Example: long-running transaction

The failure mode behind most public migration postmortems.

* `0007_add_invoice_reference.sql` adds a nullable column, a metadata-only
  change that completes in milliseconds on an idle table.
* The workload includes a background `revenue_report` session that keeps a
  read transaction open for six seconds, holding `ACCESS SHARE` on
  `invoices`.
* `ALTER TABLE` needs `ACCESS EXCLUSIVE`, so it queues behind the report;
  every API query that arrives afterwards queues behind the `ALTER TABLE`.

```bash
prodshadow verify --config examples/long-running-transaction/prodshadow.yaml
```

**Expected result:** `block` (exit code 2) with

* `OBS002 migration-lock-timeout` – the migration gave up after
  `lock_timeout` (2 s) because `prodshadow/background/revenue_report/0`
  held a conflicting lock;
* `OBS011 sessions-queued-behind-migration` – all workload sessions queued
  behind the lock request while it waited.

**What to change:** keep the migration, but run it with a short
`lock_timeout` and retries (and avoid deploying while long transactions run).
In production without a `lock_timeout`, the `ALTER TABLE` would have waited
for the whole report and stalled the API for that time.
