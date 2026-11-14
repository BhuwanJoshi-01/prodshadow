# Example: rolling deploy compatibility

During a rolling deploy the old and new application versions run at the
same time. The workload declares both: `v2_get_product` (new code) and
`v1_get_product` (old code, still selecting `legacy_sku`). Its
`compatibility` section also lists every query each version depends on,
including a rarely used export that the traffic mix does not cover.

```bash
prodshadow verify --config examples/rolling-deploy/prodshadow.yaml
```

**Expected result:** `block` (exit code 2) with `OBS007 error-rate-increase`
on `v1_get_product`: once the column is gone every old-version query fails
with `undefined_column`. `OBS013 app-version-incompatible` names version
`v1` and both of its probes, `get_product` and `export_skus`, while `v2`'s
probe keeps working; the report's "Application compatibility" table shows
each probe before and after the migration. The static rule `PS109` asks for an explicit review
of the destructive change.

You may also see `OBS005 p95-latency-regression` on `v2_get_product`. That is
a real second-order effect, not noise: after the drop, old-version clients
produce thousands of failing queries per second, and the server work spent on
those errors slows the healthy new-version queries (in our runs p95 rose by
about 1.7× while baseline repeats varied by 4 %). An error storm from stale
clients is itself an availability risk.

**What to change:** ship a release that stops reading `legacy_sku`, wait
until no instance of the old version is running, then drop the column.
