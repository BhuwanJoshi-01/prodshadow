#!/usr/bin/env bash
# Prints the CHANGELOG.md section of one version, for the release notes.
#
#   scripts/release/notes.sh v0.1.0
set -euo pipefail

version=${1:?usage: notes.sh <version>}
version=${version#v}
changelog=${CHANGELOG:-CHANGELOG.md}

section=$(awk -v v="$version" '
  /^## \[/ { if (found) exit; if (index($0, "## [" v "]") == 1) { found = 1; next } }
  found && /^\[[^]]+\]: / { exit }
  found { print }
' "$changelog")

if [[ -z "${section//[[:space:]]/}" ]]; then
  echo "notes.sh: CHANGELOG has no section for $version" >&2
  exit 1
fi
# Relative links point into the repository at the release tag, because the
# notes are shown on the release page, not next to the files.
printf '%s\n' "$section" | sed -e '/./,$!d' |
  V="$version" perl -pe 's{\]\((?!https?://|#|mailto:)(?:\./)?([^)\s]+)\)}{](https://github.com/BhuwanJoshi-01/prodshadow/blob/v$ENV{V}/$1)}g'
