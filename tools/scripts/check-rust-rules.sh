#!/usr/bin/env bash
# Enforces the structure rules of docs/rust-style.md that rustfmt and clippy cannot.
# Usage: check-rust-rules.sh [--full] [--mobile] [ROOT...]   (--mobile adds app/mobile/ios-plugin, reported separately)   |   check-rust-rules.sh --self-test
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
    case "/$rel" in */target/*|*/vendor/*) continue ;; esac
    scan_names "$rel" "$root"
    case "/$rel" in
      */tests/support/*|*/tests/fixtures/*|*/test_support/*) check_file "$rel" "$path" test "$stem" 0 1; continue ;;
      */tests/*) scan_integration "$rel" "$path" "$stem" "$root" "$crate"; continue ;;
    esac
    if [[ $base == tests.rs || $base == *_tests.rs ]]; then
      fail "$rel:1: [test-sibling] sibling test files are banned: move the tests into an inline #[cfg(test)] mod tests {} in the source file"
      continue
    fi
    scan_source "$rel" "$path" "$base" "$stem" "$root"
  done < <(cd "$root" && find . -name '*.rs' -not -path '*/target/*' -not -path '*/vendor/*' | LC_ALL=C sort)
}

check_file() { # rel path mode stem allow_unwrap allow_types
  awk -v file="$1" -v mode="$3" -v stem="$4" -v allow_unwrap="$5" -v allow_types="$6" -f "$awk_file" "$2" || status=1
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
  awk -v file="$rel" -v mode=src -v stem="$stem" -v allow_unwrap="$allow_unwrap" -v allow_types="$allow_types" -v exempt="$exempt" -v is_entry="$entry" -f "$awk_file" "$path" || status=1
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
  done < <(find "$root" -name Cargo.toml -not -path '*/target/*' -not -path '*/vendor/*' -not -path '*/node_modules/*' -not -path "$root/Cargo.toml" | LC_ALL=C sort)
}

scan_cycles() {
  local root=$1 srcs
  mapfile -t srcs < <(find "$root" -type d -name src -not -path '*/target/*' -not -path '*/vendor/*' -not -path '*/test_support/*' | LC_ALL=C sort)
  ((${#srcs[@]})) || return 0
  python3 -I "$here/check-cycles.py" rust "${srcs[@]}" | sed "s#^$root/##" ; [[ ${PIPESTATUS[0]} -eq 0 ]] || status=1
}

run_dir() { local r=${1%/}; scan_root "$r"; scan_cargo "$r"; scan_cycles "$r"; }

self_test() {
  local td="$here/testdata/rust-rules" rc=0 out case_dir tag tmp
  out=$(run_dir "$td/good" 2>&1); status=0
  if [[ -n $out ]]; then echo "FAIL good fixture reported:"; echo "$out"; rc=1; else echo "ok   good: clean"; fi
  for case_dir in "$td"/bad/*/; do
    tag=$(basename "$case_dir")
    status=0; out=$(run_dir "$case_dir" 2>&1)
    if grep -q "\[$tag\]" <<<"$out"; then echo "ok   bad/$tag"; else echo "FAIL bad/$tag not detected"; echo "$out"; rc=1; fi
  done
  tmp=$(mktemp -d); mkdir -p "$tmp/src"
  seq 1 501 | sed 's/.*/const X: u32 = 1;/' >"$tmp/src/big.rs"
  status=0; out=$(run_dir "$tmp" 2>&1); rm -rf "$tmp"
  if grep -q '\[size\]' <<<"$out"; then echo "ok   size (501 lines)"; else echo "FAIL size not detected"; rc=1; fi
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
roots=()
for a in "$@"; do
  case $a in
    --self-test) self_test ;;
    --full) do_full=1 ;;
    --mobile) mobile=1 ;;
    *) roots+=("$a") ;;
  esac
done
((${#roots[@]})) || roots=("$repo/app/crates" "$repo/app/desktop")
  ((mobile)) && roots+=("$repo/app/mobile/ios-plugin")
for r in "${roots[@]}"; do [[ -d $r ]] && run_dir "$(cd "$r" && pwd)"; done
((do_full)) && full
((status)) && echo "check-rust-rules: violations found (see docs/rust-style.md)"
exit $status
