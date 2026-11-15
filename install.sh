#!/bin/sh
# Installs the prodshadow binary from a GitHub release.
#
#   curl -fsSL https://raw.githubusercontent.com/BhuwanJoshi-01/prodshadow/main/install.sh | sh
#
# Settings, as environment variables:
#   PRODSHADOW_VERSION      release to install, e.g. v0.1.0 (default: the latest)
#   PRODSHADOW_INSTALL_DIR  directory for the binary (default: /usr/local/bin if
#                           writable, otherwise ~/.local/bin)
#
# The archive is checked against the release's SHA256SUMS before anything is
# installed. Linux (x86_64, arm64) and macOS (arm64, x86_64) are supported.
set -eu

repo="BhuwanJoshi-01/prodshadow"
releases="${PRODSHADOW_DOWNLOAD_URL:-https://github.com/$repo/releases}"

say() { printf 'prodshadow-install: %s\n' "$*" >&2; }
fail() { say "$*"; exit 1; }
have() { command -v "$1" > /dev/null 2>&1; }

fetch() { # url [output]
  if have curl; then
    if [ $# -eq 2 ]; then curl -fsSL -o "$2" "$1"; else curl -fsSL "$1"; fi
  elif have wget; then
    if [ $# -eq 2 ]; then wget -q -O "$2" "$1"; else wget -q -O - "$1"; fi
  else
    fail "curl or wget is required"
  fi
}

latest_version() {
  if have curl; then
    curl -fsSLI -o /dev/null -w '%{url_effective}' "$releases/latest" |
      sed -n 's#.*/tag/\(v[^/]*\)/*$#\1#p'
  else
    fetch "https://api.github.com/repos/$repo/releases/latest" |
      sed -n 's/.*"tag_name": *"\(v[^"]*\)".*/\1/p' | head -n 1
  fi
}

sha256() {
  if have sha256sum; then
    sha256sum "$1" | cut -d ' ' -f 1
  elif have shasum; then
    shasum -a 256 "$1" | cut -d ' ' -f 1
  else
    fail "sha256sum or shasum is required to check the download"
  fi
}

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) fail "there is no prebuilt binary for $(uname -s). On Windows, run this inside WSL; elsewhere: go install github.com/$repo/cmd/prodshadow@latest" ;;
esac
case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) fail "there is no prebuilt binary for $(uname -m); build it with: go install github.com/$repo/cmd/prodshadow@latest" ;;
esac

version="${PRODSHADOW_VERSION:-}"
if [ -z "$version" ]; then
  version=$(latest_version) || true
  [ -n "$version" ] || fail "could not find the latest release; set PRODSHADOW_VERSION=vX.Y.Z"
fi
case "$version" in v*) ;; *) version="v$version" ;; esac

archive="prodshadow_${version#v}_${os}_${arch}.tar.gz"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

say "downloading $archive ($version)"
fetch "$releases/download/$version/$archive" "$tmp/$archive" ||
  fail "could not download $releases/download/$version/$archive"
fetch "$releases/download/$version/SHA256SUMS" "$tmp/SHA256SUMS" ||
  fail "could not download the release's SHA256SUMS"

want=$(awk -v f="$archive" '$2 == f || $2 == "*" f { print $1 }' "$tmp/SHA256SUMS")
got=$(sha256 "$tmp/$archive")
[ -n "$want" ] || fail "$archive is not listed in SHA256SUMS"
[ "$want" = "$got" ] || fail "checksum mismatch for $archive: expected $want, got $got"

tar -xzf "$tmp/$archive" -C "$tmp"
binary="$tmp/${archive%.tar.gz}/prodshadow"
[ -f "$binary" ] || fail "$archive does not contain the prodshadow binary"

dir="${PRODSHADOW_INSTALL_DIR:-}"
if [ -z "$dir" ]; then
  if [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
    dir=/usr/local/bin
  else
    dir="$HOME/.local/bin"
  fi
fi
mkdir -p "$dir" 2> /dev/null || fail "cannot create $dir; set PRODSHADOW_INSTALL_DIR"
[ -w "$dir" ] || fail "cannot write to $dir; set PRODSHADOW_INSTALL_DIR or rerun with sudo"

cp "$binary" "$dir/prodshadow.tmp"
chmod 0755 "$dir/prodshadow.tmp"
mv -f "$dir/prodshadow.tmp" "$dir/prodshadow"
say "installed $dir/prodshadow"
"$dir/prodshadow" version

case ":$PATH:" in
  *":$dir:"*) ;;
  *) say "$dir is not on your PATH; add it, for example: export PATH=\"$dir:\$PATH\"" ;;
esac
