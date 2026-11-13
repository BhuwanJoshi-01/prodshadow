# Threat model and isolation invariants

ProdShadow executes SQL written by developers — migrations, fixtures and
workloads — and may run in CI on pull requests from people who are not
trusted. This document lists what must never happen, how the design
prevents it, and which tests prove it.

## Assets

1. **Production and other external databases.** ProdShadow must never
   connect to them.
2. **The host** running ProdShadow (developer laptop or CI runner).
3. **Secrets** available to the process: CI tokens, cloud credentials,
   environment variables.
4. **Trust in results.** A report must not claim more than was measured.

## Invariants

| # | Invariant | Mechanism | Verified by |
|---|---|---|---|
| I1 | SQL is executed only against servers created by the isolation manager in the same process. | No configuration field, flag or environment variable accepts a database address. `isolation.Server` has unexported credentials and a `managed` marker that only providers set; `Connect` refuses anything else. | `TestConnectRefusesUnmanagedServer`, `TestConfigSchemaHasNoConnectionStringField` |
| I2 | Connections go only to a unix socket directory or a loopback IP literal. | `security.CheckLocalAddress` before building the configuration, and again in the dialer (`guardedDial`) at connect time. Host names, including `localhost`, are rejected. | `TestCheckLocalAddress`, `TestGuardedDialRejectsRemoteAddresses`, `TestConnectRefusesRemoteHostBeforeDialing` |
| I3 | Environment variables cannot redirect connections. | Connection configurations are built field by field; `PGHOST`, `PGSERVICE`, TLS settings and fallback hosts are cleared. | `TestConnConfigIgnoresEnvironment` |
| I4 | Every session talks to *this run's* server. | Each server starts with a random `prodshadow.run_token` setting; every connection reads it back and aborts on mismatch. | `TestOwnershipTokenIsEnforced` |
| I5 | No shell interpretation of configuration. | External programs are run with argument vectors (`exec.Command`), never `sh -c`. Error messages do not echo arguments (which may contain the ephemeral password). | `TestStartFailureCleansUp` |
| I6 | The container cannot reach the network or the host. | `--network none` (unix socket shared through a private bind mount) or loopback-only port publishing; `--read-only`, tmpfs data directory, `--cap-drop ALL` plus only the capabilities the image entrypoint needs, `no-new-privileges`, memory, CPU and PID limits. The engine socket is never mounted. | `TestRunArgsAreHardened`, `TestContainerIsNetworkIsolated` |
| I7 | User SQL runs without superuser rights. | A per-run role `ps_app` (no superuser, no create-role, no replication) owns the fixture and runs workload and migration. `migration.run_as: superuser` is an explicit opt-in. | `TestDockerServerLifecycle` (COPY TO PROGRAM is refused) |
| I8 | Dangerous server features stay disabled. | Configuration rejects extensions that reach the network, files or untrusted languages, and settings that run commands or move sockets (`archive_command`, `listen_addresses`, ...). Fixtures may not use `COPY` to files or programs, `ALTER SYSTEM`, role management or `CREATE EXTENSION`. | `TestValidateReportsEachInvalidField`, `TestCheckFixtureStatement` |
| I9 | Secrets do not reach artifacts. | Ephemeral passwords and tokens are registered with a redactor; known credential formats and user patterns are redacted from every message; raw SQL and parameters are omitted unless enabled. | `TestSeededSecretNeverReachesArtifacts`, `TestRedactBuiltinPatterns` |
| I10 | Failed cleanup is visible. | Destroy errors are attached to the report and turn the run into an infrastructure failure (exit 4). `prodshadow cleanup` removes labelled leftovers and refuses unlabelled containers. | `TestRemoveOrphansRefusesUnlabelledContainers` |
| I11 | Results never overclaim. | No output says "safe"; missing inputs, too few samples and unstable baselines produce `inconclusive`, which fails CI unless configured otherwise. | `TestNoExperimentIsInconclusive`, `TestSchemaRejectsUnknownStatus` |

## Threats and responses

### A production DSN is supplied by mistake

There is nowhere to supply one. Even if code were changed to construct a
`Server` with a remote host, I2 and I4 stop the connection: the dialer
refuses non-loopback targets, and a foreign server lacks the run token.

### Malicious SQL in a pull request

Fixture and migration SQL can do anything an unprivileged role can do
inside a throwaway database with no network: fill the capped tmpfs, burn
CPU within the cgroup limit, or fail. `statement_timeout`, `lock_timeout`,
the measurement window and the container limits bound the damage. Escaping
the container would require a container-engine or kernel vulnerability.

Running untrusted pull requests is still a CI configuration concern: do not
expose deployment secrets to workflows triggered by forks (use
`pull_request`, not `pull_request_target`, for ProdShadow jobs).

### Secrets in SQL or error messages

Server error messages can echo SQL literals. They are redacted before they
reach reports, and raw SQL text is not persisted by default. Statement
fingerprints identify statements without revealing literals.

### Resource exhaustion on the host

Memory, CPU, PIDs and data size are capped by the container. The local
provider cannot enforce these limits and says so in `doctor` and in
validation warnings.

### Misleading results

The tool reports its own limits: sampling interval, missed samples,
baseline noise, representativeness of the fixture and the parser grammar.
Latency changes within the baseline's noise band are reported as
inconclusive, not as regressions.

## Out of scope

* Vulnerabilities in PostgreSQL, the container image or the container
  engine.
* Hosts where an attacker already controls the container engine.
* Read-only production statistics import, which is not implemented. When it
  is, it will use a separate command and a separate credential contract and
  must never share identities or pools with the executor.
