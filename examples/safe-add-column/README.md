# Example: safe column addition

A non-alarm case. The migration sets `lock_timeout` and adds a nullable
column, which PostgreSQL records in the catalog without rewriting the table.
The workload consists of short transactions only, so the brief
`ACCESS EXCLUSIVE` lock is granted almost immediately.

```bash
prodshadow verify --config examples/safe-add-column/prodshadow.yaml
```

**Expected result:** `pass` (exit code 0), reported as *no configured
threshold was exceeded in this experiment* — not as "safe for production".
The static rules note nothing blocking; `QUAL003` records that the fixture is
a manual fixture.

Compare with [`long-running-transaction`](../long-running-transaction): the
same kind of statement blocks traffic as soon as a long transaction is open.
