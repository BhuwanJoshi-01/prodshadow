# Example: rolling deploy compatibility

During a rolling deploy the old and new application versions run at the
same time. The workload declares both: `v2_get_product` (new code) and
`v1_get_product` (old code, still selecting `legacy_sku`).

```bash
prodshadow verify --config examples/rolling-deploy/prodshadow.yaml
```

**Expected result:** `block` (exit code 2) with `OBS007 error-rate-increase`
on `v1_get_product`: once the column is gone every old-version query fails
with `undefined_column`. The static rule `PS109` asks for an explicit review
of the destructive change.

**What to change:** ship a release that stops reading `legacy_sku`, wait
until no instance of the old version is running, then drop the column.
