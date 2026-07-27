# Diff Perf Phase 0 — Fixture, Instrumentation, Attribution — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the fixture and the frontend + daemon instrumentation needed to
prove which of three suspected causes actually freezes Crowbar on a large branch
diff, and record the before-numbers every later phase is measured against.

**Architecture:** Phase 0 fixes nothing. It adds a deterministic large-diff repo
generator, a measure-only ring on the frontend, a bounded sample ring plus
request/subprocess/lock timing in the daemon, and a `/v0/system/perf` read
endpoint. The final task drives the already-running dev Tauri app through its
MCP bridge against the fixture and writes the attribution result into
`perf-baselines.md`.

**Tech Stack:** Go 1.x + gin (daemon), React 19 + Zustand + Vite + Vitest (web),
bash + awk (fixture), Tauri MCP bridge (live measurement).

## Global Constraints

- Spec: `docs/superpowers/specs/2026-07-27-diff-subsystem-at-scale-design.md`. Phase 0 only.
- **No behaviour changes.** Phase 0 adds measurement. If a task finds a bug, record it; do not fix it here.
- **Reuse the running dev instance.** `tauri dev` pid 84589, vite :5173, `crowbar-api` on `unix:///var/folders/3k/dm5s9q9j6y7_6z69f5yyyvmw0000gn/T/crowbar-381fc6a71389fd95.sock`. Never launch a second dev app — stacked launches orphan daemons fighting one socket. Verify with `pgrep -f crowbar-api` before any live step.
- **Never touch the production Crowbar install** or `~/.crowbar/crowbar.sock`. Dev state is `CROWBAR_HOME=<worktree>/.crowbar`.
- **The fixture repo lives outside the worktree** (`${TMPDIR}/crowbar-perf-fixture`). A 1M-line fixture inside the worktree would pollute Crowbar's own git status and corrupt the very measurement being taken.
- **No timing-based synchronization in tests.** No sleeps, no polling, no `Eventually`. Block on real signals.
- Web tests: `~/.bun/bin/bun` — `bun` is NOT on the default PATH. Never `bunx tsc` (it silently runs a different package and exits 0); use `bun tsc`.
- Component files kebab-case; web tests mirror `web/src/` under `web/src/__tests__/` using `@/` imports.
- Go: follow the repo's existing package layout and comment density.

---

### Task 1: Deterministic large-diff fixture generator

**Files:**
- Create: `scripts/perf/gen-big-diff-fixture.sh`
- Create: `scripts/perf/gen-big-diff-fixture.test.sh`

**Interfaces:**
- Consumes: nothing.
- Produces: `scripts/perf/gen-big-diff-fixture.sh <dest-dir> [smoke|full]` — creates a git repo at `<dest-dir>` with branch `perf/big-diff` checked out and `main` as its base. Exits non-zero if `<dest-dir>` exists and is non-empty. Task 6 consumes it.

- [ ] **Step 1: Write the failing test**

Create `scripts/perf/gen-big-diff-fixture.test.sh`:

```bash
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
```

- [ ] **Step 2: Run test to verify it fails**

```bash
chmod +x scripts/perf/gen-big-diff-fixture.test.sh
./scripts/perf/gen-big-diff-fixture.test.sh
```

Expected: FAIL — `gen-big-diff-fixture.sh: No such file or directory`.

- [ ] **Step 3: Write the generator**

Create `scripts/perf/gen-big-diff-fixture.sh`:

```bash
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
```

- [ ] **Step 4: Run test to verify it passes**

```bash
chmod +x scripts/perf/gen-big-diff-fixture.sh
./scripts/perf/gen-big-diff-fixture.test.sh
```

Expected: `PASS`.

- [ ] **Step 5: Commit**

```bash
git add scripts/perf/gen-big-diff-fixture.sh scripts/perf/gen-big-diff-fixture.test.sh
git commit -m "test(perf): deterministic large-diff fixture generator"
```

---

### Task 2: Frontend measure-only ring + INP cap fix

**Files:**
- Modify: `web/src/lib/perf/instrumentation.ts`
- Modify: `web/src/main.tsx:70-80`
- Modify: `web/src/__tests__/lib/perf/instrumentation.test.ts`

**Interfaces:**
- Consumes: nothing.
- Produces: `pushPerfEntry(entry: PerfLogEntry): void` exported from `@/lib/perf/instrumentation`, and `window.__measures: PerfLogEntry[]` — a measure-only ring, capped at `MEASURE_RING_CAP = 2000`. Task 6 reads `window.__measures`.

**Why:** `perf-baselines.md` records that `window.__perfLog`'s 500-entry ring
floods with Event Timing entries within seconds and evicts the `measure`
entries an external reader actually wants, and that `main.tsx`'s INP push
bypasses the cap entirely (unbounded growth). Both must be fixed before any
number taken from this rig can be trusted.

- [ ] **Step 1: Write the failing tests**

Append to `web/src/__tests__/lib/perf/instrumentation.test.ts`:

```ts
describe('measure-only ring', () => {
  it('mirrors measures into window.__measures separately from __perfLog', () => {
    installPerfObserver()
    markStart('span.a')
    markEnd('span.a')

    return vi.waitFor(() => {
      expect(window.__measures?.some((e) => e.name === 'span.a')).toBe(true)
    })
  })

  it('keeps measures when __perfLog is flooded by non-measure entries', () => {
    installPerfObserver()
    markStart('span.keeper')
    markEnd('span.keeper')

    return vi
      .waitFor(() => {
        expect(window.__measures?.some((e) => e.name === 'span.keeper')).toBe(true)
      })
      .then(() => {
        for (let i = 0; i < 1000; i++) {
          pushPerfEntry({ name: `event:${i}`, startTime: i, duration: 1, entryType: 'event' })
        }
        expect(window.__measures?.some((e) => e.name === 'span.keeper')).toBe(true)
      })
  })

  it('pushPerfEntry respects the __perfLog ring cap', () => {
    installPerfObserver()
    for (let i = 0; i < 2000; i++) {
      pushPerfEntry({ name: `INP:good`, startTime: i, duration: 1, entryType: 'event' })
    }
    expect(window.__perfLog!.length).toBeLessThanOrEqual(500)
  })
})
```

Add `pushPerfEntry` to the existing import at the top of the file.

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd web && ~/.bun/bin/bun run vitest run src/__tests__/lib/perf/instrumentation.test.ts
```

Expected: FAIL — `pushPerfEntry is not exported` / `window.__measures` undefined.

- [ ] **Step 3: Implement**

In `web/src/lib/perf/instrumentation.ts`, add the cap and declaration:

```ts
const RING_CAP = 500
// Measures are the signal; Event Timing entries are the noise that evicts them
// from __perfLog within seconds under real interaction. External readers (perf
// capture runs) read THIS ring, which only ever receives `measure` entries and
// is sized for a full scenario rather than a few seconds.
const MEASURE_RING_CAP = 2000
```

Extend the `Window` interface with `__measures?: PerfLogEntry[]`.

Add the shared push helper:

```ts
/**
 * Push an entry into the shared perf ring, honouring the cap. Exported so
 * out-of-module producers (the INP reporter in main.tsx) cannot grow the ring
 * without bound — a direct `window.__perfLog.push` did exactly that.
 */
export function pushPerfEntry(entry: PerfLogEntry): void {
  if (!perfEnabled()) return
  window.__perfLog ??= []
  const log = window.__perfLog
  log.push(entry)
  if (log.length > RING_CAP) log.shift()
}
```

In `installPerfObserver`, initialise the second ring alongside the first:

```ts
  window.__perfLog ??= []
  window.__measures ??= []
```

and inside the observer's entry loop, before the existing `entryType === 'measure'` cleanup block:

```ts
      if (e.entryType === 'measure') {
        const measures = window.__measures!
        measures.push({
          name: e.name,
          startTime: e.startTime,
          duration: e.duration,
          entryType: e.entryType,
        })
        if (measures.length > MEASURE_RING_CAP) measures.shift()
      }
```

In `__resetPerfForTests`, add `delete window.__measures` next to the existing
`delete window.__perfLog`.

- [ ] **Step 4: Route the INP reporter through the capped helper**

In `web/src/main.tsx`, replace the direct `window.__perfLog?.push({...})` call
at line 76 with `pushPerfEntry({...})`, and add `pushPerfEntry` to the existing
import from `@/lib/perf/instrumentation` on line 16.

- [ ] **Step 5: Run tests to verify they pass**

```bash
cd web && ~/.bun/bin/bun run vitest run src/__tests__/lib/perf/instrumentation.test.ts
cd web && ~/.bun/bin/bun tsc --noEmit
```

Expected: all PASS, no type errors.

- [ ] **Step 6: Commit**

```bash
git add web/src/lib/perf/instrumentation.ts web/src/main.tsx web/src/__tests__/lib/perf/instrumentation.test.ts
git commit -m "perf(web): measure-only ring + cap the INP push"
```

---

### Task 3: Daemon perf sample ring

**Files:**
- Create: `api/internal/perf/ring.go`
- Create: `api/internal/perf/ring_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: package `perf` with `type Sample struct { Name string; DurationMS float64; At time.Time }`, `func Record(name string, d time.Duration)`, `func Snapshot() []Sample`, `func Reset()`, `func SetEnabled(bool)`, `func Enabled() bool`. Tasks 4, 5 and 6 consume it.

- [ ] **Step 1: Write the failing test**

Create `api/internal/perf/ring_test.go`:

```go
package perf_test

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/perf"
)

func TestRecord_DisabledByDefault(t *testing.T) {
	perf.Reset()
	perf.SetEnabled(false)
	perf.Record("git.diff", 5*time.Millisecond)
	assert.Empty(t, perf.Snapshot())
}

func TestRecord_CapturesNameAndDuration(t *testing.T) {
	perf.Reset()
	perf.SetEnabled(true)
	t.Cleanup(func() { perf.SetEnabled(false) })

	perf.Record("git.diff", 5*time.Millisecond)

	got := perf.Snapshot()
	require.Len(t, got, 1)
	assert.Equal(t, "git.diff", got[0].Name)
	assert.InDelta(t, 5.0, got[0].DurationMS, 0.001)
}

func TestSnapshot_EvictsOldestBeyondCap(t *testing.T) {
	perf.Reset()
	perf.SetEnabled(true)
	t.Cleanup(func() { perf.SetEnabled(false) })

	for i := 0; i < perf.Cap+100; i++ {
		perf.Record("n", time.Millisecond)
	}

	assert.Len(t, perf.Snapshot(), perf.Cap)
}

func TestRecord_ConcurrentIsRaceFree(t *testing.T) {
	perf.Reset()
	perf.SetEnabled(true)
	t.Cleanup(func() { perf.SetEnabled(false) })

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				perf.Record("concurrent", time.Microsecond)
			}
		}()
	}
	wg.Wait()

	assert.Len(t, perf.Snapshot(), perf.Cap)
}

func TestSnapshot_ReturnsCopy(t *testing.T) {
	perf.Reset()
	perf.SetEnabled(true)
	t.Cleanup(func() { perf.SetEnabled(false) })

	perf.Record("a", time.Millisecond)
	got := perf.Snapshot()
	got[0].Name = "mutated"

	assert.Equal(t, "a", perf.Snapshot()[0].Name)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd api && go test ./internal/perf/... -run Test -v
```

Expected: FAIL — package `internal/perf` does not exist.

- [ ] **Step 3: Implement**

Create `api/internal/perf/ring.go`:

```go
// Package perf holds a bounded, race-free ring of timing samples used to
// attribute daemon-side latency during performance work. It is OFF by default
// and costs one atomic load per Record call when disabled, so instrumented
// call sites can stay in the code permanently without a production cost.
package perf

import (
	"sync"
	"sync/atomic"
	"time"
)

// Cap bounds the ring. Sized for a full measurement scenario (a few minutes of
// interaction) rather than a few seconds, so a capture run does not evict the
// samples it is trying to read.
const Cap = 4096

// Sample is one timed operation.
type Sample struct {
	Name       string    `json:"name"`
	DurationMS float64   `json:"durationMs"`
	At         time.Time `json:"at"`
}

var (
	enabled atomic.Bool

	mu      sync.Mutex
	samples []Sample
)

// SetEnabled arms or disarms recording. Disabled is the default.
func SetEnabled(v bool) { enabled.Store(v) }

// Enabled reports whether recording is armed.
func Enabled() bool { return enabled.Load() }

// Record appends one sample. It is a no-op — one atomic load — while disabled.
func Record(name string, d time.Duration) {
	if !enabled.Load() {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	samples = append(samples, Sample{
		Name:       name,
		DurationMS: float64(d.Nanoseconds()) / 1e6,
		At:         time.Now(),
	})
	if len(samples) > Cap {
		samples = samples[len(samples)-Cap:]
	}
}

// Snapshot returns a copy of the current ring, oldest first.
func Snapshot() []Sample {
	mu.Lock()
	defer mu.Unlock()
	out := make([]Sample, len(samples))
	copy(out, samples)
	return out
}

// Reset clears the ring. Used by tests and by the capture endpoint to start a
// clean measurement window.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	samples = nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd api && go test ./internal/perf/... -race -v
```

Expected: all PASS, no race detected.

- [ ] **Step 5: Commit**

```bash
git add api/internal/perf/
git commit -m "perf(api): bounded race-free timing sample ring"
```

---

### Task 4: Git subprocess and repo-lock instrumentation

**Files:**
- Modify: `api/internal/engine/git/internal/exec/exec.go`
- Modify: `api/internal/engine/git/engine.go:76-92`
- Create: `api/internal/engine/git/internal/exec/timing_test.go`
- Create: `api/internal/engine/git/lock_timing_test.go`

**Interfaces:**
- Consumes: `perf.Record`, `perf.SetEnabled`, `perf.Snapshot` from Task 3.
- Produces: samples named `git.<subcommand>` (e.g. `git.diff`, `git.status`), `lock.read.wait`, `lock.read.hold`, `lock.write.wait`, `lock.write.hold`. Task 6 reads these names.

**Why:** The spec's attribution questions — how long `/review/files` takes at
1M lines, how long the repo read lock is held, whether writers starve — cannot
be answered without these two call sites.

- [ ] **Step 1: Write the failing tests**

Create `api/internal/engine/git/internal/exec/timing_test.go`:

```go
package exec_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
	"github.com/char2cs/crowbar/api/internal/perf"
)

func TestGit_RecordsSampleNamedForSubcommand(t *testing.T) {
	dir := initRepo(t)
	perf.Reset()
	perf.SetEnabled(true)
	t.Cleanup(func() { perf.SetEnabled(false); perf.Reset() })

	r := exec.Git(context.Background(), dir, "status")
	require.Equal(t, 0, r.ExitCode)

	var names []string
	for _, s := range perf.Snapshot() {
		names = append(names, s.Name)
	}
	assert.Contains(t, names, "git.status")
}

func TestGit_RecordsNothingWhenDisabled(t *testing.T) {
	dir := initRepo(t)
	perf.Reset()
	perf.SetEnabled(false)

	_ = exec.Git(context.Background(), dir, "status")

	assert.Empty(t, perf.Snapshot())
}

func TestGit_SubcommandNameIgnoresFlags(t *testing.T) {
	dir := initRepo(t)
	perf.Reset()
	perf.SetEnabled(true)
	t.Cleanup(func() { perf.SetEnabled(false); perf.Reset() })

	_ = exec.Git(context.Background(), dir, "-c", "core.quotepath=false", "status")

	for _, s := range perf.Snapshot() {
		assert.False(t, strings.Contains(s.Name, "-c"), "sample name leaked a flag: %s", s.Name)
	}
}
```

Create `api/internal/engine/git/lock_timing_test.go`:

```go
package git

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/perf"
)

func TestLockRepoRead_RecordsWaitAndHold(t *testing.T) {
	perf.Reset()
	perf.SetEnabled(true)
	t.Cleanup(func() { perf.SetEnabled(false); perf.Reset() })

	// New() — NOT &engine{}. repoMutex resolves the git common dir through
	// e.exec (engine.go:56), so an engine with a nil exec func panics here.
	e := New().(*engine)
	unlock := e.lockRepoRead(context.Background(), t.TempDir())
	unlock()

	var names []string
	for _, s := range perf.Snapshot() {
		names = append(names, s.Name)
	}
	assert.Contains(t, names, "lock.read.wait")
	assert.Contains(t, names, "lock.read.hold")
}

func TestLockRepo_RecordsWaitAndHold(t *testing.T) {
	perf.Reset()
	perf.SetEnabled(true)
	t.Cleanup(func() { perf.SetEnabled(false); perf.Reset() })

	e := New().(*engine)
	unlock := e.lockRepo(context.Background(), t.TempDir())
	unlock()

	var names []string
	for _, s := range perf.Snapshot() {
		names = append(names, s.Name)
	}
	assert.Contains(t, names, "lock.write.wait")
	assert.Contains(t, names, "lock.write.hold")
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd api && go test ./internal/engine/git/internal/exec/... -run 'TestGit_Records|TestGit_SubcommandName' -v
cd api && go test ./internal/engine/git/... -run LockRepo -v
```

Expected: FAIL — no samples recorded (`names` empty).

Note the `-run` pattern: no test in `timing_test.go` contains the word
"Timing", so `-run Timing` matches nothing and reports PASS with "no tests to
run" — a green that proves nothing. Match on the actual test names.

- [ ] **Step 3: Instrument the subprocess**

In `api/internal/engine/git/internal/exec/exec.go`, add imports for `time` and
`github.com/char2cs/crowbar/api/internal/perf`, then add:

```go
// subcommandName returns the git subcommand for a sample name, skipping the
// leading global flags (`-c key=value`, `--git-dir=…`) that some call sites
// pass. Returns "unknown" when no subcommand is present.
func subcommandName(args []string) string {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "-c" {
			i++
			continue
		}
		if strings.HasPrefix(a, "-") {
			continue
		}
		return a
	}
	return "unknown"
}
```

and wrap the body of `run` (the single lowest-level entry point every exported
helper funnels through) so all three of `Git`, `GitWithEnv` and `GitWithStdin`
are covered by one change:

```go
func run(
	ctx context.Context,
	dir string,
	extraEnv []string,
	stdin string,
	hasStdin bool,
	args ...string,
) Result {
	if !perf.Enabled() {
		return runInner(ctx, dir, extraEnv, stdin, hasStdin, args...)
	}
	start := time.Now()
	r := runInner(ctx, dir, extraEnv, stdin, hasStdin, args...)
	perf.Record("git."+subcommandName(args), time.Since(start))
	return r
}
```

Rename the existing `run` (declared at `exec.go:80`, params
`ctx, dir, extraEnv, stdin, hasStdin, args...`) to `runInner`, keeping its
signature and body byte-for-byte including the `//nolint:gosec` directive. Do
not change its logic. `strings` is already imported in this file.

- [ ] **Step 4: Instrument the locks**

In `api/internal/engine/git/engine.go`, add imports for `time` and the `perf`
package, then replace the two lock helpers:

```go
func (e *engine) lockRepo(ctx context.Context, repoPath string) func() {
	mu := e.repoMutex(ctx, repoPath)
	if !perf.Enabled() {
		mu.Lock()
		return mu.Unlock
	}
	waitStart := time.Now()
	mu.Lock()
	perf.Record("lock.write.wait", time.Since(waitStart))
	held := time.Now()
	return func() {
		perf.Record("lock.write.hold", time.Since(held))
		mu.Unlock()
	}
}

func (e *engine) lockRepoRead(ctx context.Context, repoPath string) func() {
	mu := e.repoMutex(ctx, repoPath)
	if !perf.Enabled() {
		mu.RLock()
		return mu.RUnlock
	}
	waitStart := time.Now()
	mu.RLock()
	perf.Record("lock.read.wait", time.Since(waitStart))
	held := time.Now()
	return func() {
		perf.Record("lock.read.hold", time.Since(held))
		mu.RUnlock()
	}
}
```

Keep both existing doc comments verbatim above their functions.

- [ ] **Step 5: Run tests to verify they pass**

```bash
cd api && go test ./internal/engine/git/... -race
cd api && go test ./internal/perf/... -race
```

Expected: all PASS, no races.

- [ ] **Step 6: Commit**

```bash
git add api/internal/engine/git/internal/exec/exec.go api/internal/engine/git/internal/exec/timing_test.go api/internal/engine/git/engine.go api/internal/engine/git/lock_timing_test.go
git commit -m "perf(api): time git subprocesses and repo lock wait/hold"
```

---

### Task 5: Request timing middleware and /system/perf endpoint

**Files:**
- Create: `api/internal/api/middleware/timing.go`
- Create: `api/internal/api/middleware/timing_test.go`
- Create: `api/internal/api/v0/endpoints/system/handlers/perf.go`
- Create: `api/internal/api/v0/endpoints/system/handlers/perf_test.go`
- Modify: `api/internal/api/v0/endpoints/system/routes.go`

**Interfaces:**
- Consumes: `perf.Record`, `perf.Snapshot`, `perf.Reset`, `perf.SetEnabled` from Task 3.
- Produces: `middleware.Timing() gin.HandlerFunc` recording samples named `http.<METHOD> <route-template>`; `GET /v0/system/perf` returning `{"enabled":bool,"samples":[…]}`; `POST /v0/system/perf?enabled=true|false` arming/disarming and clearing the ring. Task 6 calls both.

**Why:** Task 6 needs to read daemon samples from outside the daemon. The route
*template* (not the concrete path) is used so hundreds of per-workspace URLs
collapse into one bucket.

- [ ] **Step 1: Write the failing tests**

Create `api/internal/api/middleware/timing_test.go`:

```go
package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/middleware"
	"github.com/char2cs/crowbar/api/internal/perf"
)

func newRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.Timing())
	r.GET("/v0/workspaces/:wsId/review/files", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	return r
}

func TestTiming_RecordsRouteTemplateNotConcretePath(t *testing.T) {
	perf.Reset()
	perf.SetEnabled(true)
	t.Cleanup(func() { perf.SetEnabled(false); perf.Reset() })

	r := newRouter()
	for _, ws := range []string{"ws-a", "ws-b"} {
		req := httptest.NewRequest(http.MethodGet, "/v0/workspaces/"+ws+"/review/files", nil)
		r.ServeHTTP(httptest.NewRecorder(), req)
	}

	samples := perf.Snapshot()
	assert.Len(t, samples, 2)
	for _, s := range samples {
		assert.Equal(t, "http.GET /v0/workspaces/:wsId/review/files", s.Name)
	}
}

func TestTiming_RecordsNothingWhenDisabled(t *testing.T) {
	perf.Reset()
	perf.SetEnabled(false)

	r := newRouter()
	req := httptest.NewRequest(http.MethodGet, "/v0/workspaces/ws-a/review/files", nil)
	r.ServeHTTP(httptest.NewRecorder(), req)

	assert.Empty(t, perf.Snapshot())
}

func TestTiming_EmitsServerTimingHeader(t *testing.T) {
	perf.Reset()
	perf.SetEnabled(true)
	t.Cleanup(func() { perf.SetEnabled(false); perf.Reset() })

	r := newRouter()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v0/workspaces/ws-a/review/files", nil)
	r.ServeHTTP(rec, req)

	assert.Contains(t, rec.Header().Get("Server-Timing"), "total;dur=")
}
```

Create `api/internal/api/v0/endpoints/system/handlers/perf_test.go`:

```go
package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	systemhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/system/handlers"
	"github.com/char2cs/crowbar/api/internal/perf"
)

func perfRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := systemhandlers.NewPerfHandler()
	r.GET("/v0/system/perf", h.Get)
	r.POST("/v0/system/perf", h.Set)
	return r
}

func TestPerfGet_ReturnsSamples(t *testing.T) {
	perf.Reset()
	perf.SetEnabled(true)
	t.Cleanup(func() { perf.SetEnabled(false); perf.Reset() })
	perf.Record("git.diff", 12*time.Millisecond)

	rec := httptest.NewRecorder()
	perfRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v0/system/perf", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Data struct {
			Enabled bool          `json:"enabled"`
			Samples []perf.Sample `json:"samples"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.True(t, body.Data.Enabled)
	require.Len(t, body.Data.Samples, 1)
	assert.Equal(t, "git.diff", body.Data.Samples[0].Name)
}

func TestPerfSet_ArmsAndClears(t *testing.T) {
	perf.Reset()
	perf.SetEnabled(false)
	t.Cleanup(func() { perf.SetEnabled(false); perf.Reset() })

	rec := httptest.NewRecorder()
	perfRouter().ServeHTTP(rec,
		httptest.NewRequest(http.MethodPost, "/v0/system/perf?enabled=true", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, perf.Enabled())
	assert.Empty(t, perf.Snapshot())
}

func TestPerfSet_Disarms(t *testing.T) {
	perf.Reset()
	perf.SetEnabled(true)
	t.Cleanup(func() { perf.SetEnabled(false); perf.Reset() })

	rec := httptest.NewRecorder()
	perfRouter().ServeHTTP(rec,
		httptest.NewRequest(http.MethodPost, "/v0/system/perf?enabled=false", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.False(t, perf.Enabled())
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd api && go test ./internal/api/middleware/... -run Timing -v
cd api && go test ./internal/api/v0/endpoints/system/... -run Perf -v
```

Expected: FAIL — `middleware.Timing` and `handlers.NewPerfHandler` undefined.

- [ ] **Step 3: Implement the middleware**

Create `api/internal/api/middleware/timing.go`:

```go
package middleware

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/perf"
)

// Timing records per-request wall time into the perf ring and echoes it as a
// Server-Timing header so the frontend can attribute latency without a second
// round trip.
//
// The sample name uses gin's route TEMPLATE (c.FullPath()), not the concrete
// URL: every workspace hits the same handler under a different :wsId, and
// bucketing by concrete path would shatter one hot endpoint into hundreds of
// single-sample names that no aggregation could reassemble.
//
// Disabled is the default; the cost is then one atomic load per request.
func Timing() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !perf.Enabled() {
			c.Next()
			return
		}
		start := time.Now()
		c.Next()
		d := time.Since(start)

		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		perf.Record(fmt.Sprintf("http.%s %s", c.Request.Method, route), d)
		c.Header("Server-Timing", fmt.Sprintf("total;dur=%.1f", float64(d.Nanoseconds())/1e6))
	}
}
```

- [ ] **Step 4: Implement the endpoint**

Create `api/internal/api/v0/endpoints/system/handlers/perf.go`:

```go
package handlers

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/perf"
)

// PerfHandler exposes the daemon's timing ring for performance capture runs.
type PerfHandler struct{}

// NewPerfHandler returns a handler for the /system/perf routes.
func NewPerfHandler() *PerfHandler { return &PerfHandler{} }

type perfResponse struct {
	Enabled bool          `json:"enabled"`
	Samples []perf.Sample `json:"samples"`
}

// Get handles GET /v0/system/perf, returning the current ring contents.
func (h *PerfHandler) Get(ctx *gin.Context) {
	samples := perf.Snapshot()
	if samples == nil {
		samples = []perf.Sample{}
	}
	libs.WriteQueryOK(ctx, perfResponse{Enabled: perf.Enabled(), Samples: samples})
}

// Set handles POST /v0/system/perf?enabled=true|false. Arming always clears the
// ring so a capture run starts from a known-empty window.
func (h *PerfHandler) Set(ctx *gin.Context) {
	on := ctx.Query("enabled") == "true"
	perf.Reset()
	perf.SetEnabled(on)
	libs.WriteQueryOK(ctx, perfResponse{Enabled: on, Samples: []perf.Sample{}})
}
```

- [ ] **Step 5: Wire the routes and the middleware**

In `api/internal/api/v0/endpoints/system/routes.go`, inside `Register`, add
below the existing prerequisites route:

```go
	p := systemhandlers.NewPerfHandler()
	rg.GET("/system/perf", p.Get)
	rg.POST("/system/perf", p.Set)
```

In `api/internal/api/container.go:46`, add `middleware.Timing(),` immediately
after the existing `middleware.Logger(),` entry so the timing middleware wraps
every v0 route.

- [ ] **Step 6: Run tests to verify they pass**

```bash
cd api && go test ./internal/api/... -race
cd api && go build ./...
```

Expected: all PASS, build clean.

- [ ] **Step 7: Commit**

```bash
git add api/internal/api/middleware/timing.go api/internal/api/middleware/timing_test.go api/internal/api/v0/endpoints/system/handlers/perf.go api/internal/api/v0/endpoints/system/handlers/perf_test.go api/internal/api/v0/endpoints/system/routes.go
git commit -m "perf(api): request timing middleware + /system/perf capture endpoint"
```

---

### Task 6: Live attribution run against the fixture

**Files:**
- Modify: `docs/superpowers/specs/perf-baselines.md`
- Create: `docs/superpowers/specs/perf-phase0-attribution.md`

**Interfaces:**
- Consumes: the generator from Task 1, `window.__measures` from Task 2, `/v0/system/perf` from Task 5, and the sample names from Task 4.
- Produces: the recorded attribution that orders Phase 1. No code.

**This task is measurement, not implementation.** It answers the spec's three
attribution questions and nothing else. Do not fix anything found here.

- [ ] **Step 1: Confirm the dev instance, do not start a second one**

```bash
pgrep -f "crowbar-api" | while read p; do ps -o pid=,etime=,command= -p "$p"; done
pgrep -fl "tauri dev"
```

Expected: exactly one `crowbar-api` and one `tauri dev`. If zero, ask the user
before launching — a second instance orphans daemons that fight over one socket.
Rebuild the daemon from source and hot-restart it so Tasks 3–5 are live (the
dev sidecar is built from source by `make dev-desktop`).

- [ ] **Step 2: Generate the full-scale fixture**

```bash
./scripts/perf/gen-big-diff-fixture.sh "${TMPDIR}/crowbar-perf-fixture" full
git -C "${TMPDIR}/crowbar-perf-fixture" diff --shortstat -M main
```

Expected: roughly 1,000,000 insertions across ~400 files. Record the exact
`--shortstat` line; it goes in the results doc.

- [ ] **Step 3: Import the fixture into the dev app**

Drive the running app through the Tauri MCP bridge — import
`${TMPDIR}/crowbar-perf-fixture` as a project/repo, then create a workspace on
branch `perf/big-diff` with `main` as parent. Use
`mcp__tauri__webview_screenshot` after each step to confirm state; the app's own
UI is the source of truth for whether the import succeeded.

- [ ] **Step 4: Arm both instrumentation layers**

Daemon (through the dev socket, not the production one):

```bash
curl -s --unix-socket /var/folders/3k/dm5s9q9j6y7_6z69f5yyyvmw0000gn/T/crowbar-381fc6a71389fd95.sock \
  -X POST "http://localhost/v0/system/perf?enabled=true"
```

Frontend: the dev build arms `perfEnabled()` automatically via
`import.meta.env.DEV`, so `window.__measures` is already collecting. Confirm
with `mcp__tauri__webview_execute_js`:

```js
console.log(JSON.stringify({ armed: Array.isArray(window.__measures) }))
```

Read the result with `mcp__tauri__read_logs` — per the known bridge limitation,
`webview_execute_js` does not await promises and reports a timeout even when the
script ran, so log-and-read is the reliable path.

- [ ] **Step 5: Capture the review-pane-CLOSED scenario**

This is the scenario that reproduces the reported symptom. With the fixture
workspace active and the Branch Review pane **closed**:

1. Reset both rings (`POST /v0/system/perf?enabled=true` re-arms and clears;
   `window.__measures.length = 0` in the webview).
2. Touch the working tree to trigger `git-status-changed`:
   `echo "x" >> "${TMPDIR}/crowbar-perf-fixture/src/keep/untouched.ts"`.
3. Let the app sit for 30 seconds without interaction.
4. Read both rings.

Record: how many times `http.GET …/review/files` fired, its median and max
duration, the `git.diff` / `git.status` durations underneath it, and
`lock.read.hold` max. Compare the sum of daemon time against the frontend
`measure` durations in the same window — that ratio is the attribution answer.

- [ ] **Step 6: Capture the editor-latency scenario**

Open a source file from the fixture workspace, type a burst of characters, and
capture. Then repeat on an empty workspace. The delta between the two is the
collateral damage the spec's fourth budget row bounds.

- [ ] **Step 7: Capture the review-pane-OPEN scenario**

Open the Branch Review pane on the fixture and record `diff.open`, JS heap via
`mcp__chrome-devtools-mcp__take_heapsnapshot` if the bridge exposes it, and
whether the app becomes unresponsive. If it hangs hard enough to need a restart,
that is itself the result — record it and move on.

- [ ] **Step 8: Capture the two remaining budget baselines**

The spec budgets a daemon-RSS row and an entry-chunk row. Both need a
before-number now, or the Phase 3 comparison has nothing to compare against.

**Daemon RSS.** Sample the daemon's resident size before and during the
scenarios above:

```bash
pgrep -f "crowbar-api serve" | while read p; do
  ps -o pid=,rss=,command= -p "$p" | awk '{printf "%s RSS=%.1fMB\n", $1, $2/1024}'
done
```

Take one sample at idle, one immediately after the Step 5 tick storm, and one
after the Step 7 review-pane open. The spec's budget is "no diff-sized spike";
this establishes whether there currently is one.

**Entry chunk.** Measure the current gzip entry size so the Shiki cost in
Phase 3 is attributable:

```bash
cd web && ~/.bun/bin/bun run build
ENTRY=$(grep -oE 'assets/index-[A-Za-z0-9_-]+\.js' dist/index.html | head -1)
gzip -c "dist/$ENTRY" | wc -c
```

Record the byte count and the entry filename against the standing 840,000 B
budget.

- [ ] **Step 9: Write the results**

Create `docs/superpowers/specs/perf-phase0-attribution.md` with: the fixture's
exact shortstat, each scenario's numbers, and a plain verdict answering the
spec's three questions —

1. What fraction of a frozen interval is daemon request latency vs main-thread React work?
2. How long does `/review/files` actually take at 1M lines, and how often is it called?
3. How long is the repo read lock held, and are writers starving?

End with an explicit **Phase 1 ordering recommendation** (daemon-first or
frontend-first) justified by the numbers.

Then add the Phase 0 baseline rows to the table in
`docs/superpowers/specs/perf-baselines.md`, following its existing column
format, and note the measurement caveats honestly — if the window was hidden,
if a shim was used, if a number could not be obtained, say so. The existing doc
sets that standard; match it.

- [ ] **Step 10: Disarm and commit**

```bash
curl -s --unix-socket /var/folders/3k/dm5s9q9j6y7_6z69f5yyyvmw0000gn/T/crowbar-381fc6a71389fd95.sock \
  -X POST "http://localhost/v0/system/perf?enabled=false"

git add docs/superpowers/specs/perf-phase0-attribution.md docs/superpowers/specs/perf-baselines.md
git commit -m "docs(perf): phase 0 attribution results and baselines"
```

Leave the fixture in place — Phases 1–3 re-measure against it. Do not delete
`${TMPDIR}/crowbar-perf-fixture`.

---

## Definition of done

- [ ] `./scripts/perf/gen-big-diff-fixture.test.sh` passes.
- [ ] `cd api && go test ./... -race` passes.
- [ ] `cd web && ~/.bun/bin/bun run vitest run` passes; `~/.bun/bin/bun tsc --noEmit` clean.
- [ ] `make lint` reports no NEW findings against the pre-existing baseline (10 errors in `web/scripts/provision-tree-sitter.mjs`, 49 `react-hooks/exhaustive-deps` warnings).
- [ ] `docs/superpowers/specs/perf-phase0-attribution.md` answers all three attribution questions with numbers, and recommends a Phase 1 ordering.
- [ ] Daemon instrumentation is disarmed; no behaviour changed.
