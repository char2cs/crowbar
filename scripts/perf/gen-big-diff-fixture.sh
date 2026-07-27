#!/usr/bin/env bash
# Generates a deterministic git repo carrying a large branch diff, for perf
# measurement of Crowbar's diff subsystem (see
# docs/superpowers/specs/2026-07-27-diff-subsystem-at-scale-design.md).
#
# Deterministic by construction: content is a pure function of file index and
# line index, commits use fixed author/date, so two runs produce identical
# trees and measurements are comparable across runs.
#
# Usage: gen-big-diff-fixture.sh <dest-dir> [smoke|full]
#   smoke — ~4k changed lines, seconds to build. For CI and iteration.
#   full  — ~1M changed lines across 400 files. The real target scale.
set -euo pipefail

DEST="${1:?usage: gen-big-diff-fixture.sh <dest-dir> [smoke|full]}"
SCALE="${2:-smoke}"

case "$SCALE" in
  smoke) FILES=40  ; LINES=100  ; HUGE_FILES=1 ; HUGE_LINES=2000   ;;
  full)  FILES=400 ; LINES=1250 ; HUGE_FILES=2 ; HUGE_LINES=250000 ;;
  *) echo "unknown scale: $SCALE (want smoke|full)" >&2; exit 2 ;;
esac

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

# gen_binary <path> <bytes> — deterministic non-UTF8 bytes so git marks it binary.
gen_binary() {
  mkdir -p "$(dirname "$1")"
  perl -e 'my $n = $ARGV[0]; print pack("C*", map { ($_ * 7 + 13) % 256 } 0 .. $n - 1);' "$2" > "$1"
}

echo "generating base tree ($FILES files x $LINES lines)..." >&2
for i in $(seq 1 "$FILES"); do
  gen_text "src/pkg$((i % 20))/file$i.ts" "$LINES" "base$i"
done
gen_text "src/keep/untouched.ts" "$LINES" "stable"
gen_text "src/move/original-name.ts" "$LINES" "moved"
gen_text "src/gone/deleted.ts" "$LINES" "doomed"
gen_binary "assets/base.bin" 4096

git add -A
git commit -q -m "base: initial tree"

echo "generating branch diff..." >&2
git checkout -q -b perf/big-diff

# Modify every generated file: a new salt changes nearly every line, which is
# what makes this a *content* diff rather than a cheap add-only one.
for i in $(seq 1 "$FILES"); do
  gen_text "src/pkg$((i % 20))/file$i.ts" "$LINES" "head$i"
done

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
