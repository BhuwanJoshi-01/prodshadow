# Example: dropping a foreign key

A migration that names one table and is blocked by activity on another.

* `invoices.customer_id` references `customers` through
  `invoices_customer_fk`.
* The API reads customers and invoices and issues invoices.
* A background `customer_export` session reads only `customers`, in a
  transaction that stays open for six seconds.
* `0052_drop_invoice_customer_fk.sql` runs
  `ALTER TABLE invoices DROP CONSTRAINT invoices_customer_fk`.

The statement names `invoices`, but dropping a foreign key also removes its
triggers from `customers`, so PostgreSQL takes `ACCESS EXCLUSIVE` on both
tables. `plan` shows it before anything runs, because it learns the foreign
key from the fixture schema:

```
1.1  ALTER TABLE — ACCESS EXCLUSIVE on invoices (blocks-reads-and-writes);
     ACCESS EXCLUSIVE on customers (via foreign key invoices_customer_fk, blocks-reads-and-writes)

Conflicts
  customer_profile             conflicts with 1.1 (ACCESS EXCLUSIVE on customers vs its ACCESS SHARE)
  customer_invoices            conflicts with 1.1 (ACCESS EXCLUSIVE on invoices vs its ACCESS SHARE)
  issue_invoice                conflicts with 1.1 (ACCESS EXCLUSIVE on invoices vs its ROW EXCLUSIVE)
  issue_invoice                conflicts with 1.1 (ACCESS EXCLUSIVE on customers vs its ROW SHARE)
  customer_export (background) conflicts with 1.1 (ACCESS EXCLUSIVE on customers vs its ACCESS SHARE)
```

```bash
prodshadow verify --config examples/foreign-key-drop/prodshadow.yaml
```

**Expected result:** `block` (exit code 2) with

* `OBS002 migration-lock-timeout` – the migration gave up after
  `lock_timeout` (2 s), waiting for `prodshadow/background/customer_export/0`,
  a session that never touched `invoices`;
* `OBS011 sessions-queued-behind-migration` – meanwhile every workload
  session queued behind the migration's lock on `invoices`;
* `PS114 missing-lock-timeout` – naming both tables.

In our runs (PostgreSQL 17.12, three repeats) the migration timed out after
2.0 s in every repeat, and invoice reads and inserts stalled for 2.0 s.
Whether customer reads stall too depends on timing: in one run they did
(2.0 s), in another they showed no stall, because each of the 8
closed-loop sessions had already issued an invoice query and was waiting
there, so none was left to read customers. `QUAL014` points out runs like
the second one; with more clients, customer reads queue behind the
migration's waiting request on `customers` as well.

**Rehearsing the fix:** there is no lock-free way to drop a foreign key.
Run it with a short `lock_timeout` and retries, as a deploy tool would:

```yaml
migration:
  lock_timeout_ms: 1000
  lock_retries: 6
  retry_backoff_ms: 500
```

In our runs each attempt gave up after 1 s, the migration succeeded on its
third attempt between two export transactions, and the longest stall
dropped to 1.0 s, inside the 2.5 s budget: the decision is `pass`, with
`OBS014` noting the two retries.
