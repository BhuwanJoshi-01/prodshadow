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
   hyphen (`v0.2.0-rc.1`) are marked as pre-releases.
7. **Update the Homebrew tap**: copy `prodshadow.rb` from the release to
   `Formula/prodshadow.rb` in `BhuwanJoshi-01/homebrew-tap` (see below).

`go install github.com/BhuwanJoshi-01/prodshadow/cmd/prodshadow@vX.Y.Z` works
as soon as the tag is pushed; pkg.go.dev picks the version up on its first
request. The documentation site deploys from `main` on its own.

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
