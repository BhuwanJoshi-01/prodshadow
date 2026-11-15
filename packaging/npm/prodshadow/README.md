# prodshadow

**Rehearse PostgreSQL migrations under load, not in production.**

ProdShadow runs your migration in a disposable PostgreSQL server while a
workload you define keeps querying it, compares the result with an identical
run without the migration, and tells you which sessions waited, for how long,
and behind which lock.

```bash
npm install --save-dev prodshadow     # or: npm install -g prodshadow
npx prodshadow init      # starter configuration, fixture, workload and migration
npx prodshadow plan      # predicted locks and conflicts, no database needed
npx prodshadow verify    # the full experiment (needs Docker or Podman)
```

This package runs the native `prodshadow` binary for your platform, which
npm installs as an optional dependency (`prodshadow-<os>-<cpu>`). Nothing is
downloaded by install scripts. Prebuilt binaries exist for Linux (x64 and
arm64, statically linked, so glibc and musl distributions both work) and
macOS (Apple silicon and Intel). On Windows, install it inside WSL.

Set `PRODSHADOW_BINARY` to run a different binary through the same command.

- Documentation: <https://bhuwanjoshi-01.github.io/prodshadow/>
- Source, issues and releases: <https://github.com/BhuwanJoshi-01/prodshadow>
- Other ways to install: Homebrew, `go install`, release archives (see the
  documentation)

ProdShadow produces evidence and uncertainty. A passing result means no
configured threshold was exceeded in the experiment, not that the migration
is safe in production.

Licensed under Apache-2.0.
