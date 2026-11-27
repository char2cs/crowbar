#!/usr/bin/env bash
# Fails when installing from ARROW.md would ask a release or the repository for a file it lacks.
# Usage: check-arrow.sh <release-tag> <assets-dir>
set -euo pipefail
ref=$1 assets=$2
repo=$(cd "$(dirname "$0")/../.." && pwd)
status=0

fail() { echo "check-arrow: $1" >&2; status=1; }

wanted() {
  local found
  found=$(grep -oE "$1" "$repo/ARROW.md" | sed 's|.*\${REF}/||' | sort -u)
  [[ -n $found ]] || { echo "check-arrow: ARROW.md no longer matches $1" >&2; exit 1; }
  echo "$found"
}

downloads=$(wanted 'releases/download/\$\{REF\}/[^"]+')
scripts=$(wanted 'raw\.githubusercontent\.com/char2cs/crowbar/\$\{REF\}/[^ "|]+')
# service-install.sh downloads the daemon for the machine it runs on.
daemons=$(printf 'crowbar-api-%s\n' darwin-amd64 darwin-arm64 linux-amd64 linux-arm64)

for name in $downloads $daemons; do
  name=${name//'${REF}'/$ref}
  [[ -n $(find "$assets" -type f -name "$name") ]] || fail "the release has no $name"
done
for path in $scripts; do
  [[ -f $repo/$path ]] || fail "the repository has no $path"
done
exit $status
