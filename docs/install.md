# Installation

ProdShadow is a single binary. `inspect`, `plan`, `validate`, `rules` and
`locks` need nothing else. `verify` also needs a container engine (Docker,
or Podman) to start its disposable PostgreSQL server, or installed
PostgreSQL binaries for the `local` provider; see the
[README](../README.md#quick-start) for the requirements.

!!! info "Release channels start with v0.1.0"

    npm, Homebrew, the install script, the container image and the release
    archives install published releases. Until the first release is out,
    install with `go install`.

=== "npm"

    ```bash
    npm install --save-dev prodshadow
    npx prodshadow version
    ```

    or `npm install -g prodshadow` for a global command. npm installs the
    native binary for your machine from a per-platform package
    (`prodshadow-linux-x64`, `prodshadow-darwin-arm64`, ...); no install
    scripts run. Node.js 18 or later.

=== "Homebrew"

    ```bash
    brew install BhuwanJoshi-01/tap/prodshadow
    ```

    Installs the binary and shell completions for bash, zsh and fish.

=== "Install script"

    ```bash
    curl -fsSL https://raw.githubusercontent.com/BhuwanJoshi-01/prodshadow/main/install.sh | sh
    ```

    Downloads the latest release for your platform, checks it against the
    release's `SHA256SUMS`, and installs it to `/usr/local/bin` if that is
    writable, otherwise to `~/.local/bin`. Set `PRODSHADOW_VERSION=v0.1.0`
    to pin a release and `PRODSHADOW_INSTALL_DIR` to choose the directory.
    Read the [script](../install.sh) before piping it to a shell if you
    prefer.

=== "Go"

    ```bash
    go install github.com/BhuwanJoshi-01/prodshadow/cmd/prodshadow@latest
    ```

    Builds from source: Go 1.26 or later and a C compiler (the SQL parser,
    libpg_query, uses cgo). This also works on platforms without a prebuilt
    binary.

=== "Container"

    ```bash
    docker run --rm -v "$PWD:/work" ghcr.io/bhuwanjoshi-01/prodshadow:latest \
      inspect --migration migrations/0042_add_index.sql --fail-on warning
    ```

    A distroless image (linux/amd64 and linux/arm64) with the release
    binary, for CI systems without Go or Node. It runs the commands that
    need no database; `verify` starts its own container and needs the
    binary on a host with Docker or Podman.

=== "Release archive"

    Download `prodshadow_<version>_<os>_<arch>.tar.gz` and `SHA256SUMS`
    from the [releases page](https://github.com/BhuwanJoshi-01/prodshadow/releases),
    then:

    ```bash
    sha256sum --check --ignore-missing SHA256SUMS   # macOS: shasum -a 256 -c
    tar -xzf prodshadow_0.1.0_linux_amd64.tar.gz
    sudo install prodshadow_0.1.0_linux_amd64/prodshadow /usr/local/bin/
    ```

## Supported platforms

| Platform | npm, Homebrew, script, archive | Container | `go install` |
|---|---|---|---|
| Linux x86-64 | yes (static binary, any C library) | yes | yes |
| Linux arm64 | yes (static binary, any C library) | yes | yes |
| macOS Apple silicon | yes | – | yes |
| macOS Intel | yes | – | yes |
| Windows | inside WSL | – | inside WSL |

## Verifying a download

Every release archive has a SHA-256 checksum in `SHA256SUMS` and a build
provenance attestation that ties it to the release workflow run that built
it. With the GitHub CLI:

```bash
gh attestation verify prodshadow_0.1.0_linux_amd64.tar.gz --repo BhuwanJoshi-01/prodshadow
```

npm packages carry npm provenance, shown on their npm page, and the
container image has an SBOM and a provenance attestation.

## Shell completion

```bash
source <(prodshadow completion bash)   # or zsh; fish: prodshadow completion fish | source
```

Add the line to your shell's start-up file to keep it. Homebrew installs
completions for you.

## Checking the installation

```bash
prodshadow version   # version, commit and parser
prodshadow doctor    # container engine, configuration and fixture checks
```

## Upgrading and uninstalling

Upgrade with the tool you installed with (`npm update prodshadow`,
`brew upgrade prodshadow`, or the install script again). The binary keeps
no state outside your project: uninstalling is removing it
(`npm uninstall prodshadow`, `brew uninstall prodshadow`, or deleting the
file), and `prodshadow cleanup` first removes any containers a crashed run
left behind.
