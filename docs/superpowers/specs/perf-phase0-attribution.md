# Phase 0 — Attribution Results

Date: 2026-07-27
Spec: `2026-07-27-diff-subsystem-at-scale-design.md`
Plan: `../plans/2026-07-27-diff-perf-phase-0-attribution.md`

Phase 0 changed no behaviour. It built the fixture and the instrumentation, and
answers the three attribution questions the spec's phase ordering depends on.

## Fixture

`scripts/perf/gen-big-diff-fixture.sh <dest> full`, deterministic (identical
tree SHAs across runs into different paths), 15.1s to build:

```
407 files changed, 1004001 insertions(+), 165250 deletions(-)
total hunks: 39,104   max hunks in one file: 1,500   patch size: 46.3 MB
```

Composition: 300 scattered files (every 10th line changed), 100 dense
whole-file rewrites, 2 × 420k-line monsters, 1 shotgun file (1,500 hunks),
1 minified file (657k-char single line), binary add + modify, rename, delete,
and an untouched control.

The first version of this generator was wrong in a way worth recording: it
rewrote every line of every file, collapsing each to a single hunk — 404 hunks
for a million changed lines. That shape silently under-tests the outline
scanner, hunk separators, expand-unchanged, and the per-file density guard. The
suite now fails against that old generator (`too few hunks overall: 43`), so the
regression is caught rather than described.

## Q1 — Daemon latency vs main-thread React work

**Partially answered. State the gap plainly: the in-app frozen-interval split
was not measured.** Doing so requires the fixture imported as a live workspace,
which would mean a 45 MB repo and async provisioning in the shared dev home
while two sibling sessions were working in it. Both sides were measured
independently instead.

**Frontend, measured** (`changed-files-tree.scale.test.tsx`):

| Files | Rows in DOM | DOM nodes | Nodes/file |
|---|---|---|---|
| 500 | 500 | 6,292 | 12.6 |

Cost is strictly linear — 4× the files produces exactly 4× the rows. Plus one
`useSettingsStore` subscription per row (`git-status-file-item.tsx:37`).

**Frontend, confirmed live in the running app.** All four sidebar carousel
panels report `display: flex`, `visibility: visible`, `contentVisibility:
visible`; panels 2 and 3 are offscreen but fully in the DOM. None is dormant.
So `GitPanel` — and its unvirtualised tree — renders on every tab, not just the
Git tab. This was the load-bearing structural assumption behind cause 1 and it
holds.

**Daemon, measured** — see Q2.

**Honest comparison:** at 400 files the frontend holds ~5,000 permanently
mounted DOM nodes and ~400 store subscriptions, re-rendered whenever the `files`
reference changes. The daemon spends ~490ms *per call* at a tenth of target
scale, at up to 4Hz. In absolute milliseconds per tick the daemon cost is the
larger of the two by roughly an order of magnitude. That is an inference from
two separately-measured halves, not a single measured split.

## Q2 — `/review/files` cost and call rate

**Answered.**

`BenchmarkBranchReview_GetFiles_Scale`, 200 files / 100k lines — **a tenth of
target scale**:

```
BenchmarkBranchReview_GetFiles_Scale/200files_100k_lines-10   5   489816625 ns/op

  lock.read.hold    calls=10  total=2039.2ms  mean=203.9ms  max=370.9ms
  git.diff          calls=10  total=1673.5ms  mean=167.4ms  max=307.9ms
  git.merge-base    calls=10  total= 407.1ms  mean= 40.7ms  max= 77.9ms
  git.status        calls=5   total= 364.8ms  mean= 73.0ms  max=122.4ms
  lock.read.wait    calls=10  total=   0.0ms  mean=  0.0ms  max=  0.0ms
```

**490 ms/op against a 50 ms budget, at a tenth of scale.**

Raw git on the full 1M-line fixture, isolating the spec's cause-3 claim:

| Command | median | class |
|---|---|---|
| `git diff --name-status -M -z main` | 51ms | O(tree) |
| `git diff --numstat -M -z main` | **345ms** | O(diff size) |
| `git status --porcelain` | 44ms | O(worktree) |
| `git diff -M main` (full patch) | 477ms | O(diff size), 46.3 MB |

**`--numstat` is ~6.7× `--name-status`.** `file_summary.go:15` calls the pair
"O(file count)" — true of *output size*, false of *compute*. `GetFiles` runs
name-status + numstat + `Status()` (`branch_review_files.go:30,46`).

**Call rate:** `use-review-files-summary.ts:76` refetches on every
`git-status-changed`, 250ms trailing debounce, from an always-mounted sidebar
hook — up to ~4Hz during agent activity, **with the review pane closed**. At
4Hz × 490ms that exceeds a 100% duty cycle on git alone.

All numbers are on a *clean* worktree. The reported symptom occurs while agents
churn the tree, which makes `Status()` and the working-tree diff strictly more
expensive. These are a floor.

## Q3 — Repo read-lock hold, and writer starvation

**Half answered, and the other half is a correction to the spec.**

**Hold times, measured:**

| Scenario | mean | max | budget |
|---|---|---|---|
| Uncontended (1 reader) | 203.9ms | 370.9ms | 100ms |
| 16 concurrent readers | 180.0ms | 522.9ms | 100ms |

**2–5× over budget at a tenth of target scale.**

**Writer starvation was NOT reproduced.** `BenchmarkBranchReview_GetFiles_-
Contention` was written specifically to demonstrate it. With 16 concurrent
readers and a real engine-level writer (`StageFile`, which takes `lockRepo`),
both `lock.read.wait` and `lock.write.wait` measure **0.0ms on every run**. The
writer consistently acquires and releases before readers reach their own
acquisition — `GetFiles` resolves the diff ref first, so readers spend their
first ~25ms in `merge-base` before ever asking for the lock.

An earlier version of that benchmark was worse than useless: its "writer"
shelled out through `benchGitRun`, bypassing the engine mutex entirely, so it
could never have measured contention regardless of scale. That is fixed; the
result did not change.

Forcing the overlap would need either a sleep (banned in this repo) or a hook
inside the lock itself (which measures the instrument, not the system). **The
spec has been corrected** to stop asserting starvation as an established cause
and to mark it a documented `sync.RWMutex` property and plausible amplifier.
The long holds are measured and are reason enough to act on their own.

## Verdict and Phase 1 ordering

**Both causes are real. The daemon dominates in absolute cost per tick and
should be fixed first.**

1. **Daemon first.** `--numstat` off the tick path behind the ref-keyed cache is
   the single highest-value change in the program: it removes ~345ms of the
   ~490ms per call, on the endpoint a timer hits ~4Hz with the review pane
   closed. `exec.GitStream`, the non-network timeout, and context cancellation
   ride along.
2. **Frontend second.** Virtualising `ChangedFilesTree` and lifting the per-row
   `useSettingsStore` subscription removes ~5,000 permanently-mounted DOM nodes
   and ~400 subscriptions at 400 files. Real, smaller, and independently
   shippable.

Nothing found here contradicts the spec's three causes. The one correction is
Q3's starvation claim.

## Instrumentation added

- `api/internal/perf` — bounded race-free ring, disarmed by default (one atomic
  load when off).
- `git.<subcommand>`, `lock.read/write.wait/hold` samples.
- `middleware.Timing()` bucketing by gin route template; `GET`/`POST
  /v0/system/perf`. Verified end-to-end against a real daemon on an isolated
  socket: `http.GET /v0/health` recorded at 0.073ms.
- `window.__measures` — a measure-only frontend ring, because `__perfLog`'s
  500-entry ring floods with Event Timing entries and evicts measures within
  seconds. The unbounded INP push was capped at the same time.

## Reproducing

```bash
./scripts/perf/gen-big-diff-fixture.sh "${TMPDIR}crowbar-perf-fixture" full

cd api && go test -tags noEmbed -run='^$' -bench='GetFiles_Scale' \
  -benchtime=5x -timeout 900s ./internal/app/usecases/branchreview/...

cd web && ~/.bun/bin/bun run vitest run \
  src/__tests__/features/git/components/changed-files-tree.scale.test.tsx
```

## Entry chunk baseline (pre-Shiki)

`vite build` at `a3053446`:

| | raw | gzip | budget |
|---|---|---|---|
| `assets/index-Bdrrbyrx.js` | 692,175 B | **243,804 B** | 840,000 B |

**596 KB of gzip headroom for Shiki** — substantially more than the spec's risk
section assumed, since the entry has shrunk well below the 481,307 B recorded at
the end of the bare-metal program. Shiki's bundle cost is unlikely to be the
binding constraint in Phase 3; the worker-pool and lazy-language work is still
worth doing for runtime cost, not bundle size.

Measured with `vite build` directly rather than `bun run build`, because the
latter runs `tsc` first and a sibling session's untracked
`sidebar-peek.test.tsx` currently fails `TS6133`. Unrelated to this work.

## Not yet measured

Carried into Phase 1 rather than quietly dropped:

- The in-app frozen-interval split (Q1) — needs the fixture as a live workspace.
- Daemon RSS under load, and the JS heap curve for the review pane at 1M lines.
- Writer starvation, if it can be measured without a timing hack.
