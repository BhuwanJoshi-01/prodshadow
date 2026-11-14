# Example: materialized view refresh

A dashboard reads per-customer revenue from a materialized view; a data
migration refreshes it after historical orders were corrected.

* `orders` has 1.5 million rows; `daily_revenue` aggregates them per day
  and customer and has a unique index.
* The dashboard reads one customer's last 30 days from the view; new
  orders keep arriving in `orders`.
* `0044_refresh_daily_revenue.sql` runs `REFRESH MATERIALIZED VIEW
  daily_revenue`, which holds `ACCESS EXCLUSIVE` on the view for the whole
  recomputation.

```bash
prodshadow verify --config examples/matview-refresh/prodshadow.yaml
```

**Expected result:** `block` (exit code 2) with

* `OBS001 lock-wait-budget` – dashboard queries waited for the whole
  refresh, far beyond the 250 ms budget;
* `OBS011 sessions-queued-behind-migration` – every workload session that
  read the view queued behind it;
* `PS113 rewrite-under-exclusive-lock` – the static rule that predicts it.

`plan` predicts the same before anything runs: `customer_revenue`
conflicts with the refresh, `record_order` does not.

In our runs (PostgreSQL 17.12, three repeats) the refresh took 1.3 to
6.4 s, all 8 dashboard sessions queued behind it, and the longest
dashboard query took 6.4 s. Inserts into `orders` were unaffected: the
refresh only reads the table (`ACCESS SHARE`).

## The alternative

`alternatives/0044_refresh_daily_revenue.sql` uses
`REFRESH MATERIALIZED VIEW CONCURRENTLY`. It computes the new contents next
to the old ones and applies the difference under `EXCLUSIVE`, which lets
`SELECT` on the view continue. It needs a unique index on the view, which
the schema has.

```bash
prodshadow verify --config examples/matview-refresh/prodshadow.yaml \
  --migration examples/matview-refresh/alternatives/0044_refresh_daily_revenue.sql
```

**Expected result:** no session queues behind the refresh. In our runs the
longest dashboard query during the refresh took 26 ms instead of 6.4 s.

The concurrent refresh is not free: it took 5.5 to 6.5 s, about four times
as long, and competes for CPU and I/O. In one of our two runs, one repeat
in three saw an insert stall of 290 ms with no lock wait at all, just over
the 250 ms budget, and `OBS001` blocked with low confidence; the other run
passed. A stall that no lock conflict explains is what `OBS017` reports.
On production hardware the effect depends on spare capacity, so rehearse
with representative data and keep refreshes off peak.
