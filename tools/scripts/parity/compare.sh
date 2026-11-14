#!/usr/bin/env bash
# Usage: compare.sh <reference-dir> <candidate-dir> <out-dir>
# Compares every PNG of the reference directory (and frames/*/f-*.png) with the candidate's file of the same
# path. Needs ImageMagick `compare` on the host or in PARITY_CONTAINER (used when the host has none).
set -euo pipefail
ref=${1:?}; cand=${2:?}; out=${3:?}; thr=${THRESHOLD:-0.02}
mkdir -p "$out"; printf 'name\trmse\tdiffering_px\n' > "$out/report.tsv"; fail=0
cmp_() { # a b diff -> "rmse px"
  if command -v compare >/dev/null; then
    r=$(compare -metric RMSE "$1" "$2" "$3" 2>&1 | sed 's/.*(\(.*\))/\1/' || true)
    p=$(compare -metric AE "$1" "$2" null: 2>&1 || true)
  else
    c=${PARITY_CONTAINER:?host has no ImageMagick; set PARITY_CONTAINER}
    docker cp "$1" "$c:/tmp/cmp-a.png" >/dev/null; docker cp "$2" "$c:/tmp/cmp-b.png" >/dev/null
    r=$(docker exec "$c" bash -c 'compare -metric RMSE /tmp/cmp-a.png /tmp/cmp-b.png /tmp/cmp-d.png 2>&1 | sed "s/.*(\(.*\))/\1/"' || true)
    p=$(docker exec "$c" bash -c 'compare -metric AE /tmp/cmp-a.png /tmp/cmp-b.png null: 2>&1' || true)
    docker cp "$c:/tmp/cmp-d.png" "$3" >/dev/null 2>&1 || true
  fi
  echo "$r $p"
}
while IFS= read -r f; do
  rel=${f#"$ref"/}; name=${rel//\//__}
  if [ ! -f "$cand/$rel" ]; then printf '%s\tMISSING\t-\n' "$rel" >> "$out/report.tsv"; fail=1; continue; fi
  read -r rmse px <<<"$(cmp_ "$f" "$cand/$rel" "$out/$name")"
  printf '%s\t%s\t%s\n' "$rel" "$rmse" "$px" >> "$out/report.tsv"
  awk -v r="$rmse" -v t="$thr" 'BEGIN{exit !(r+0>t)}' && fail=1
done < <(find "$ref" -name '*.png' | sort)
cat "$out/report.tsv"; exit $fail
