# Troubleshooting

Start with `prodshadow doctor`: it checks the configuration, the container
provider, the image and leftover servers, and prints one line per check.
Exit codes are listed in the [CLI reference](cli.md#exit-codes); exit code 4
means the infrastructure failed, not your migration.

## The server does not start

**`network "none" shares a unix socket with the container and is only supported on Linux; use network: loopback`**
Docker Desktop (macOS, Windows) cannot share a unix socket between the host
and a container. Set `postgres.isolation.network: loopback`; the port is
published on `127.0.0.1` only.

**`container exited during start-up`** or **`server did not become ready`**
The error includes the last lines of the server log. Common causes:

* A parameter in `postgres.settings` that the server rejects (a typo, or a
  value too large for `memory_mb`).
* `ephemeral_disk_mb` too small for the fixture: the data directory lives in
  a tmpfs of that size. Generated rows and indexes need room; raise it.
* A slow runner pulling the image during start-up: pull it in an earlier CI
  step, or raise `startup_timeout_seconds`.

**`image not found`** / registry errors
The image is pulled on first use. In CI, Docker Hub's anonymous pull limit
is a common cause; log in before the run or mirror the image, and pin it by
digest.

**`the local provider cannot run as root because PostgreSQL refuses to`**
PostgreSQL will not start as root. Use the docker or podman provider, or
run as an unprivileged user.

**`no local PostgreSQL <major> binaries found`**
Install the server package for that major version, or set
`postgres.isolation.binary` to its `bin` directory (for example
`/usr/lib/postgresql/17/bin`).

## The run is inconclusive

The report's decision lists why. The common reasons:

| Reason | What to do |
|---|---|
| No fixture or workload (`QUAL007`) | Add `fixture.schema` and a workload; without them nothing is measured. |
| Too few operations (`QUAL001`) | Raise `measurement_seconds` or `concurrency`, or make scenarios cheaper. |
| Noisy baseline (`QUAL002`) | Use a quieter runner, raise `repeats`, or pin `cpus` lower than the runner has. |
| A latency change inside the noise band | Same as above; ProdShadow does not round uncertainty to a pass. |
| A statement the parser cannot read (`PS116`) | Newer syntax than the bundled grammar. It still runs; set `PS116: info` if that is expected. |

## The migration fails in the disposable database

`OBS003` reports the SQLSTATE and the statement. Frequent ones:

| SQLSTATE | Meaning | Usual cause |
|---|---|---|
| `42P01` | undefined table | The fixture schema is missing earlier migrations. |
| `42501` | insufficient privilege | The migration needs more than the owner role; see `migration.run_as`. |
| `25001` | active SQL transaction | A statement such as `CREATE INDEX CONCURRENTLY` inside a transaction (PS102). |
| `55P03` | lock not available | `OBS002`: the lock was not granted within `lock_timeout_ms`. |
| `55P04` | unsafe use of new enum value | `PS128`: an enum value used in the transaction that added it. |
| `23502` / `23505` | not null / unique violation | The fixture's data violates the new constraint, as production data may. |

## Leftover containers

Interrupted runs can leave servers behind. `prodshadow cleanup --dry-run`
lists them (they carry the `io.prodshadow.managed=true` label) and
`prodshadow cleanup` removes them; it never touches containers without the
label.

## Results differ between runs

Small differences are expected; the report states the noise band it used.
Large differences usually mean the runner was busy. Compare runs only when
the fixture and workload hashes match; `prodshadow history` marks runs where
they changed.
