#!/usr/bin/env bash
# Enforces docs/ts-style.md on TypeScript files changed on this branch (vs develop, plus working tree).
# Usage: check-ts-rules.sh [--full] [--all] [ROOT]   |   check-ts-rules.sh --self-test
# ROOT is the web package (default app/panes). --all scans every file instead of the changed ones.
set -uo pipefail
here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)
awk_file="$here/ts-rules.awk"
exempt_file="$here/ts-test-exemptions.txt"
max_lines=500
status=0

fail() { echo "$1"; status=1; }

kebab() { [[ $1 =~ ^[a-z0-9]+(-[a-z0-9]+)*(\.[a-z0-9]+(-[a-z0-9]+)*)*$ ]]; }

# Lists files (relative to root) to scan, tagged "A" (new on the branch) or "M" (legacy file edited in the working tree).
# The web/ -> app/panes move shows as added; a file whose web/ twin exists on develop is legacy and skipped unless edited.
changed_files() {
  local root=$1 all=$2 prefix p rel twin
  prefix=${root#"$repo"/}
  if ((all)); then
    (cd "$root" && find src entries -type f \( -name '*.ts' -o -name '*.tsx' \) -not -path '*/node_modules/*' 2>/dev/null | LC_ALL=C sort | sed 's/^/A /')
    return
  fi
  {
    git -C "$repo" diff --name-only --diff-filter=A develop...HEAD -- "$prefix" | sed 's/^/C /'
    git -C "$repo" diff --name-only --diff-filter=AMR HEAD -- "$prefix" | sed 's/^/W /'
    git -C "$repo" ls-files --others --exclude-standard -- "$prefix" | sed 's/^/W /'
  } | awk '!seen[$2]++' | while read -r src p; do
    [[ -f $repo/$p && $p =~ \.(ts|tsx)$ ]] || continue
    rel=${p#"$prefix"/}
    twin="web/$rel"
    if git -C "$repo" cat-file -e "develop:$twin" 2>/dev/null; then
      [[ $src == W ]] && echo "M $rel"
    else
      echo "A $rel"
    fi
  done | LC_ALL=C sort -k2
}

scan() {
  local root=${1%/} all=$2 tag rel base stem mode lines src
  local layer_files=()
  while read -r tag rel; do
    [[ -z $rel ]] && continue
    case "$rel" in node_modules/*|*.d.ts|*/generated/*|*.config.ts|*.config.js) continue ;; esac
    base=$(basename "$rel"); stem=${base%.*}
    kebab "${base%.*}" || fail "$rel:1: [file-name] '$base' must be kebab-case"
    if [[ $tag == A ]]; then
      for comp in $(echo "$rel" | tr '/' ' '); do
        case "${comp%%.*}" in util|utils|common|helper|helpers|base|misc|*-util|*-utils|*-helper|*-helpers) fail "$rel:1: [module-name] '$comp' is a banned name (util/common/helpers/base/misc)" ;; esac
      done
    fi
    if [[ $rel == src/__tests__/* || $base == *.test.ts || $base == *.test.tsx ]]; then
      mode=test
      [[ $rel == src/__tests__/* ]] || fail "$rel:1: [test-location] tests live in src/__tests__/ mirroring src/"
      [[ $rel == */__fixtures__/* || $rel == */__mocks__/* || $rel == */test-support/* || $rel == src/__tests__/setup* ]] && mode=support
      if [[ $mode == test ]]; then
        if [[ $base =~ -(coverage|extra|gaps)\.test\. ]]; then
          fail "$rel:1: [test-scenario] extend the unit's test file instead of <unit>-<scenario>.test.ts"
        else
          local want=${rel#src/__tests__/}; want=${want%.test.ts*}
          [[ -f $root/src/$want.ts || -f $root/src/$want.tsx || -f $root/src/$want/index.ts || -f $root/src/$want/index.tsx || -f $root/$want.ts ]] \
            || fail "$rel:1: [test-orphan] no src/$want.ts(x) for this test"
        fi
      fi
      [[ $mode == support ]] && continue
    else
      mode=src
    fi
    lines=$(grep -cvE '^[[:space:]]*(//.*|\*.*|/\*.*)?$' "$root/$rel")
    ((lines > max_lines)) && [[ $mode == src ]] && fail "$rel:1: [size] $lines lines (max $max_lines); split by concept"
    awk -v file="$rel" -v repo="$repo" -v mode="$mode" -v stem="$stem" -f "$here/tag-comments.awk" -f "$awk_file" "$root/$rel" || status=1
    awk -v file="$rel" -v lang=ts -v mode="$mode" -f "$here/brace-rules.awk" "$root/$rel" || status=1
    [[ $mode == src ]] && layer_files+=("$rel")
    if [[ $mode == src && $tag == A && $rel == src/* && $base != index.ts && $base != index.tsx ]]; then
      if grep -qE '(function|=>)' "$root/$rel" && ! grep -qE "^${rel//./\\.}[[:space:]]+#" "$exempt_file" 2>/dev/null; then
        local t=${rel#src/}; t=${t%.ts*}
        [[ -f $root/src/__tests__/$t.test.ts || -f $root/src/__tests__/$t.test.tsx ]] || fail "$rel:1: [test-missing] needs src/__tests__/$t.test.ts(x)"
      fi
    fi
    if [[ $base == index.ts || $base == index.tsx ]] && [[ $tag == A ]] && ! grep -qE '(function|class|=>|interface|type) ' "$root/$rel"; then
      fail "$rel:1: [barrel] index file that only re-exports: import from the defining module"
    fi
  done < <(changed_files "$root" "$all")
  ((${#layer_files[@]})) && { python3 -I "$here/check-cycles.py" layer "$root" "${layer_files[@]}" || status=1; }
  local changed
  mapfile -t changed < <(changed_files "$root" "$all" | awk '{print $2}')
  ((${#changed[@]})) && { python3 -I "$here/check-cycles.py" ts "$root" "${changed[@]}" || status=1; }
}

self_test() {
  local td="$here/testdata/ts-rules" rc=0 out d tag tmp
  status=0; out=$(scan "$td/good" 1 2>&1)
  if [[ -n $out ]]; then echo "FAIL good fixture reported:"; echo "$out"; rc=1; else echo "ok   good: clean"; fi
  for d in "$td"/bad/*/; do
    tag=$(basename "$d"); tag=${tag%%--*}; status=0; out=$(scan "$d" 1 2>&1)
    if grep -q "\[$tag\]" <<<"$out"; then echo "ok   bad/$tag"; else echo "FAIL bad/$tag not detected"; echo "$out"; rc=1; fi
  done
  tmp=$(mktemp -d); mkdir -p "$tmp/src"; seq 1 501 | sed 's/.*/export const x = 1/' >"$tmp/src/big.ts"
  status=0; out=$(scan "$tmp" 1 2>&1); rm -rf "$tmp"
  if grep -q '\[size\]' <<<"$out"; then echo "ok   size (501 lines)"; else echo "FAIL size not detected"; rc=1; fi
  python3 -I "$here/check-ratchet.py" --self-test >/dev/null || rc=1
  exit $rc
}

full() {
  local root=$1 files work
  mapfile -t files < <(changed_files "$root" 0 | awk '{print $2}')
  ((${#files[@]})) || return 0
  command -v docker >/dev/null || { fail "docker not available for eslint (--full)"; return; }
  work=/tmp/ts-standards
  docker exec inventory-app sh -c "mkdir -p $work" || { fail "inventory-app container not running"; return; }
  tar -C "$root" --exclude=node_modules -cf - . | docker exec -i inventory-app sh -c "cd $work && tar -xf -" || { fail "copy to container failed"; return; }
  docker exec inventory-app sh -c "cd $work && { [ -d node_modules ] || bun install --frozen-lockfile >/dev/null; } && bunx eslint --max-warnings 0 ${files[*]} && bunx tsc --noEmit && bunx prettier --check ${files[*]}" || status=1
}

do_full=0; all=0; root=""; ratchet_flags=()
for a in "$@"; do
  case $a in --self-test) self_test ;; --full) do_full=1 ;; --all) all=1 ;; --strict|--shrink|--seed) ratchet_flags+=("$a") ;; --mobile) ;; *) root=$a ;; esac
done
[[ -z $root ]] && root="$repo/app/panes"
root=$(cd "$root" && pwd)
rel_root=${root#"$repo"/}
findings=$(mktemp); scanned=$(mktemp)
trap 'rm -f "$findings" "$scanned"' EXIT
changed_files "$root" "$all" | awk -v p="$rel_root/" '{print p $2}' >"$scanned"
scan "$root" "$all" | sed -E "s#^([^ :]+:[0-9]+: \\[)#$rel_root/\\1#" >"$findings"
status=0
((do_full)) && full "$root"
python3 -I "$here/check-ratchet.py" --lang ts --scope "$rel_root/" --scanned "$scanned" ${ratchet_flags[@]+"${ratchet_flags[@]}"} <"$findings" || status=1
((status)) && echo "check-ts-rules: violations found (see docs/ts-style.md)"
exit $status
