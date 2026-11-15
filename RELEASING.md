# Releasing ProdShadow

Releases are cut from `main` by pushing a tag. Everything after the tag is
automated, and nothing is published until a maintainer publishes the draft
release.

## Checklist

1. **Pick the version.** ProdShadow follows semantic versioning. Before 1.0,
   a minor version may change configuration or report fields; the report
   schema (`schema_version`) is versioned separately.
2. **Update `CHANGELOG.md`.** Move the entries under `[Unreleased]` to a new
   `## [X.Y.Z] - YYYY-MM-DD` section, write a one-paragraph summary at its
   top, and update the comparison links at the bottom. The section becomes
   the release notes (`scripts/release/notes.sh vX.Y.Z` prints them).
3. **Check `main` is green.** CI, Integration and the nightly jobs pass on
   the commit you will tag.
4. **Tag and push.**

   ```bash
   git switch main && git pull
   git tag -a vX.Y.Z -m "ProdShadow vX.Y.Z"
   git push origin vX.Y.Z
   ```

5. **Wait for the Release workflow.** It runs the tests and builds
   `prodshadow_X.Y.Z_<os>_<arch>.tar.gz` on Linux (amd64, arm64; statically
   linked) and macOS (arm64, amd64), checks each binary, records build
   provenance, and creates a **draft** release with the archives,
   `SHA256SUMS`, a Homebrew formula (`prodshadow.rb`) and the changelog
   section as notes.
6. **Review and publish the draft** on the Releases page. Tags with a
   hyphen (`v0.2.0-rc.1`) are marked as pre-releases. Publishing starts the
   **npm** workflow, which publishes the npm packages, and the **Container
   image** workflow, which pushes `ghcr.io/bhuwanjoshi-01/prodshadow` (see
   below). The install script picks the new release up by itself.
7. **Update the Homebrew tap**: copy `prodshadow.rb` from the release to
   `Formula/prodshadow.rb` in `BhuwanJoshi-01/homebrew-tap` (see below).

`go install github.com/BhuwanJoshi-01/prodshadow/cmd/prodshadow@vX.Y.Z` works
as soon as the tag is pushed; pkg.go.dev picks the version up on its first
request. The documentation site deploys from `main` on its own.

## Dry run

Actions → Release → Run workflow builds and checks all four platforms from
the chosen branch under a `v0.0.0-dev.N` version and keeps the archives as
workflow artifacts, without attestations or a release. Run it before tagging
when the build or the runners changed.

## Building a release archive locally

```bash
make dist VERSION=v0.1.0-dev
```

builds the archive for the machine you are on into `dist/`. The SQL parser
needs cgo, so each platform is built on its own operating system and
architecture rather than cross-compiled. Two builds of the same commit
produce identical archives.

## Homebrew

The formula installs the prebuilt binaries and shell completions. To set up
the tap once:

1. Create the public repository `BhuwanJoshi-01/homebrew-tap`.
2. Add the release's `prodshadow.rb` as `Formula/prodshadow.rb` and push.

Users then install with:

```bash
brew install BhuwanJoshi-01/tap/prodshadow
```

For each later release, replace `Formula/prodshadow.rb` with the new
release's `prodshadow.rb`. `brew test prodshadow` and
`brew audit --strict prodshadow` check it locally.

## npm

`npm install prodshadow` installs two packages: `prodshadow`, a small
launcher, and the one `prodshadow-<os>-<cpu>` package that matches the
machine (npm's `os` and `cpu` fields pick it), which holds the native
binary. No install scripts run and nothing is downloaded from GitHub.
`packaging/npm/stage.mjs` builds all five packages from a release's
archives; `packaging/npm/publish.sh` publishes them, platform packages
first, skipping versions already on the registry and using the `next`
dist-tag for pre-releases.

**One-time setup:**

1. Sign in to npmjs.com with the account that will own the packages
   (`prodshadow`, `prodshadow-darwin-arm64`, `prodshadow-darwin-x64`,
   `prodshadow-linux-arm64`, `prodshadow-linux-x64`; all were unclaimed on
   2026-11-15).
2. Create a granular access token with read and write access to packages
   (allowed to bypass two-factor authentication, since CI cannot answer a
   prompt).
3. Add it to the repository as the Actions secret `NPM_TOKEN` (Settings →
   Secrets and variables → Actions). The workflow runs in the `npm`
   environment, so you can require an approval before each publish
   (Settings → Environments → npm).

**Each release:** publishing the GitHub release runs the workflow. It
downloads the archives, checks them against `SHA256SUMS`, stages the
packages, installs and runs the Linux one, and publishes with provenance.
To try it without publishing, run the workflow by hand with *dry-run*
(Actions → npm → Run workflow).

**By hand**, for example from your own machine:

```bash
gh release download vX.Y.Z --repo BhuwanJoshi-01/prodshadow --dir dist \
  --pattern 'prodshadow_*.tar.gz' --pattern SHA256SUMS
node packaging/npm/stage.mjs --version vX.Y.Z --sums dist/SHA256SUMS
npm login
packaging/npm/publish.sh dist/npm --dry-run   # check, then run without --dry-run
```

Once the packages exist, npm's trusted publishing (configured per package
on npmjs.com for this repository and `npm.yml`) can replace the token.

## Container image

The Container image workflow builds `packaging/docker/Dockerfile` from the
release's static Linux binaries (no compilation, so arm64 needs no
emulation) and pushes `ghcr.io/bhuwanjoshi-01/prodshadow` with the tags
`X.Y.Z`, `X.Y` and, for stable releases, `latest`, plus an SBOM and
provenance. It uses the workflow's own token; nothing to set up, except
that after the first push the package is private: make it public once under
the repository's Packages → prodshadow → Package settings → Change
visibility. To build the image locally from archives in `dist/`:

```bash
packaging/docker/context.sh dist build/docker
docker build -t prodshadow:dev build/docker
```

## Install script

`install.sh` at the repository root downloads the latest release (or
`PRODSHADOW_VERSION`) for the machine, checks it against `SHA256SUMS` and
installs it. It needs no change per release. `PRODSHADOW_DOWNLOAD_URL`
points it at another server with the same layout, which the Packaging
workflow uses to test it.
