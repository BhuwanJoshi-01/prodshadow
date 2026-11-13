# Example: blocking index build

`CREATE INDEX` without `CONCURRENTLY` takes a `SHARE` lock for the entire
build. Reads continue; inserts and updates wait. The fixture is generated
deterministically from `fixtures/generate.yaml` (800,000 rows, Zipf-skewed
customers), so every run seeds identical data.

```bash
prodshadow verify --config examples/blocking-index/prodshadow.yaml
```

**Expected result:** `block` (exit code 2) with `OBS001 lock-wait-budget`:
write scenarios stall for the duration of the build, longer than the
configured 250 ms budget. The static rule `PS101` explains why.

Now rehearse the non-blocking variant:

```bash
prodshadow verify --config examples/blocking-index/prodshadow.yaml \
  --migration examples/blocking-index/alternatives/0012_index_orders_by_status_concurrently.sql
```

**Expected result:** no lock-wait finding. `PS121` reminds you that
`CONCURRENTLY` waits for older transactions and can leave an invalid index if
it fails.

> On a production table with hundreds of millions of rows the build takes far
> longer than here. The experiment demonstrates the *blocking behaviour*; its
> duration is not a prediction.
