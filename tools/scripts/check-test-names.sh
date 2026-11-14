#!/usr/bin/env bash
# Rejects Go test files named after coverage rather than behaviour
# (*_coverage_test.go, *_extra_test.go, *_gaps_test.go). Existing offenders are
# listed in scripts/test-name-baseline.txt; the list may only shrink.
set -euo pipefail
cd "$(dirname "$0")/.."

baseline=scripts/test-name-baseline.txt
current=$(git ls-files -- 'api/**/*_coverage_test.go' 'api/**/*_extra_test.go' 'api/**/*_gaps_test.go' | LC_ALL=C sort)

new=$(LC_ALL=C comm -13 "$baseline" <(printf '%s\n' "$current" | sed '/^$/d'))
if [[ -n "$new" ]]; then
  echo "Test files must be named for the behaviour they test, not for coverage:"
  echo "$new" | sed 's/^/  /'
  echo "Extend the unit's existing _test.go file instead."
  exit 1
fi
gone=$(LC_ALL=C comm -23 "$baseline" <(printf '%s\n' "$current" | sed '/^$/d'))
if [[ -n "$gone" ]]; then
  echo "Renamed or deleted, remove from $baseline:"
  echo "$gone" | sed 's/^/  /'
  exit 1
fi
echo "test names: ok ($(wc -l <"$baseline") baselined)"
