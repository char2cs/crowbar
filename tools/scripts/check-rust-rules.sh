#!/usr/bin/env bash
# Enforces the structure rules of docs/rust-style.md that rustfmt and clippy cannot.
# Usage: check-rust-rules.sh [--full] [ROOT...]   |   check-rust-rules.sh --self-test
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
    if [[ $base == tests.rs ]]; then
      fail "$rel:1: [test-orphan] tests.rs: name it <source>_tests.rs beside its source file"
      continue
    fi
    if [[ $base == *_tests.rs ]]; then
      [[ -f "$(dirname "$path")/${stem%_tests}.rs" ]] || fail "$rel:1: [test-orphan] test file without ${stem%_tests}.rs"
      check_file "$rel" "$path" test "$stem" 0 1
      continue
    fi
    scan_source "$rel" "$path" "$base" "$stem" "$root"
  done < <(cd "$root" && find . -name '*.rs' -not -path '*/target/*' -not -path '*/vendor/*' | LC_ALL=C sort)
}

check_file() { # rel path mode stem allow_unwrap allow_types
  awk -v file="$1" -v mode="$3" -v stem="$4" -v allow_unwrap="$5" -v allow_types="$6" -f "$awk_file" "$2" || status=1
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
  depth=$(echo "$rel" | sed -E 's#.*/tests/##' | tr -cd '/' | wc -c)
  if ((depth > 0)); then fail "$rel:1: [test-orphan] only tests/<name>.rs, tests/support/ and tests/fixtures/ are allowed"; return; fi
  local cratename
  cratename=$(basename "$crate")
  if [[ $stem != "${cratename//-/_}" && ! -f "$root$crate/src/$stem.rs" && ! -f "$root/${crate#/}/src/$stem.rs" ]]; then
    fail "$rel:1: [test-orphan] tests/$stem.rs has no src/$stem.rs and is not the crate entry ($cratename)"
  fi
  check_file "$rel" "$path" test "$stem" 0 1
}

scan_source() {
  local rel=$1 path=$2 base=$3 stem=$4 root=$5 lines allow_unwrap=0 allow_types=0
  [[ $base == mod.rs ]] && fail "$rel:1: [mod-rs] mod.rs is banned; use foo.rs next to foo/"
  lines=$(wc -l <"$path")
  ((lines > max_lines)) && fail "$rel:1: [size] $lines lines (max $max_lines); split by concept"
  [[ $base == main.rs || $rel == */src/bin/* ]] && allow_unwrap=1
  [[ $base == lib.rs || $base == main.rs || $base == build.rs || $rel == */src/bin/* ]] && allow_types=1
  if [[ $base == lib.rs ]]; then
    grep -nE '^[[:space:]]*(pub(\([^)]*\))? )?(async |const |unsafe )*(fn|impl|struct|enum|trait|union)[[:space:]<]' "$path" | while IFS=: read -r n _; do
      echo "$rel:$n: [lib-decl] lib.rs holds only mod/pub use/crate docs"
    done | grep . && status=1
  fi
  check_file "$rel" "$path" src "$stem" "$allow_unwrap" "$allow_types"
  case $base in lib.rs|main.rs|build.rs) return ;; esac
  [[ $rel == */src/bin/* ]] && return
  if grep -qE '^[[:space:]]*(pub(\([^)]*\))? )?(async |const |unsafe |extern "C" )*fn[[:space:]]' "$path"; then
    local tests="$(dirname "$path")/${stem}_tests.rs"
    if [[ ! -f $tests ]]; then
      fail "$rel:1: [test-missing] needs ${stem}_tests.rs beside it"
    elif ! grep -qE "^#\[path = \"${stem}_tests\.rs\"\]" "$path"; then
      fail "$rel:1: [test-unwired] add #[cfg(test)] #[path = \"${stem}_tests.rs\"] mod tests;"
    fi
  fi
}

scan_cargo() {
  local root=$1 toml rel dir
  while IFS= read -r toml; do
    rel=${toml#"$root"/}
    dir=$(dirname "$rel")
    grep -qE '^\[lints' "$toml" || [[ $dir == . ]] || fail "$rel:1: [cargo-lints] missing [lints] workspace = true"
    case "/$dir/" in
      */crates/*|*/app/*|*/ios-plugin/*)
        if grep -qE '^[[:space:]]*anyhow[[:space:]]*(=|\.)' "$toml" && [[ $dir != *desktop/app* && $dir != app ]]; then
          fail "$rel:1: [cargo-anyhow] anyhow is for binary crates only; libraries use thiserror"
        fi ;;
    esac
    if [[ $dir == crates/* || $dir == app/crates/* || $dir == */shared/* ]]; then
      grep -nE '^[[:space:]]*(gpui[a-z_-]*|tauri[a-z_-]*|wry|gtk[a-z0-9_-]*)[[:space:]]*(=|\.)|desktop' "$toml" | while IFS=: read -r n _; do
        echo "$rel:$n: [cargo-layer] shared crates never depend on gpui/tauri/wry/gtk or desktop/"
      done | grep . && status=1
    fi
  done < <(find "$root" -name Cargo.toml -not -path '*/target/*' -not -path '*/vendor/*' -not -path '*/node_modules/*' -not -path "$root/Cargo.toml" | LC_ALL=C sort)
}

run_dir() { local r=${1%/}; scan_root "$r"; scan_cargo "$r"; }

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
  seq 1 501 | sed 's/.*/\/\/\/ x/' >"$tmp/src/big.rs"
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
roots=()
for a in "$@"; do
  case $a in
    --self-test) self_test ;;
    --full) do_full=1 ;;
    *) roots+=("$a") ;;
  esac
done
((${#roots[@]})) || roots=("$repo/app/crates" "$repo/app/desktop" "$repo/app/mobile/ios-plugin")
for r in "${roots[@]}"; do [[ -d $r ]] && run_dir "$(cd "$r" && pwd)"; done
((do_full)) && full
((status)) && echo "check-rust-rules: violations found (see docs/rust-style.md)"
exit $status
