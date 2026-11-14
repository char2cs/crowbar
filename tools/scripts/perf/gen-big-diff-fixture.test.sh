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

# Hunk shape. The first version of this generator rewrote every line of every
# file, yielding exactly ONE hunk per file — a diff that looks huge but never
# exercises the outline scanner, hunk separators, expand-unchanged, or any
# per-file hunk cap. These three assertions are what would have caught that.
hunks_per_file=$(git diff -M main -- |
  awk '/^diff --git/{f=$3} /^@@/{c[f]++} END{for (k in c) print c[k]}')

total_hunks=$(echo "$hunks_per_file" | awk '{s+=$1} END{print s+0}')
max_hunks=$(echo "$hunks_per_file" | sort -rn | head -1)
multi_hunk_files=$(echo "$hunks_per_file" | awk '$1 >= 5' | wc -l | tr -d ' ')

[ "$total_hunks" -gt 300 ] || fail "too few hunks overall: $total_hunks"
[ "$max_hunks" -ge 100 ] || fail "no shotgun file: max hunks in one file is $max_hunks"
[ "$multi_hunk_files" -ge 20 ] || fail "only $multi_hunk_files files have >=5 hunks; diff is not scattered"

# A single-hunk file must also exist — the dense whole-file-rewrite shape is a
# distinct rendering path from the scattered one and both must be represented.
echo "$hunks_per_file" | grep -qx '1' || fail "no single-hunk (dense-rewrite) file in diff"

# Determinism: a second generation from the same inputs yields the same tree.
"$GEN" "$DEST/repo2" smoke
a=$(git -C "$DEST/repo" rev-parse 'perf/big-diff^{tree}')
b=$(git -C "$DEST/repo2" rev-parse 'perf/big-diff^{tree}')
[ "$a" = "$b" ] || fail "generator is not deterministic: $a != $b"

echo "PASS"
