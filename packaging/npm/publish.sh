#!/usr/bin/env bash
# Publishes the packages staged by stage.mjs, platform packages first.
#
#   packaging/npm/publish.sh dist/npm            # publish
#   packaging/npm/publish.sh dist/npm --dry-run  # show what would be published
#
# Versions that are already on the registry are skipped, so a failed run can
# be repeated. Pre-release versions (with a hyphen) are published under the
# "next" dist-tag so "latest" keeps pointing at the last stable release. In
# GitHub Actions the packages get provenance statements.
set -euo pipefail

dir=${1:?usage: publish.sh <staged-dir> [--dry-run]}
shift
extra=("$@")

if [[ "${GITHUB_ACTIONS:-}" == true ]]; then
  extra+=(--provenance)
fi

order=()
for pkg in "$dir"/prodshadow-*/; do
  order+=("${pkg%/}")
done
order+=("$dir/prodshadow")

for pkg in "${order[@]}"; do
  json="$(cd "$pkg" && pwd)/package.json"
  name=$(node -p 'require(process.argv[1]).name' "$json")
  version=$(node -p 'require(process.argv[1]).version' "$json")
  tag=latest
  [[ "$version" == *-* ]] && tag=next
  if npm view "$name@$version" version > /dev/null 2>&1; then
    echo "skip    $name@$version (already published)"
    continue
  fi
  echo "publish $name@$version (dist-tag $tag)"
  (cd "$pkg" && npm publish --access public --tag "$tag" ${extra[@]+"${extra[@]}"})
done
