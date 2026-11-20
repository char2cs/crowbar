#!/usr/bin/env bash
# Re-vendors the crates pinned in upstream.txt: the published .crate (sha256 checked), minus the
# files patches/<dir>/trim names, plus the patches patches/<dir>/series lists, in order.
# Usage: update.sh <dir>...   replace those vendored directories
#        update.sh --check    rebuild every pinned directory in a scratch tree and diff it
set -euo pipefail

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
registry=${VENDOR_REGISTRY:-https://static.crates.io/crates}
# Packaging files cargo adds to a release; a vendored copy never keeps them.
packaging=(.cargo_vcs_info.json .cargo-ok Cargo.lock Cargo.toml.orig)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

pin() {
  local dir=$1
  awk -v dir="$dir" '$1 == dir { print $2, $3, $4; found = 1 } END { exit !found }' "$here/upstream.txt" ||
    { echo "update.sh: $dir is not pinned in upstream.txt" >&2; return 1; }
}

pinned_dirs() {
  awk '!/^#/ && NF { print $1 }' "$here/upstream.txt"
}

# build <dir> <out>: writes upstream plus the series of <dir> to <out>.
build() {
  local dir=$1 out=$2 name version sha archive
  read -r name version sha < <(pin "$dir")
  archive="$work/$dir.crate"
  curl -sSfL -A vendor-update "$registry/$name/$name-$version.crate" -o "$archive"
  echo "$sha  $archive" | sha256sum --check --quiet -
  mkdir -p "$out"
  tar xzf "$archive" -C "$out" --strip-components=1
  local trimmed=("${packaging[@]}")
  [[ -f $here/patches/$dir/trim ]] && mapfile -t -O "${#trimmed[@]}" trimmed <"$here/patches/$dir/trim"
  local path
  for path in "${trimmed[@]}"; do
    [[ $path == *..* || $path == /* ]] && { echo "update.sh: bad trim path $path" >&2; return 1; }
    rm -rf "${out:?}/$path"
  done
  [[ -f $here/patches/$dir/series ]] || return 0
  local patch
  while read -r patch; do
    [[ -z $patch || $patch == \#* ]] && continue
    patch -d "$out" -p1 -s --fuzz=0 --no-backup-if-mismatch -i "$here/patches/$dir/$patch" ||
      { echo "update.sh: $dir: $patch does not apply" >&2; return 1; }
  done <"$here/patches/$dir/series"
}

check() {
  local dir status=0
  while read -r dir; do
    build "$dir" "$work/tree/$dir"
    if ! diff -r "$work/tree/$dir" "$here/$dir" >"$work/$dir.diff"; then
      echo "update.sh: $dir differs from upstream plus its series:" >&2
      head -n 40 "$work/$dir.diff" >&2
      status=1
    fi
  done < <(pinned_dirs)
  ((status == 0)) && echo "update.sh: every vendored crate is upstream plus its series"
  return "$status"
}

(($# > 0)) || { echo "usage: update.sh <dir>... | --check" >&2; exit 2; }
if [[ $1 == --check ]]; then
  check
  exit
fi
for dir in "$@"; do
  build "$dir" "$work/tree/$dir"
  rm -rf "${here:?}/$dir"
  mv "$work/tree/$dir" "$here/$dir"
done
