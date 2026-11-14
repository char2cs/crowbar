#!/usr/bin/env bash
# Enforces the structure rules of docs/swift-style.md that SwiftLint and swift-format cannot.
# Usage: check-swift-rules.sh [--full] [ROOT]   |   check-swift-rules.sh --self-test
# ROOT is the Swift package directory (default app/mobile/ios).
set -uo pipefail
here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)
awk_file="$here/swift-rules.awk"
max_lines=500
status=0

fail() { echo "$1"; status=1; }
check_file() { awk -v file="$1" -v mode="$3" -v stem="$4" -f "$awk_file" "$2" || status=1; }

scan_package() {
  local root=${1%/} rel base stem lines tdir comp
  while IFS= read -r rel; do
    rel=${rel#./}; base=$(basename "$rel"); stem=${base%.swift}
    while IFS= read -r comp; do
      comp=${comp%.swift}
      case "$comp" in Util|Utils|Common|Helper|Helpers|Base|Misc|*Util|*Utils|*Helper|*Helpers) fail "$rel:1: [module-name] '$comp' is a banned name (util/common/helpers/base/misc)" ;; esac
    done < <(echo "$rel" | tr '/' '\n')
    case "$rel" in
      Tests/*/Support/*) check_file "$rel" "$root/$rel" test "$stem"; continue ;;
      Tests/*/*Tests.swift)
        [[ $(echo "$rel" | tr -cd '/' | wc -c) -eq 2 ]] || fail "$rel:1: [test-orphan] tests live directly in Tests/<Target>/ or Tests/<Target>/Support/"
        [[ -n $(find "$root/Sources" -name "${stem%Tests}.swift" 2>/dev/null) ]] || fail "$rel:1: [test-orphan] no Sources/**/${stem%Tests}.swift"
        check_file "$rel" "$root/$rel" test "$stem"; continue ;;
      Tests/*) fail "$rel:1: [test-orphan] test files are named <Source>Tests.swift"; continue ;;
    esac
    lines=$(wc -l <"$root/$rel")
    ((lines > max_lines)) && fail "$rel:1: [size] $lines lines (max $max_lines); split by concept"
    check_file "$rel" "$root/$rel" src "$stem"
    if grep -qE '^[[:space:]]*((public|open|internal|private|fileprivate|static|override|mutating) +)*func[[:space:]]' "$root/$rel" \
      && ! grep -qE "^${stem}[[:space:]]+#" "$here/swift-test-exemptions.txt" 2>/dev/null; then
      [[ -n $(find "$root/Tests" -name "${stem}Tests.swift" 2>/dev/null) ]] || fail "$rel:1: [test-missing] needs Tests/<Target>/${stem}Tests.swift"
    fi
  done < <(cd "$root" && find Sources Tests -name '*.swift' -not -path '*/.build/*' 2>/dev/null | LC_ALL=C sort)
}

self_test() {
  local td="$here/testdata/swift-rules" rc=0 out d tag tmp
  status=0; out=$(scan_package "$td/good" 2>&1)
  if [[ -n $out ]]; then echo "FAIL good fixture reported:"; echo "$out"; rc=1; else echo "ok   good: clean"; fi
  for d in "$td"/bad/*/; do
    tag=$(basename "$d"); status=0; out=$(scan_package "$d" 2>&1)
    if grep -q "\[$tag\]" <<<"$out"; then echo "ok   bad/$tag"; else echo "FAIL bad/$tag not detected"; echo "$out"; rc=1; fi
  done
  tmp=$(mktemp -d); mkdir -p "$tmp/Sources"; seq 1 501 | sed 's/.*/let x = 1/' >"$tmp/Sources/Big.swift"
  status=0; out=$(scan_package "$tmp" 2>&1); rm -rf "$tmp"
  if grep -q '\[size\]' <<<"$out"; then echo "ok   size (501 lines)"; else echo "FAIL size not detected"; rc=1; fi
  exit $rc
}

full() {
  local root=$1 tool
  for tool in swiftlint swift-format; do command -v "$tool" >/dev/null || fail "$tool not installed"; done
  command -v swiftlint >/dev/null && { (cd "$root" && swiftlint lint --strict --config .swiftlint.yml) || status=1; }
  command -v swift-format >/dev/null && { (cd "$root" && swift-format lint --strict --recursive --configuration .swift-format Sources Tests) || status=1; }
}

do_full=0; root=""
for a in "$@"; do
  case $a in --self-test) self_test ;; --full) do_full=1 ;; *) root=$a ;; esac
done
if [[ -z $root ]]; then
  root="$repo/app/mobile/ios"
fi
scan_package "$root"
((do_full)) && full "$root"
((status)) && echo "check-swift-rules: violations found (see docs/swift-style.md)"
exit $status
