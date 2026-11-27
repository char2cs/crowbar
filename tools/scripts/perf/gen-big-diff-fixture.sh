#!/usr/bin/env bash
# Generates a deterministic git repo carrying a large branch diff, for perf
# measurement of Crowbar's diff subsystem (see
# docs/superpowers/specs/2026-07-27-diff-subsystem-at-scale-design.md).
#
# Deterministic by construction: content is a pure function of file index and
# line index, commits use fixed author/date, so two runs produce identical
# trees and measurements are comparable across runs.
#
# The file mix is deliberate. An earlier version rewrote every line of every
# file, which produced exactly ONE hunk per file — 404 hunks for a million
# changed lines. That shape silently under-tests everything hunk-shaped: the
# outline scanner, hunk separators, expand-unchanged, and the per-file density
# guard. A measuring instrument that cannot fail is worthless, so the mix now
# spans the axes that actually stress different code paths:
#
#   scattered — every Nth line changed, so hunks are many and separated by
#               context. The common real-world review shape.
#   dense     — whole file rewritten: one enormous hunk.
#   huge      — a single file large enough to break naive renderers.
#   shotgun   — enough scattered hunks in ONE file to trip a per-file hunk cap.
#   minified  — pathological line LENGTH rather than line count.
#   binary / rename / delete — non-text and non-modify statuses.
#
# Usage: gen-big-diff-fixture.sh <dest-dir> [smoke|full]
#   smoke — ~3k changed lines, seconds to build. For CI and iteration.
#   full  — ~1M changed lines across ~400 files. The real target scale.
set -euo pipefail

DEST="${1:?usage: gen-big-diff-fixture.sh <dest-dir> [smoke|full]}"
SCALE="${2:-smoke}"

case "$SCALE" in
  smoke)
    SCATTERED_FILES=30 ; DENSE_FILES=10  ; LINES=100
    HUGE_FILES=1       ; HUGE_LINES=2000 ; SHOTGUN_LINES=1500
    ;;
  full)
    SCATTERED_FILES=300 ; DENSE_FILES=100   ; LINES=1250
    HUGE_FILES=2        ; HUGE_LINES=420000 ; SHOTGUN_LINES=15000
    ;;
  *) echo "unknown scale: $SCALE (want smoke|full)" >&2; exit 2 ;;
esac

# Change every Nth line in scattered files. 10 is chosen against git's default
# 3 lines of context: consecutive changes 10 apart leave a 3-line gap, so each
# becomes its OWN hunk. At 5 the context windows would overlap and git would
# merge them back into one hunk, quietly undoing the point of this file.
SCATTER_EVERY=10

if [ -e "$DEST" ] && [ -n "$(ls -A "$DEST" 2>/dev/null)" ]; then
  echo "refusing to generate into non-empty $DEST" >&2
  exit 1
fi

mkdir -p "$DEST"
cd "$DEST"

# Fixed identity and timestamps so commit SHAs are reproducible.
export GIT_AUTHOR_NAME=Perf GIT_AUTHOR_EMAIL=perf@example.com
export GIT_COMMITTER_NAME=Perf GIT_COMMITTER_EMAIL=perf@example.com
export GIT_AUTHOR_DATE='2026-01-01T00:00:00+00:00'
export GIT_COMMITTER_DATE='2026-01-01T00:00:00+00:00'

git init -q -b main
git config user.name Perf
git config user.email perf@example.com

# gen_text <path> <lines> <salt>
# Content is a pure function of (line index, salt) — no randomness.
gen_text() {
  mkdir -p "$(dirname "$1")"
  awk -v n="$2" -v s="$3" 'BEGIN{
    for (i = 1; i <= n; i++)
      printf "%s line %d token %d\n", s, i, (i * 2654435761 + length(s)) % 1000003
  }' > "$1"
}

# gen_text_scattered <path> <lines> <base-salt> <head-salt> <every>
# Same as gen_text but every Nth line carries the head salt instead of the
# base salt, so a diff against the gen_text version of the same file yields one
# small hunk per changed line rather than a single whole-file hunk.
gen_text_scattered() {
  mkdir -p "$(dirname "$1")"
  awk -v n="$2" -v b="$3" -v h="$4" -v e="$5" 'BEGIN{
    for (i = 1; i <= n; i++) {
      s = (i % e == 0) ? h : b
      printf "%s line %d token %d\n", s, i, (i * 2654435761 + length(s)) % 1000003
    }
  }' > "$1"
}

# gen_binary <path> <bytes> — deterministic non-UTF8 bytes so git marks it binary.
gen_binary() {
  mkdir -p "$(dirname "$1")"
  perl -e 'my $n = $ARGV[0]; print pack("C*", map { ($_ * 7 + 13) % 256 } 0 .. $n - 1);' "$2" > "$1"
}

echo "generating base tree (${SCATTERED_FILES} scattered + ${DENSE_FILES} dense x ${LINES} lines)..." >&2
for i in $(seq 1 "$SCATTERED_FILES"); do
  gen_text "src/scattered/pkg$((i % 20))/file$i.ts" "$LINES" "base$i"
done
for i in $(seq 1 "$DENSE_FILES"); do
  gen_text "src/dense/pkg$((i % 10))/file$i.ts" "$LINES" "base$i"
done
gen_text "src/shotgun/wide.ts" "$SHOTGUN_LINES" "base0"
gen_text "src/keep/untouched.ts" "$LINES" "stable"
gen_text "src/move/original-name.ts" "$LINES" "moved"
gen_text "src/gone/deleted.ts" "$LINES" "doomed"
gen_binary "assets/base.bin" 4096

git add -A
git commit -q -m "base: initial tree"

echo "generating branch diff..." >&2
git checkout -q -b perf/big-diff

# Scattered: many small hunks per file, separated by unchanged context.
for i in $(seq 1 "$SCATTERED_FILES"); do
  gen_text_scattered "src/scattered/pkg$((i % 20))/file$i.ts" \
    "$LINES" "base$i" "head$i" "$SCATTER_EVERY"
done

# Dense: whole-file rewrite, one enormous hunk each.
for i in $(seq 1 "$DENSE_FILES"); do
  gen_text "src/dense/pkg$((i % 10))/file$i.ts" "$LINES" "head$i"
done

# Shotgun: one file with enough hunks to exceed a per-file hunk cap.
gen_text_scattered "src/shotgun/wide.ts" \
  "$SHOTGUN_LINES" "base0" "head0" "$SCATTER_EVERY"

# The monster files — a single file large enough to break naive renderers.
for i in $(seq 1 "$HUGE_FILES"); do
  gen_text "src/huge/monster$i.ts" "$HUGE_LINES" "huge$i"
done

# A minified single-line file: pathological line length, not line count.
mkdir -p src/minified
awk 'BEGIN{ for (i = 0; i < 40000; i++) printf "var a%d=%d;", i, i; printf "\n" }' \
  > src/minified/bundle.min.js

git mv src/move/original-name.ts src/move/renamed-name.ts
git rm -q src/gone/deleted.ts
gen_binary "assets/changed.bin" 8192

git add -A
git commit -q -m "head: large branch diff"

echo "fixture ready at $DEST (scale=$SCALE)" >&2
git diff --shortstat -M main >&2
printf 'hunks: %s\n' "$(git diff -M main -- | grep -c '^@@')" >&2
