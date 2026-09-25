#!/usr/bin/env bash
# Dead-code gate: fails on any function reachable only from tests that is not
# in deadcode-baseline.txt, and on baseline entries that are no longer dead
# (so the baseline can only shrink). Mock packages are allowlisted.
#
#   scripts/deadcode-check.sh            check
#   scripts/deadcode-check.sh --update   rewrite the baseline (only to shrink it)
set -euo pipefail
cd "$(dirname "$0")/.."
# `go install golang.org/x/tools/cmd/deadcode@latest` puts it here.
PATH="$(go env GOPATH)/bin:$PATH"

baseline=deadcode-baseline.txt
current=$(mktemp)
trap 'rm -f "$current"' EXIT

# "path:line:col: unreachable func: Name" -> "path: Name" (line numbers churn).
deadcode -tags noEmbed ./cmd/... |
  { grep -v '/mocks/' || true; } |
  sed -E 's/^([^:]+):[0-9]+:[0-9]+: unreachable func: (.*)$/\1: \2/' |
  LC_ALL=C sort -u >"$current"

if [[ "${1:-}" == "--update" ]]; then
  cp "$current" "$baseline"
  echo "deadcode: baseline rewritten ($(wc -l <"$baseline") entries)"
  exit 0
fi

new=$(LC_ALL=C comm -13 "$baseline" "$current")
gone=$(LC_ALL=C comm -23 "$baseline" "$current")
status=0
if [[ -n "$new" ]]; then
  echo "deadcode: functions reachable only from tests (delete them, or wire them up):"
  echo "$new" | sed 's/^/  /'
  status=1
fi
if [[ -n "$gone" ]]; then
  echo "deadcode: no longer dead, remove from $baseline (scripts/deadcode-check.sh --update):"
  echo "$gone" | sed 's/^/  /'
  status=1
fi
if [[ $status -eq 0 ]]; then
  echo "deadcode: clean ($(wc -l <"$baseline") baselined)"
fi
exit $status
