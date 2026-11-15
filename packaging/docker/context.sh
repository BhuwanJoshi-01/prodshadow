#!/usr/bin/env bash
# Lays out a docker build context from release archives:
#
#   packaging/docker/context.sh dist build/docker
#
# extracts dist/prodshadow_*_linux_<arch>.tar.gz to
# build/docker/bin/linux_<arch>/prodshadow and copies the Dockerfile.
set -euo pipefail

dist=${1:?usage: context.sh <dist-dir> <context-dir>}
out=${2:?usage: context.sh <dist-dir> <context-dir>}
here=$(cd "$(dirname "$0")" && pwd)

mkdir -p "$out/bin"
cp "$here/Dockerfile" "$out/Dockerfile"
found=0
for archive in "$dist"/prodshadow_*_linux_*.tar.gz; do
  [[ -e "$archive" ]] || continue
  arch=${archive%.tar.gz}
  arch=${arch##*_}
  tmp=$(mktemp -d)
  tar -xzf "$archive" -C "$tmp"
  mkdir -p "$out/bin/linux_$arch"
  cp "$tmp"/*/prodshadow "$out/bin/linux_$arch/prodshadow"
  echo "$out/bin/linux_$arch/prodshadow"
  found=$((found + 1))
done
if [[ $found -eq 0 ]]; then
  echo "context.sh: no Linux archives in $dist" >&2
  exit 1
fi
