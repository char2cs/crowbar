#!/usr/bin/env bash
# Enforces the structure rules of docs/rust-style.md that rustfmt and clippy cannot.
# Usage: check-rust-rules.sh [--full] [--mobile] [ROOT...]   (--mobile adds app/mobile/ios-plugin, reported separately)   |   check-rust-rules.sh --self-test
#        check-rust-rules.sh --rekey OLD NEW [ROOT...]   (after a pure file move: re-keys the baseline entries of OLD to NEW, never growing them)
set -uo pipefail
here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)
awk_file="$here/rust-rules.awk"
max_lines=500
status=0

fail() { echo "$1"; status=1; }

scan_root() {
  local root=$1 rel path base stem dir crate
  while IFS= read -r rel; do
    rel=${rel#./}
    path="$root/$rel"
    base=$(basename "$rel")
    stem=${base%.rs}
    crate=$(echo "/$rel" | sed -E 's#^(.*)/(src|tests)/.*#\1#; t; s#/[^/]*$##')
    case "/$rel" in */target/*) continue ;; esac
    scan_names "$rel" "$root"
    if [[ $base == test_support.rs ]] && declared_for_tests "$path"; then check_file "$rel" "$path" test "$stem" 0 1; continue; fi
    case "/$rel" in
      */tests/support/*|*/tests/fixtures/*|*/test_support/*) check_file "$rel" "$path" test "$stem" 0 1; continue ;;
      */tests/*) scan_integration "$rel" "$path" "$stem" "$root" "$crate"; continue ;;
    esac
    if [[ $base == tests.rs || $base == *_tests.rs ]]; then
      fail "$rel:1: [test-sibling] sibling test files are banned: move the tests into an inline #[cfg(test)] mod tests {} in the source file"
      continue
    fi
    scan_source "$rel" "$path" "$base" "$stem" "$root"
  done < <(cd "$root" && find . -name '*.rs' -not -path '*/target/*' | LC_ALL=C sort)
}

# A lone src/.../test_support.rs is test code like a test_support/ folder when it compiles only under
# cfg(test): an inner #![cfg(test)], or #[cfg(test)] on its `mod test_support;` in the parent module.
declared_for_tests() {
  local file=$1 dir parent
  grep -qE '^#!\[cfg\(test\)\]' "$file" && return 0
  dir=$(dirname "$file")
  for parent in "$dir.rs" "$dir/lib.rs" "$dir/main.rs"; do
    [[ -f $parent ]] || continue
    awk 'prev ~ /^[ \t]*#\[cfg\(test\)\][ \t]*$/ && /^[ \t]*(pub(\([a-z]+\))? )?mod test_support;/ { found = 1 } { prev = $0 } END { exit !found }' "$parent" && return 0
  done
  return 1
}

# unsafe lives only in the OS layers of platform; the iOS plugin's C ABI file is the one exception (unsafe-home).
unsafe_home() {
  case "$1" in
    */app/desktop/crates/platform/src/linux/*|*/app/desktop/crates/platform/src/macos/*|*/app/mobile/ios-plugin/src/ffi.rs) echo 1 ;;
    *) echo 0 ;;
  esac
}

check_file() { # rel path mode stem allow_unwrap allow_types
  local ri_file=0
  [[ "/$1" == */real_infra/* ]] && ri_file=1
  awk -v unsafe_home="$(unsafe_home "$2")" -v ri_file="$ri_file" -v file="$1" -v repo="$repo" -v mode="$3" -v stem="$4" -v allow_unwrap="$5" -v allow_types="$6" -f "$here/tag-comments.awk" -f "$awk_file" "$2" || status=1
  awk -v file="$1" -v lang=rust -v mode="$3" -f "$here/brace-rules.awk" "$2" || status=1
}

scan_names() {
  local rel=$1 root=$2 comp
  while IFS= read -r comp; do
    comp=${comp%.rs}
    case "$comp" in
      util|utils|common|helper|helpers|base|misc|*_util|*_utils|*_helpers|*_helper) fail "$rel:1: [module-name] '$comp' is a banned module name (util/common/helpers/base/misc)" ;;
    esac
  done < <(echo "$rel" | tr '/' '\n')
}

scan_integration() {
  local rel=$1 path=$2 stem=$3 root=$4 crate=$5
  local depth
  depth=$(echo "$rel" | sed -E 's#^(.*/)?tests/##' | tr -cd '/' | wc -c)
  if ((depth > 0)); then fail "$rel:1: [test-orphan] only tests/<name>.rs, tests/support/ and tests/fixtures/ are allowed"; return; fi
  local cratename
  cratename=$(basename "${crate:-$root}")
  if [[ $stem != "${cratename//-/_}" && ! -f "$root$crate/src/$stem.rs" && ! -f "$root/${crate#/}/src/$stem.rs" ]]; then
    fail "$rel:1: [test-orphan] tests/$stem.rs has no src/$stem.rs and is not the crate entry ($cratename)"
  fi
  check_file "$rel" "$path" test "$stem" 0 1
}

scan_source() {
  local rel=$1 path=$2 base=$3 stem=$4 root=$5 allow_unwrap=0 allow_types=0 entry=0 exempt=0
  [[ $base == mod.rs ]] && fail "$rel:1: [mod-rs] mod.rs is banned; use foo.rs next to foo/"
  [[ $base == main.rs || "/$rel" == */src/bin/* ]] && allow_unwrap=1
  # SPEC: a bench is a harness=false binary measured by criterion; it has no unit-test module.
  [[ "/$rel" == */benches/* ]] && entry=1
  [[ $base == lib.rs || $base == main.rs || $base == build.rs || "/$rel" == */src/bin/* ]] && allow_types=1 && entry=1
  if [[ $base == lib.rs ]]; then
    grep -nE '^[[:space:]]*(pub(\([^)]*\))? )?(async |const |unsafe )*(fn|impl|struct|enum|trait|union)[[:space:]<]' "$path" | while IFS=: read -r n _; do
      echo "$rel:$n: [lib-decl] lib.rs holds only mod/pub use/crate docs"
    done | grep . && status=1
  fi
  grep -qE "^$(printf '%s' "$rel" | sed 's/[.[\*^$/]/\\&/g')[[:space:]]+#" "$here/rust-test-exemptions.txt" 2>/dev/null && exempt=1
  awk -v unsafe_home="$(unsafe_home "$path")" -v file="$rel" -v repo="$repo" -v mode=src -v stem="$stem" -v allow_unwrap="$allow_unwrap" -v allow_types="$allow_types" -v exempt="$exempt" -v is_entry="$entry" -f "$here/tag-comments.awk" -f "$awk_file" "$path" || status=1
  awk -v file="$rel" -v lang=rust -v mode=src -f "$here/brace-rules.awk" "$path" || status=1
}

exports_entry_function() {
  sed -nE 's/^pub use [a-z_]+::(.*);$/\1/p' "$1" | tr -d '{} ' | tr ',' '\n' | grep -qE '^[a-z_]+$'
}

scan_cargo() {
  local root=$1 toml rel dir
  while IFS= read -r toml; do
    rel=${toml#"$root"/}
    dir=$(dirname "$rel")
    [[ -f $root/$dir/src/lib.rs && ! -f $root/$dir/src/$(basename "$dir" | tr - _).rs ]] && exports_entry_function "$root/$dir/src/lib.rs" && fail "$rel:1: [entry-file] crate entry lives in src/$(basename "$dir" | tr - _).rs (manifest 10)"
    grep -qE '^\[lints' "$toml" || [[ $dir == . ]] || fail "$rel:1: [cargo-lints] missing [lints] workspace = true"
    case "/$dir/" in
      */crates/*|*/app/*|*/ios-plugin/*)
        if grep -qE '^[[:space:]]*anyhow[[:space:]]*(=|\.)' "$toml" && [[ $dir != *desktop/app* && $dir != app ]]; then
          fail "$rel:1: [cargo-anyhow] anyhow is for binary crates only; libraries use thiserror"
        fi ;;
    esac
    if [[ $(basename "$root") != desktop && ( $dir == crates/* || $dir == app/crates/* || $dir == */shared/* ) ]]; then
      grep -nE '^[[:space:]]*(gpui[a-z_-]*|tauri[a-z_-]*|wry|gtk[a-z0-9_-]*)[[:space:]]*(=|\.)|desktop' "$toml" | while IFS=: read -r n _; do
        echo "$rel:$n: [cargo-layer] shared crates never depend on gpui/tauri/wry/gtk or desktop/"
      done | grep . && status=1
    fi
  done < <(find "$root" -name Cargo.toml -not -path '*/target/*' -not -path '*/node_modules/*' -not -path "$root/Cargo.toml" | LC_ALL=C sort)
}

scan_layers() {
  local root=$1 layers="$here/rust-layers.txt"
  # A fixture root carries its own layer table.
  [[ -f $root/rust-layers.txt ]] && layers=$root/rust-layers.txt
  python3 -I "$here/check-layers.py" "$layers" "$root" --complete || status=1
}

scan_cycles() {
  local root=$1 srcs
  mapfile -t srcs < <(find "$root" -type d -name src -not -path '*/target/*' -not -path '*/test_support/*' | LC_ALL=C sort)
  ((${#srcs[@]})) || return 0
  python3 -I "$here/check-cycles.py" rust "${srcs[@]}" | sed "s#^$root/##" ; [[ ${PIPESTATUS[0]} -eq 0 ]] || status=1
}

scan_module_count() { # a folder holds at most 12 modules; foo.rs + foo/ count once (rust-tree plan 2.1)
  local root=$1
  # The binary crate (app/desktop/app) is regrouped by its own lane and joins this check when that lands.
  python3 -I - "$root" <<'PY'
import os, sys
root = sys.argv[1]
for d, subs, files in os.walk(root):
    subs[:] = sorted(s for s in subs if s not in ("target", "node_modules"))
    rel = os.path.relpath(d, root)
    if "src" not in rel.split(os.sep) or rel == "app" or rel.startswith("app" + os.sep): continue
    mods = {f[:-3] for f in files if f.endswith(".rs")}
    mods |= {s for s in subs if any(n.endswith(".rs") for _, _, ns in os.walk(os.path.join(d, s)) for n in ns)}
    if len(mods) > 12:
        print(f"{rel}:1: [module-count] {len(mods)} modules in one folder; at most 12, cut by responsibility")
PY
}

run_dir() { local r=${1%/}; scan_root "$r"; [[ $r == */mobile/src-tauri ]] || scan_module_count "$r"; scan_cargo "$r"; scan_layers "$r"; scan_cycles "$r"; }

self_test() {
  local td="$here/testdata/rust-rules" rc=0 out case_dir tag tmp
  out=$(run_dir "$td/good" 2>&1); status=0
  if [[ -n $out ]]; then echo "FAIL good fixture reported:"; echo "$out"; rc=1; else echo "ok   good: clean"; fi
  for case_dir in "$td"/bad/*/; do
    tag=$(basename "$case_dir"); tag=${tag%%--*}
    status=0; out=$(run_dir "$case_dir" 2>&1)
    if grep -q "\[$tag\]" <<<"$out"; then echo "ok   bad/$tag"; else echo "FAIL bad/$tag not detected"; echo "$out"; rc=1; fi
  done
  tmp=$(mktemp -d); mkdir -p "$tmp/src"
  seq 1 501 | sed 's/.*/const X: u32 = 1;/' >"$tmp/src/big.rs"
  status=0; out=$(run_dir "$tmp" 2>&1); rm -rf "$tmp"
  if grep -q '\[size\]' <<<"$out"; then echo "ok   size (501 lines)"; else echo "FAIL size not detected"; rc=1; fi
  python3 -I "$here/check-ratchet.py" --self-test || rc=1
  exit $rc
}

full() {
  command -v cargo >/dev/null || { fail "cargo not installed"; return; }
  (cd "$repo/app" && cargo fmt --all --check) || status=1
  (cd "$repo/app" && cargo clippy --workspace --all-targets -- -D warnings) || status=1
  for tool in deny machete; do
    if cargo "$tool" --version >/dev/null 2>&1; then
      case $tool in deny) (cd "$repo/app" && cargo deny check) || status=1 ;; machete) (cd "$repo/app" && cargo machete) || status=1 ;; esac
    else
      fail "cargo-$tool not installed (cargo install cargo-$tool)"
    fi
  done
}

do_full=0
mobile=0
ratchet_flags=()
roots=()
rekey=()
while (($#)); do
  a=$1; shift
  case $a in
    --rekey) rekey=(--rekey "$1" "$2"); shift 2 ;;
    --self-test) self_test ;;
    --full) do_full=1 ;;
    --mobile) mobile=1 ;;
    --strict|--shrink|--seed) ratchet_flags+=("$a") ;;
    *) roots+=("$a") ;;
  esac
done
((${#roots[@]})) || roots=("$repo/app/crates" "$repo/app/desktop" "$repo/app/mobile/src-tauri")
((mobile)) && roots+=("$repo/app/mobile/ios-plugin")
findings=$(mktemp)
trap 'rm -f "$findings"' EXIT
scopes=()
for r in "${roots[@]}"; do
  [[ -d $r ]] || continue
  abs=$(cd "$r" && pwd); rel=${abs#"$repo"/}
  scopes+=(--scope "$rel/")
  run_dir "$abs" | sed -E "s#^([^ :]+:[0-9]+: \\[)#$rel/\\1#" >>"$findings"
done
status=0
((do_full)) && full
if ((${#rekey[@]})); then
  python3 -I "$here/check-ratchet.py" --lang rust "${rekey[@]}" <"$findings" || status=1
else
  python3 -I "$here/check-ratchet.py" --lang rust ${scopes[@]+"${scopes[@]}"} ${ratchet_flags[@]+"${ratchet_flags[@]}"} <"$findings" || status=1
fi
((status)) && echo "check-rust-rules: violations found (see docs/rust-style.md)"
exit $status
