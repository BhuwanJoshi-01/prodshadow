#!/usr/bin/env bash
# Builds one release archive: dist/prodshadow_<version>_<goos>_<goarch>.tar.gz
#
#   scripts/release/build.sh v0.1.0 linux amd64
#
# The SQL parser (libpg_query) needs cgo, so each platform is built on a
# native machine rather than cross-compiled. Linux binaries are linked
# statically (with the pure-Go resolver and user lookup) so they run on any
# distribution regardless of its C library; macOS binaries link only the
# system libraries.
set -euo pipefail

version=${1:?usage: build.sh <version> <goos> <goarch>}
goos=${2:?usage: build.sh <version> <goos> <goarch>}
goarch=${3:?usage: build.sh <version> <goos> <goarch>}

host_os=$(go env GOOS)
host_arch=$(go env GOARCH)
if [[ "$goos/$goarch" != "$host_os/$host_arch" ]]; then
  echo "build.sh: building $goos/$goarch needs a $goos/$goarch machine (this is $host_os/$host_arch)" >&2
  exit 1
fi

module=github.com/BhuwanJoshi-01/prodshadow
commit=$(git rev-parse HEAD 2>/dev/null || echo "")
date=$(TZ=UTC git log -1 --format=%cd --date=format-local:%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u +%Y-%m-%dT%H:%M:%SZ)
ldflags="-s -w -X $module/internal/version.Version=$version -X $module/internal/version.Commit=$commit -X $module/internal/version.Date=$date"
tags=""
if [[ "$goos" == linux ]]; then
  tags="netgo,osusergo"
  ldflags="$ldflags -linkmode external -extldflags -static"
fi

name="prodshadow_${version#v}_${goos}_${goarch}"
rm -rf "dist/$name" "dist/$name.tar.gz"
mkdir -p "dist/$name"
CGO_ENABLED=1 TZ=UTC go build -trimpath -tags "$tags" -ldflags "$ldflags" \
  -o "dist/$name/prodshadow" ./cmd/prodshadow
cp LICENSE NOTICE README.md "dist/$name/"
go version -m "dist/$name/prodshadow" > "dist/$name/BUILDINFO.txt"

# Reproducible archive: fixed owner, order and timestamps.
tar_flags=(--owner=0 --group=0 --numeric-owner --sort=name --mtime="$date")
if ! tar --version 2>/dev/null | grep -q GNU; then
  tar_flags=(--uid 0 --gid 0)
fi
tar -C dist "${tar_flags[@]}" -czf "dist/$name.tar.gz" "$name"
"dist/$name/prodshadow" version
echo "dist/$name.tar.gz"
