#!/usr/bin/env bash
# Runs the Rust, Swift and TypeScript rule checkers (docs/rust-style.md, swift-style.md, ts-style.md).
# Usage: check-standards.sh [--self-test] [--full]
set -uo pipefail
here=$(cd "$(dirname "$0")" && pwd)
status=0
args=("$@")
for checker in check-rust-rules.sh check-swift-rules.sh check-ts-rules.sh; do
  echo "== $checker ${args[*]}"
  if [[ " ${args[*]} " == *" --self-test "* ]]; then bash "$here/$checker" --self-test || status=1; fi
  [[ " ${args[*]} " == *" --self-test "* ]] && continue
  extra=(); [[ $checker == check-rust-rules.sh ]] && extra=(--mobile)
  bash "$here/$checker" "${extra[@]}" "${args[@]}" || status=1
done
((status)) && echo "check-standards: FAILED" || echo "check-standards: clean"
exit $status
