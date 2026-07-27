#!/usr/bin/env bash
# Smoke test for gen-big-diff-fixture.sh. Asserts the generated repo has the
# shape the perf fixture promises: a base branch, a big-diff branch, and a diff
# between them containing modifications, an addition, a deletion, a rename, and
# a binary file.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GEN="$SCRIPT_DIR/gen-big-diff-fixture.sh"
DEST="$(mktemp -d)"
trap 'rm -rf "$DEST"' EXIT

"$GEN" "$DEST/repo" smoke

cd "$DEST/repo"

fail() { echo "FAIL: $1" >&2; exit 1; }

[ "$(git rev-parse --abbrev-ref HEAD)" = "perf/big-diff" ] || fail "not on perf/big-diff"

names=$(git diff --name-status -M main)
echo "$names" | grep -q '^M' || fail "no modified files in diff"
echo "$names" | grep -q '^A' || fail "no added files in diff"
echo "$names" | grep -q '^D' || fail "no deleted files in diff"
echo "$names" | grep -q '^R' || fail "no renamed files in diff"

git diff --numstat -M main | grep -q '^-	-	' || fail "no binary file in diff"

changed=$(git diff --shortstat -M main | grep -oE '[0-9]+ insertion' | grep -oE '[0-9]+')
[ "$changed" -gt 2000 ] || fail "smoke diff too small: $changed insertions"

# Determinism: a second generation from the same inputs yields the same tree.
"$GEN" "$DEST/repo2" smoke
a=$(git -C "$DEST/repo" rev-parse 'perf/big-diff^{tree}')
b=$(git -C "$DEST/repo2" rev-parse 'perf/big-diff^{tree}')
[ "$a" = "$b" ] || fail "generator is not deterministic: $a != $b"

echo "PASS"
