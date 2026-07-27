# Diff Perf Phase 1 — Always-On Cost (the freeze) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove both always-on costs that scale with diff size, so a workspace carrying a large branch diff stops degrading the whole app.

**Architecture:** Daemon first, per the Phase 0 attribution. `FileSummaries` stops running a whole-branch `--numstat` on every git tick by splitting committed counts (cached on `(ref, headSHA)`) from dirty-path counts (recomputed, O(dirty)). A streaming exec and a non-network timeout land alongside. Then the frontend: virtualise `ChangedFilesTree` and lift its per-row store subscription.

**Tech Stack:** Go 1.x (daemon), React 19 + TanStack Virtual + Vitest (web).

## Global Constraints

- Spec: `docs/superpowers/specs/2026-07-27-diff-subsystem-at-scale-design.md`. Attribution: `docs/superpowers/specs/perf-phase0-attribution.md`. Phase 1 only.
- **Phase 0 numbers are the bar.** `GetFiles` 489.8ms/op at 200 files/100k lines; `lock.read.hold` mean 203.9ms / max 370.9ms; `ChangedFilesTree` 500 files → 500 rows / 6,292 DOM nodes.
- **Correctness outranks the perf win on ± counts.** A wrong count in the sidebar is a visible bug. When in doubt, recompute.
- Multiple sessions share this worktree's git index. **Subagents must NOT run any git write command.** Leave changes in the working tree; the coordinating session commits path-limited.
- Do not touch files owned by sibling sessions: `api/internal/app/usecases/workspace/workspace.go`, `web/src/components/layout/{ide-shell,sidebar-carousel,drag-ghost,sidebar-peek}.tsx`, their tests, `api/tests/integration/files/home_files_test.go`.
- No timing-based synchronization in tests. No sleeps, no polling, no `Eventually`.
- Go: `go test -tags noEmbed -race`; build with `-tags noEmbed` (plain `go build ./...` fails on a pre-existing missing embed artifact). Lint: `golangci-lint run --build-tags noEmbed`.
- Web: `~/.bun/bin/bun` (NOT on default PATH). Never `bunx tsc`; use `~/.bun/bin/bun tsc --noEmit`.
- Web tests mirror `web/src/` under `web/src/__tests__/`, `@/` imports, kebab-case component files.

---

### Task 1: Streaming exec + non-network git timeout

**Files:**
- Modify: `api/internal/engine/git/internal/exec/exec.go`
- Create: `api/internal/engine/git/internal/exec/stream.go`
- Create: `api/internal/engine/git/internal/exec/stream_test.go`

**Interfaces:**
- Consumes: `perf.Record` (already wired in `run`).
- Produces:
  - `exec.GitStream(ctx, dir string, args ...string) (io.ReadCloser, func() error, error)` — the reader streams stdout; the second return is a `Wait` closing over the process, returning a `*GitError` on non-zero exit. Callers MUST close the reader.
  - `exec.GitOpTimeout` — the bound applied to non-network invocations.
- Consumed by Task 2 and by Phase 2's outline/patch/search endpoints.

**Why:** `exec.Git` buffers all stdout into a `bytes.Buffer` and returns a `string`, so a 1M-line diff materialises ~46MB per call. And non-network git has no timeout at all — only `execNet` is bounded (`engine.go:105`), whose own comment documents that an unbounded subprocess under the repo mutex wedges every git operation on that clone.

- [ ] **Step 1: Write the failing tests**

Create `api/internal/engine/git/internal/exec/stream_test.go`:

```go
package exec_test

import (
	"bufio"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

func TestGitStream_StreamsStdout(t *testing.T) {
	dir := initRepo(t)

	r, wait, err := exec.GitStream(context.Background(), dir, "status", "--porcelain=v2", "--branch")
	require.NoError(t, err)
	defer func() { _ = r.Close() }()

	var lines int
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		lines++
	}
	require.NoError(t, sc.Err())
	require.NoError(t, wait())
	assert.Positive(t, lines)
}

func TestGitStream_WaitReportsNonZeroExit(t *testing.T) {
	dir := t.TempDir() // not a git repo

	r, wait, err := exec.GitStream(context.Background(), dir, "log")
	require.NoError(t, err)
	_, _ = io.Copy(io.Discard, r)
	_ = r.Close()

	err = wait()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "log")
}

// The point of streaming is that a consumer can stop early without the whole
// output ever being materialised. Closing the reader must not deadlock.
func TestGitStream_CloseBeforeEOFDoesNotBlock(t *testing.T) {
	dir := initRepo(t)
	for i := range 200 {
		writeAndCommit(t, dir, "f.txt", strings.Repeat("x", 100)+string(rune('a'+i%26)))
	}

	r, wait, err := exec.GitStream(context.Background(), dir, "log", "-p")
	require.NoError(t, err)

	buf := make([]byte, 64)
	_, _ = r.Read(buf)
	require.NoError(t, r.Close())
	_ = wait() // may report a signal kill; must return rather than hang
}

func TestGitStream_ContextCancelStopsProcess(t *testing.T) {
	dir := initRepo(t)
	ctx, cancel := context.WithCancel(context.Background())

	r, wait, err := exec.GitStream(ctx, dir, "log")
	require.NoError(t, err)
	cancel()
	_, _ = io.Copy(io.Discard, r)
	_ = r.Close()
	_ = wait()
}
```

Add a `writeAndCommit(t *testing.T, dir, name, content string)` helper to this file if the package's existing test helpers do not already provide one — read `exec_test.go` first and reuse what is there.

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd api && go test -tags noEmbed ./internal/engine/git/internal/exec/... -run GitStream -v
```

Expected: FAIL — `undefined: exec.GitStream`.

- [ ] **Step 3: Implement `GitStream`**

Create `api/internal/engine/git/internal/exec/stream.go`. Requirements, all load-bearing:

- Use `exec.CommandContext` with the same `GIT_OPTIONAL_LOCKS=0` environment the buffering path uses — read `run`/`runInner` and reuse the env construction rather than duplicating it.
- Return `cmd.StdoutPipe()` wrapped so `Close()` also kills the process and drains, so an early-closing consumer cannot leak a git process or deadlock on a full pipe buffer.
- Capture stderr into a bounded buffer (cap it — stderr on a broken repo can be large) so `wait()` can build a `*GitError` with a useful message.
- `wait()` must be safe to call exactly once and must return `*GitError` on non-zero exit, matching `RequireSuccess`'s error shape.
- Record a `perf.Record("git."+subcommandName(args), …)` sample in `wait()`, covering the full streamed duration — do not leave streamed calls invisible to the instrumentation the previous phase added.
- Do NOT route `GitStream` through `runWithLockRetry`: that helper retries a whole invocation, which is meaningless once bytes have been handed to a consumer. Document that in a comment.

- [ ] **Step 4: Add the non-network timeout**

In `exec.go`, define `GitOpTimeout = 60 * time.Second` and apply it in `runInner` via `context.WithTimeout`, mirroring `execNet`'s classification so a deadline is reported as an explicit timeout error rather than the opaque exit -1 a killed subprocess produces.

Write a test asserting a context already past its deadline yields a classified timeout rather than a bare non-zero exit.

- [ ] **Step 5: Run tests to verify they pass**

```bash
cd api && go test -tags noEmbed -race -count=1 ./internal/engine/git/...
cd api && go build -tags noEmbed ./...
cd api && golangci-lint run --build-tags noEmbed ./internal/engine/git/...
```

Expected: all PASS, 0 lint issues. Run the FULL git package suite — `exec` is a hot shared path.

- [ ] **Step 6: Prove it actually streams**

A `GitStream` that secretly buffers would pass every test above. In the scratchpad (touching no repo file), write a throwaway probe that runs `GitStream` with `log -p` against `${TMPDIR}crowbar-perf-fixture`, reads the first 1KB, closes, and samples the process's peak RSS. Compare against `exec.Git` on the same command. Report both numbers. If RSS is comparable, the implementation is buffering and is wrong.

---

### Task 2: Split committed and dirty ± counts

**Files:**
- Modify: `api/internal/engine/git/internal/diff/file_summary.go`
- Create: `api/internal/engine/git/internal/diff/summary_cache.go`
- Create: `api/internal/engine/git/internal/diff/summary_cache_test.go`
- Modify: `api/internal/engine/git/review_files.go`
- Modify/extend: `api/internal/engine/git/internal/diff/file_summary_test.go`

**Interfaces:**
- Consumes: nothing from Task 1 (keep these independent so they can land in either order).
- Produces: `FileSummaries` keeps its exact signature — `FileSummaries(ctx, repoPath, ref) ([]gitdomain.ReviewFileSummary, error)`. Behaviour is unchanged; only cost changes. No caller is modified.

**Why (measured, not assumed).** `FileSummaries` runs `git diff --numstat -M -z <ref> --`, which diffs the ref against the **working tree** — a whole-branch numstat on every tick. On the 1M fixture that is ~2173ms. Caching it under a key that includes a working-tree hash does **not** help: during agent churn the tree changes every tick, so the key changes every tick. The split below is what makes the cache hit under churn.

**The design:**

| part | query | keyed on | cost (1M fixture) |
|---|---|---|---|
| file set | `git diff --name-status -M -z <ref> --` | not cached | ~51ms |
| committed counts | `git diff --numstat -M -z <ref> HEAD --` | `(repoPath, ref, headSHA)` | ~2856ms, only on commit |
| dirty counts | `git diff --numstat -M -z <ref> -- <dirty paths>` | not cached | ~87ms |

Merge with **dirty winning per path**: a file that is both committed-changed and dirty needs its true ref→worktree count, not the sum of two diffs.

- [ ] **Step 1: Write the failing correctness tests first**

The risk here is wrong counts, not slowness. Before any caching, write an equivalence test that pins the new implementation to the old one across every shape that matters. Create/extend `file_summary_test.go`:

```go
// TestFileSummaries_MatchesUnsplitReference asserts the committed/dirty split
// produces byte-identical summaries to a single ref->worktree numstat, which is
// what the implementation did before the split. Any divergence here is a
// user-visible wrong ± count in the sidebar.
func TestFileSummaries_MatchesUnsplitReference(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, repo string)
	}{
		{"clean tree", func(t *testing.T, repo string) {}},
		{"dirty tracked file", /* modify a committed file without staging */},
		{"staged file", /* modify + git add */},
		{"untracked file", /* new file, never added */},
		{"committed-changed AND dirty", /* the case a naive sum gets wrong */},
		{"rename", /* git mv + commit */},
		{"binary file", /* non-UTF8 bytes; numstat reports "-" */},
		{"deleted file", /* git rm + commit */},
		{"file deleted in worktree only", /* rm without git rm */},
	}
	// For each: build the repo, compute want := unsplitReference(ctx, repo, ref)
	// (a test-local helper running the ORIGINAL single numstat), compute
	// got := FileSummaries(ctx, repo, ref), and require.Equal(want, got).
}
```

Write `unsplitReference` as a small test-local function reproducing the pre-split query. Keeping the old algorithm alive in the test is the whole point — it is the oracle.

Then cache-behaviour tests in `summary_cache_test.go`:

```go
func TestSummaryCache_HitsWhenOnlyWorktreeChanged(t *testing.T)   // the churn case: no recompute of committed counts
func TestSummaryCache_MissesAfterCommit(t *testing.T)             // headSHA moved
func TestSummaryCache_MissesAfterRefChange(t *testing.T)          // different base
func TestSummaryCache_MissesAfterReset(t *testing.T)              // headSHA moved backwards
func TestSummaryCache_IsPerRepo(t *testing.T)                     // two repos do not share entries
func TestSummaryCache_BoundedSize(t *testing.T)                   // does not grow without limit
```

Assert cache hits by counting invocations through an injected exec function — **not** by timing.

- [ ] **Step 2: Run the tests to verify they fail**

```bash
cd api && go test -tags noEmbed ./internal/engine/git/internal/diff/... -run 'FileSummaries_Matches|SummaryCache' -v
```

Expected: the equivalence test may pass against the current implementation (it is the oracle for behaviour that has not changed yet); the `SummaryCache` tests MUST fail with `undefined`. If the equivalence test passes now, that is correct and desirable — it locks in current behaviour before you refactor.

- [ ] **Step 3: Implement the split**

In `file_summary.go`, replace the single numstat with:

1. `parseNameStatusZ` from `--name-status -M -z <ref> --` (unchanged, gives the file set).
2. Committed counts from `--numstat -M -z <ref> HEAD --`, via the cache.
3. Dirty paths: accept them as a parameter rather than shelling `git status` again — `ReviewFiles`'s caller already has the status. **Read `branch_review_files.go:46` first**: `mergeWorkingTree` calls `u.git.Status(...)` after `ReviewFiles`. Reordering so the status is fetched once and threaded down is part of this task; do not add a second `status` call.
4. Dirty counts from `--numstat -M -z <ref> -- <paths…>`, chunked if the path list would exceed `ARG_MAX` (a 400-file dirty set with long paths can).
5. Merge, dirty winning per path.

- [ ] **Step 4: Implement the cache**

`summary_cache.go`: a bounded, mutex-guarded map keyed by `(repoPath, ref, headSHA)` holding `map[string]numCount`. Cap entries (say 32) with oldest-eviction — a long session across many workspaces must not grow it without bound. No TTL: the key is exact, so a stale entry is impossible by construction, and a TTL would only add nondeterminism.

- [ ] **Step 5: Run tests to verify they pass**

```bash
cd api && go test -tags noEmbed -race -count=1 ./internal/engine/git/... ./internal/app/usecases/branchreview/...
cd api && golangci-lint run --build-tags noEmbed ./internal/engine/git/...
```

Expected: all PASS including the equivalence oracle, 0 lint issues.

- [ ] **Step 6: Prove the win against the Phase 0 baseline**

```bash
cd api && go test -tags noEmbed -run='^$' -bench='GetFiles_Scale' -benchtime=5x \
  -timeout 900s ./internal/app/usecases/branchreview/...
```

Baseline to beat: **489.8 ms/op**, `lock.read.hold` mean 203.9ms / max 370.9ms.
Target: **< 50 ms/op** on a warm cache, `lock.read.hold` < 100ms.

The benchmark's loop calls `GetFiles` repeatedly without committing, so iterations 2..N are the warm-cache churn case this task exists to fix. Report before/after for both metrics. If the win is not there, say so rather than adjusting the benchmark.

---

### Task 3: Single-flight the review-files read

**Files:**
- Modify: `api/internal/app/usecases/branchreview/branch_review_files.go`
- Create: `api/internal/app/usecases/branchreview/single_flight_test.go`

**Interfaces:**
- Consumes: `GetFiles` from Task 2.
- Produces: no signature change. Concurrent `GetFiles` calls for the same workspace share one computation.

**Why:** the frontend debounces at 250ms, but a burst still produces overlapping in-flight requests, each doing full git work under the read lock. Sharing one computation is strictly better than cancelling the loser, because every caller still gets a correct, current answer.

- [ ] **Step 1: Write the failing test**

```go
// TestGetFiles_SingleFlightsConcurrentCallers asserts that N concurrent
// GetFiles calls for one workspace perform ONE underlying computation.
// Counted through an injected git engine, never timed.
func TestGetFiles_SingleFlightsConcurrentCallers(t *testing.T) {
	// Use a fake git engine whose ReviewFiles blocks on a channel the test
	// controls, and counts invocations. Release the channel only after all N
	// callers are provably registered — use a sync.WaitGroup the fake signals
	// on entry, so the test blocks on a real signal rather than a sleep.
	// Assert: invocations == 1, and all N callers receive equal results.
}

func TestGetFiles_SeparateWorkspacesDoNotShare(t *testing.T)
func TestGetFiles_ErrorPropagatesToAllWaiters(t *testing.T)
func TestGetFiles_NewCallAfterCompletionRecomputes(t *testing.T)
```

- [ ] **Step 2: Run to verify failure**

```bash
cd api && go test -tags noEmbed -race ./internal/app/usecases/branchreview/... -run SingleFlight -v
```

Expected: FAIL — invocations == N, not 1.

- [ ] **Step 3: Implement**

Add a `golang.org/x/sync/singleflight.Group` keyed by workspace id, or a hand-rolled equivalent if that module is not already a dependency (check `go.mod` first; do not add a dependency without noting it). Key on `wsID`. Ensure errors propagate to every waiter and that a completed flight does not serve a subsequent call.

- [ ] **Step 4: Verify**

```bash
cd api && go test -tags noEmbed -race -count=1 ./internal/app/usecases/branchreview/...
cd api && golangci-lint run --build-tags noEmbed ./internal/app/usecases/...
```

---

### Task 4: Virtualise the changed-files tree

**Files:**
- Modify: `web/src/features/git/components/changed-files-tree.tsx`
- Modify: `web/src/features/git/components/status/git-status-file-item.tsx`
- Modify: `web/src/__tests__/features/git/components/changed-files-tree.scale.test.tsx`
- Modify: `web/src/__tests__/features/git/components/changed-files-tree.test.tsx` (only if the DOM shape it asserts changes)

**Interfaces:**
- Consumes: nothing.
- Produces: `ChangedFilesTree` keeps its exact props — `{ files: GitDiff[]; repoPath?: string; onFileOpen: (filePath: string) => void }`.

**Why:** 500 files → 500 rows / 6,292 DOM nodes, mounted on every sidebar tab because the carousel never unmounts a panel (confirmed live). Plus one `useSettingsStore` subscription per row (`git-status-file-item.tsx:37`).

- [ ] **Step 1: Flip the characterisation test to the target behaviour**

`changed-files-tree.scale.test.tsx` currently asserts the defect and says in its own doc comment that this step must happen. Rewrite both cases:

```ts
it('renders a bounded number of rows regardless of file count', () => {
  const { container } = render(
    <ChangedFilesTree files={makeFiles(500)} repoPath="/repo" onFileOpen={() => {}} />,
  )
  const { fileRows } = countRows(container)
  // Bounded by the virtual window, not the input.
  expect(fileRows).toBeLessThan(100)
  expect(fileRows).toBeGreaterThan(0)
})

it('row count does not scale with file count', () => {
  const small = render(<ChangedFilesTree files={makeFiles(100)} repoPath="/repo" onFileOpen={() => {}} />)
  const large = render(<ChangedFilesTree files={makeFiles(400)} repoPath="/repo" onFileOpen={() => {}} />)
  const ratio = countRows(large.container).fileRows / countRows(small.container).fileRows
  expect(ratio).toBeLessThan(1.5) // was exactly 4.0 before virtualisation
})
```

Note for the implementer: jsdom reports zero-height containers, so TanStack Virtual may render only one row unless the scroll element is given a size. Read `web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-visible-rows.ts` and its tests — that tree is already virtualised and has already solved this. Follow it rather than inventing a second approach.

- [ ] **Step 2: Run to verify failure**

```bash
cd web && ~/.bun/bin/bun run vitest run src/__tests__/features/git/components/changed-files-tree.scale.test.tsx
```

Expected: FAIL — 500 rows rendered, ratio exactly 4.

- [ ] **Step 3: Flatten the tree to a row model**

Replace the recursive `renderNode` with a flatten step producing a flat array of `{ type: 'folder' | 'file', depth, key, … }`, honouring `collapsedFolders` (a collapsed folder contributes its own row and none of its descendants). Keep `buildGitFolderTree` — only the rendering changes. Memoise the flatten on `[files, collapsedFolders]`.

- [ ] **Step 4: Virtualise the rows**

Render through `useVirtualizer`, mirroring `use-file-explorer-visible-rows.ts`. The scroll container is currently the `ScrollArea` in `git-panel.tsx:108` — check whether the virtualizer needs to own the scroll element, and if `ScrollArea` must be replaced or given a ref, do it in `changed-files-tree.tsx` rather than reshaping `GitPanel`.

- [ ] **Step 5: Lift the per-row store subscription**

Remove `useSettingsStore` from `GitFileItem` and pass `compactGitStatusBadges` as a prop from the tree root, which subscribes once. `GitFileItem` is used elsewhere — **grep for every call site** and update them all; make the prop required so a missed call site is a type error rather than a silent default.

- [ ] **Step 6: Verify**

```bash
cd web && ~/.bun/bin/bun run vitest run src/__tests__/features/git/
cd web && ~/.bun/bin/bun tsc --noEmit
cd web && ~/.bun/bin/bun run lint
```

Expected: all PASS; tsc clean except a sibling session's untracked `sidebar-peek.test.tsx` (`TS6133`), which is not ours.

- [ ] **Step 7: Live verification in the running Tauri app**

Non-negotiable: this is a visible UI change. The dev app is already running — reuse it, never launch a second (stacked launches orphan daemons fighting one socket; check with `pgrep -f crowbar-api` first).

Via the Tauri MCP bridge: open a workspace with changed files, switch the sidebar to Git, and confirm with `mcp__tauri__webview_screenshot` that the tree renders correctly — folder nesting, indentation, ± badges, the uncommitted badge, click-to-open, and folder collapse/expand all still work. Then re-run the carousel probe from Phase 0 and confirm the Git panel's DOM node count is now bounded rather than proportional to file count:

```js
(() => {
  const panels = Array.from(document.querySelector('[data-sidebar-carousel]').children);
  return panels.map((p, i) => ({ index: i, domNodes: p.querySelectorAll('*').length }));
})()
```

Report the before (Phase 0: 6,292 nodes at 500 files) and after.

---

## Definition of done

- [ ] `cd api && go test -tags noEmbed -race ./...` passes.
- [ ] `cd web && ~/.bun/bin/bun run vitest run` passes; `bun tsc --noEmit` clean but for the sibling file.
- [ ] `golangci-lint run --build-tags noEmbed ./...` — no NEW findings.
- [ ] `GetFiles` benchmark: **< 50 ms/op** warm (from 489.8), `lock.read.hold` **< 100ms** (from mean 203.9 / max 370.9).
- [ ] `ChangedFilesTree`: row count bounded, ratio < 1.5 at 4× files (from exactly 4.0).
- [ ] The ± count equivalence oracle passes on all nine repo shapes.
- [ ] Live Tauri verification done, with a screenshot, and the DOM-node before/after reported.
- [ ] No-collateral-damage gate: existing `perf-baselines.md` M-series re-run on a small repo, no regression.
