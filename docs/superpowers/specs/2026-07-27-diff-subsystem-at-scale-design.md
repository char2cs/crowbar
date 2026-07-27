# Diff Subsystem at Scale — Design

Date: 2026-07-27
Status: approved for planning
Supersedes: the diff half of `2026-07-13-bare-metal-frontend-performance-design.md` (§P2)

## Problem

Crowbar degrades across the whole app once a workspace carries a large branch
diff — not only inside the Branch Review pane. The degradation is observed with
the review pane **closed** and no `diff://` tabs open, so it is not caused by the
diff renderer being on screen.

Target scale, stated by the product owner: **hundreds of changed files and
100k–1M changed lines**, with no perceptible slowdown anywhere in the app.

Three independent causes were identified by code inspection. Causes 1 and 3 are
always-on and explain the reported symptom; cause 2 explains why the review pane
itself is unusable at scale. None has yet been confirmed by measurement.

1. **Always-mounted O(files) cost.** `sidebar-carousel.tsx` mounts all four
   sidebar panels permanently (they are scrolled offscreen in an
   `overflow-x-scroll` strip — not unmounted, not `display:none`). `GitPanel` is
   therefore live on every tab, feeding `ChangedFilesTree`
   (`changed-files-tree.tsx:81`), which is **unvirtualized**: a recursive
   `renderNode` mounting one component per changed file plus one per folder.
   Each `GitFileItem` additionally opens its own `useSettingsStore` subscription
   (`git-status-file-item.tsx:37`), so a 400-file branch holds ~400 live store
   subscriptions that no one can see. The tree re-renders whenever the `files`
   reference changes — every `git-status-changed` tick carrying a real change
   (250ms debounce, `use-review-files-summary.ts:76`).

   By contrast `file-explorer-tree` and `git-history-list` are already
   virtualized; they are not implicated.

2. **O(total diff lines) materialization.** The review wire format is one JSON
   object per diff line (`domain/git/diff.go:7`), and the renderer mounts a full
   **Monaco editor per diff file**, each registered as a buffer in the workspace
   store (`use-diff-editor-buffer.ts:67`). Because the workspace registry
   persists on any `state.buffers` change, every diff file mutation schedules a
   structured-clone of the entire buffers array — diff content strings and token
   arrays included — into IndexedDB (`workspace-store-registry.ts:54-84`).

3. **O(diff size) daemon work on a recurring timer.** `/review/files` runs
   `git diff --numstat -M -z <ref>` (`file_summary.go:31`). Its *output* is
   O(file count) — which is what the code comment claims — but its *compute* is
   O(diff size): `--numstat` must run the diff algorithm over every file's
   content to count lines. `--name-status`, by contrast, compares tree blob
   SHAs and is O(tree).

   That endpoint is called from an always-mounted sidebar hook on every
   `git-status-changed` tick (`use-review-files-summary.ts:76`, 250ms debounce,
   up to ~4Hz during agent activity) — **with the review pane closed**, which is
   precisely the reported symptom.

   Three daemon properties amplify it:

   - `exec.Git` buffers all stdout into a `bytes.Buffer` and returns it as a
     `string` (`exec/exec.go:44`), so a 1M-line diff materializes ~40MB in
     daemon memory per invocation.
   - Non-network git has **no timeout**. Only `execNet` is bounded
     (`engine.go:105`), whose own comment documents that an unbounded
     subprocess under the mutex wedges every git operation on the clone.
   - `lockRepoRead` is an RWMutex read lock (`engine.go:88`), and reads hold it
     for the whole operation. Measured at 200 files / 100k lines — a tenth of
     target scale — hold time is **mean ~180-230ms, max ~520ms**, against the
     100ms budget below.

     Go's RWMutex is writer-preferring, so in principle a waiting writer (a
     background origin-sync fetch, a terminal-side commit) blocks every read
     arriving behind it, turning one long hold into a queue. **This
     amplification is NOT measured.** `BenchmarkBranchReview_GetFiles_Contention`
     was written to demonstrate it and does not: at 16 concurrent readers both
     `lock.read.wait` and `lock.write.wait` are 0.0ms, because the writer
     finishes before readers reach their acquisition. Treat starvation as a
     documented `sync.RWMutex` property and a plausible amplifier, not as an
     established cause. The long holds are measured and are reason enough to
     act; the queue behaviour is not evidence we have.

   Causes 1 and 3 are both always-on, both scale with diff size, and both are
   live with the review pane closed. Which dominates is **not established** —
   see Measurement.

## Goal

Substantially improve the performance of the diff mechanism and the git branch
model, **without losing any capability** and **without sacrificing performance
anywhere else in the app**, using `@pierre/diffs` as the renderer.

Concretely: branch review and every other diff surface stay responsive at
hundreds of files and 1M changed lines; client memory is constant with respect
to diff size; and the daemon does no diff-sized work on a timer.

## The law

Every decision in this document follows from three invariants. A change that
violates any of them is wrong regardless of how well it benchmarks on a small
diff.

1. **No client-side structure is O(total diff lines).**
2. **No always-mounted component is O(changed files).**
3. **No recurring-timer request is O(diff size), and no git output that can be
   streamed is buffered.**

The third invariant constrains the daemon, not just the frontend. It was added
after review found that the original two would have permitted — in fact
required — new endpoints that made the daemon's contribution to the freeze
worse.

## Non-goals

- Remote PR creation or review. This is the local branch-vs-parent surface only.
- Changing the review-thread data model, its REST API, or its WebSocket stream.
  Threads are re-hosted, not redesigned.
- Replacing Monaco as the *editor*. Monaco stays for file editing; it stops
  being used for diffs.

## Approach

Adopt **`@pierre/diffs`** (`diffs.com`, v1.2.12, Apache-2.0 — compatible with
Crowbar's AGPL-3.0 side) as the sole diff renderer, and feed it through a
**windowed data path** that never materializes the whole diff.

`@pierre/diffs` alone does not meet the bar: its `CodeViewItem` carries a fully
parsed `FileDiffMetadata` per file, with `additionLines` / `deletionLines` as
real JS string arrays. It virtualizes *rendering* and offloads *highlighting* to
a worker pool; it does not solve *materialization*. That is this design's job.

Two library behaviours were verified against the published package before
committing to this approach:

- **Annotations render in light DOM.** `renderAnnotation` output is
  `createPortal`'d with a `slot="…"` attribute and projected into the shadow
  root (`dist/react/utils/renderDiffChildren.js:24`). Slotted content is styled
  by the *document* stylesheet, so Tailwind utilities, CSS-variable design
  tokens, and `@/components/ui/*` all work verbatim inside thread UI. The shadow
  root encapsulates only the code rows.
- **Heights are computable without content.** `computeEstimatedDiffHeights`
  reads only `hunks[].splitLineCount / unifiedLineCount / collapsedBefore` and
  `fileDiff.isPartial` — never line content. A file can therefore occupy correct
  scroll space before its patch has been fetched.

## Architecture

Five layers: the daemon foundation, three data layers cheapest-first, then the
renderer that consumes them.

### Layer 0 — Daemon foundation

Every layer above depends on the daemon being able to produce diff data without
holding it. Four changes, all in `internal/engine/git`:

**Streaming exec.** A new `exec.GitStream(ctx, dir, args…) (io.ReadCloser, …)`
alongside the existing buffering `exec.Git`. Callers that scan line-by-line —
the outline scanner, the patch endpoint, the search scanner — consume the pipe
and never hold the diff. `exec.Git` stays for the many callers whose output is
genuinely small (status, log, rev-parse); this is an addition, not a
replacement.

**Timeout on non-network git.** Non-network invocations get a bounded context
(`gitOpTimeout`, 60s) so a pathological diff cannot hold the repo mutex
indefinitely. A timeout is reported as an explicit error, matching `execNet`'s
existing classification.

**`--numstat` off the hot path — split committed from dirty.**

`FileSummaries` currently runs `git diff --numstat -M -z <ref> --`, which
diffs the ref against the **working tree**, so every tick pays a numstat over
the entire branch diff.

The obvious fix — cache it under `(baseSHA, headSHA, worktreeDirtyHash)` — does
**not** work, and it is worth recording why: during agent churn the working tree
changes on every tick, so that key changes on every tick too. Permanent cache
miss, on exactly the workload the freeze was reported under. Measured on the 1M
fixture:

| query | cost | recomputed |
|---|---|---|
| `numstat <ref> --` (today, ref→worktree) | 2173ms | every tick |
| `numstat <ref> HEAD --` (committed only) | 2856ms | **only on commit** |
| `numstat <ref> -- <dirty paths>` | **87ms** | every tick |
| `status --porcelain` | 163ms | already paid today |

(Machine under load; the ratios are the point, not the absolutes.)

So the split is:

- **Committed part** — `git diff --numstat -M -z <ref> HEAD --`, keyed on
  `(ref, headSHA)`. Expensive, but invalidated only by a commit or a ref move.
- **Dirty part** — the set of dirty paths comes from `Status()`, which
  `GetFiles` already calls; ± counts for exactly those paths come from
  `git diff --numstat -M -z <ref> -- <paths…>`. O(dirty files), not O(branch).
- **Merge**, with the dirty result winning per path, because a file that is both
  committed-changed and dirty needs its true ref→worktree count, not a sum.

A tick during churn then pays the restricted numstat (~87ms) instead of the full
one (~2173ms) — a ~25× reduction on the path that actually froze.

`--name-status` stays per-request (O(tree), ~51ms) so the file *set* is never
stale.

**Cancellation over queueing.** A review request superseded by a newer tick is
cancelled through its context rather than left to complete behind the lock. The
frontend's debounce already coalesces bursts; this makes the daemon side honour
the same intent.

The outline and search caches (Layers 2 and below) share the same
`(baseSHA, headSHA, worktreeDirtyHash)` key, so one ref change invalidates all
derived diff state coherently.

### Layer 1 — Index (eager, O(files))

`GET /v0/workspaces/:ws/review/files` exists (`file_summary.go:19`) and is
restructured per Layer 0: `--name-status` runs on every request (O(tree)), while
the `--numstat` ± counts are served from the `(baseSHA, headSHA,
worktreeDirtyHash)` cache and recomputed only when that key changes. Per file:
path, oldPath, status, additions, deletions, uncommitted, staged.

This is the **only** payload fetched eagerly. It paints the sidebar tree and
seeds one `CodeView` item per file.

This endpoint is the single hottest path in the system — it is what a
recurring git tick hits with the review pane closed — so invariant 3 applies to
it most sharply. Its cost on a cache hit must be independent of diff size.

### Layer 2 — Outline (deferred, O(hunks))

New `GET /v0/workspaces/:ws/review/outline`.

Returns per file an array of hunk shapes — `{oldStart, oldLines, newStart,
newLines}` — obtained by scanning `git diff -M <forkPoint>` through
`exec.GitStream` and keeping **only the `@@` header lines**. Daemon memory is
O(hunks); the diff itself is never held. Same context setting (`-U3`) as the
patch endpoint, so shapes match what the client will actually render.

Cached under the shared `(baseSHA, headSHA, worktreeDirtyHash)` key, so it is
computed once per ref change rather than per request.

Fetched immediately *after* the index, never blocking first paint: the file list
renders from Layer 1, and heights sharpen when the outline lands. It is not on
a recurring timer.

**Density guard.** A file whose hunk count exceeds `MAX_OUTLINE_HUNKS_PER_FILE`
(1000) keeps its **first 1000 hunks** and is marked `isPartial`. (This differs
from the earlier draft, which said "a single synthetic hunk sized from its
±counts" — keeping real leading geometry is strictly more useful and the
implementation follows it.)

**Consequence for the client:** a partial file's geometry is a **lower bound**,
so height estimation must top it up from the Layer 1 ±counts rather than trust
the hunk list alone. Phase 3 must handle this or a shotgun file will under-
reserve scroll space.

**Measured payload — 2× the original estimate.** On the 1M-line fixture the
outline is **2.28 MB** across 38,604 reported hunks (39,104 present, 500 capped
from one shotgun file). That is ~59 bytes of JSON per hunk, not the ~28 this
spec first assumed. Still O(hunks) and still bounded, but large enough that it
justifies keeping the outline off the first-paint path — which is what Layer 2
already specifies — and worth revisiting if the fetch proves slow over the unix
socket.

### Layer 3 — Patch (on demand, O(one file))

New `GET /v0/workspaces/:ws/review/patch?path=<p>`, `Content-Type: text/plain`,
returning the unified patch for **one** file.

The backend already produces exactly this text and currently spends CPU
destroying it into per-line JSON (`internal/diff/diff.go:22`). This endpoint
stops doing that.

`?maxLines=<n>` caps the response (client default 20,000); over the cap the
server truncates at a hunk boundary and sets `X-Crowbar-Diff-Truncated: true`,
which the client surfaces as a "showing first N of M changed lines — show all"
affordance. Requesting "show all" refetches that file with no cap, and the file
is pinned in the window until the user scrolls past it.

### Layer 4 — Windowed render

One `CodeView` for the whole review. Items are created from the index — one per
file — with a placeholder `FileDiffMetadata` carrying hunk shapes from the
outline and no content.

A viewport controller:

- fetches + `parsePatchFiles` for files entering a **lookahead band**
  (viewport ± 1.5 screens) and `updateItem`s the real metadata;
- reverts files leaving the **eviction band** (viewport ± 4 screens) to
  placeholders — deliberately wider than the lookahead band so a file oscillating
  at the boundary is not repeatedly refetched;
- enforces a hard budget — `MAX_MATERIALIZED_FILES` (40) and
  `MAX_MATERIALIZED_LINES` (60,000) — evicting least-recently-visible first.

Consequence: client memory is a constant. A 2k-line diff and a 1M-line diff hold
the same working set.

`tokenizeMaxLength` and `tokenizeMaxLineLength` guard minified/generated files
with pathological line lengths.

## Threads

Threads are re-hosted, not redesigned. `review-api.ts`, the `/threads` REST
surface, the WebSocket stream, and `review-thread-item.tsx` (411 loc) are all
**kept as-is**.

**Rendering.** `ReviewThread` → `DiffLineAnnotation<ReviewThread>`:

```ts
{ side: thread.side === 'old' ? 'deletions' : 'additions',
  lineNumber: thread.lineNumber,
  metadata: thread }
```

`renderAnnotation(a) => <ReviewThreadItem thread={a.metadata} />`. Because
annotations are slotted into light DOM, this renders with the app's existing
styling and needs no shadow-DOM workaround.

**Creation.** `enableLineSelection` + `onSelectedLinesChange` yields
`{start, end, side}`, which maps directly onto the existing `openThread({filePath,
line, startLine, endLine, side})`. The hover "+" affordance uses
`renderGutterUtility`. The entire Monaco view-zone layer
(`use-review-comment-layer.tsx`) is deleted with no capability loss — it is
strictly a downgrade of what the annotation API provides natively.

**Unmaterialized files.** A thread on a file outside the window has nowhere to
render. Two compensations:

- Per-file thread counts render on the file header from the threads store, which
  is already live independently of the diff (`useWorkspaceThreadsStream`).
- Navigating to a thread forces materialization of its file, then
  `scrollTo({type: 'line', id, lineNumber, side})`.

## Diff search

Client-side find-in-diff is structurally impossible when only a window is
materialized. It is replaced by a server-side search, not dropped.

`GET /v0/workspaces/:ws/review/search?q=&regex=&case=&limit=`

The daemon scans `git diff -M <forkPoint>` through `exec.GitStream` line by
line, tracking the current file and old/new line numbers from `@@` headers. It
never holds the diff in memory — O(diff bytes) time, O(hits) memory. Search is
user-initiated, never on a timer, and superseded queries are cancelled by
context rather than queued.

Response: `{hits: [{path, side, lineNumber, preview}], truncated: bool}`, capped
at `limit` (default 200, max 1000).

Client: the search bar debounces 200ms into this endpoint and renders a hit list.
Selecting a hit materializes the target file and calls
`scrollTo({type: 'line', …})`. Next/prev navigate the hit list. This is a
different interaction from the current in-place highlight sweep, and the change
is deliberate.

## Capability parity

No capability may be lost. Every feature the current diff stack provides, and
where it lands:

| Capability | Disposition |
|---|---|
| Unified / split view toggle | Native — `splitHeight` / `unifiedHeight`, `diffStyle` |
| Syntax highlighting | Native — Shiki, via worker pool |
| Inline review threads | `renderAnnotation`, existing `review-thread-item.tsx` reused |
| Create thread on a line range, old/new side | `enableLineSelection` + `onSelectedLinesChange` |
| Resolve / reply / edit / delete thread | Unchanged — existing REST + WS surface |
| Expand / collapse all files | `CodeViewItem.collapsed` |
| Jump to file from sidebar | `scrollTo({type: 'item'})` |
| Expand unchanged context | Native — `expandUnchanged`, `expansionLineCount` |
| Image diffs | `git-diff-image.tsx` retained unchanged |
| GitHub compare / commit link | Header, via `renderHeaderMetadata` |
| Commit diff, working-tree diff | Same `CodeView`, different item source |
| Find in diff | **Changed** — server-side search + `scrollTo`, approved as the honest replacement at this scale |
| Stage / unstage hunk | **Verify** — `DiffAcceptRejectHunkConfig`, fallback custom gutter control; backend unchanged |
| Whitespace toggle | **Verify** — rendering via `unsafeCSS`; if `-w` semantics are wanted instead, the patch endpoint takes `?ignoreWhitespace=` |

The two **Verify** rows are the only unconfirmed mappings in this design. Both
are resolved in Phase 3 *before* the corresponding old code is deleted, and both
have a stated fallback that does not block the phase.

## What gets deleted

Big-bang replacement. No legacy path, no feature flag, no dual renderer —
Crowbar is pre-production and carries no users whose state must be migrated.

Frontend:

- `components/diff/git-diff-editor-stack.tsx` (963 loc)
- `components/diff/git-diff-editor-surface.tsx`
- `components/diff/git-diff-viewer.tsx`
- `components/diff/use-review-comment-layer.tsx`
- `components/diff/use-hunk-staging-zones.tsx`
- `components/diff/diff-search-bar.tsx`, `use-diff-search.ts`,
  `diff-search-context.ts`, `utils/diff-search.ts` (replaced by server search)
- `utils/diff-editor-content.ts` (397 loc of serializers)
- `utils/git-diff-cache.ts` (replaced by the patch LRU)
- `utils/diff-viewer-scale.ts`, `utils/working-tree-multi-diff.ts`
- **`hooks/use-diff-editor-buffer.ts`**

The last one is load-bearing: with it gone, diff files stop entering
`state.buffers` entirely, which removes the IndexedDB layout-write amplification
at the root rather than tuning it.

Backend:

- `DiffLine` / `FileDiff.Lines` on the review path, and the parser that builds
  them, once no endpoint serves them.

Retained: `review-thread-item.tsx`, `review-api.ts`, `git-diff-image.tsx` (image
diffs are not text and keep their own viewer), the hunk-staging backend
(`hunk_patch.go`, `hunk_id.go`).

## Other diff surfaces

Big bang means every diff surface moves to the same renderer, not just branch
review:

- **Working-tree file diff** (`diff://staged|unstaged/<path>` buffers) — a
  `CodeView` with a single item, fed by the patch endpoint scoped to that file.
- **Commit diff** (`getCommitDiff`) — a `CodeView` whose items come from the
  commit's file list, patches fetched per file through the same window.
- **Hunk staging** — the backend stage/unstage calls are unchanged; the per-hunk
  affordance is re-hosted on `renderGutterUtility` / the library's
  `DiffAcceptRejectHunkConfig`. **This mapping is to be verified in Phase 3
  before the old zone layer is deleted**; if the library's hunk actions cannot
  carry stage/unstage semantics, the fallback is a custom gutter control, which
  the same API supports.

## Sidebar cost

The frontend half of Phase 1, independent of the renderer and shipped before it
(the daemon half is Layer 0):

- Virtualize `ChangedFilesTree` — flatten the folder tree into a row model and
  render through TanStack `useVirtualizer`, exactly as
  `use-file-explorer-visible-rows.ts` already does for the file explorer.
- Lift `useSettingsStore` out of `GitFileItem` to the tree root and pass
  `compactGitStatusBadges` down as a prop, collapsing N subscriptions to one.

Carousel panels stay mounted. Prior work established that `display:none`
dormancy is load-bearing and that unmounting fights the scroll-snap model; the
correct fix is making each panel's *content* O(viewport), which virtualization
achieves.

`workspace-tree.tsx` and `agent-chats-panel.tsx` are also unvirtualized but are
O(workspaces) / O(chats), not O(diff). Out of scope here.

## Measurement

There is no workspace available that exhibits the target scale, so the first
work item builds one.

**Fixture.** A generated scratch repo with a realistic mix: ~400 changed files,
several 100k-line files, one ~1M-line total branch, binaries, renames, a
minified single-line file, and a deleted directory. Committed as a generator
script, not as data.

**Instrumentation.** The existing `lib/perf/instrumentation.ts` spans
(`markStart`/`markEnd`, `window.__perfLog`). New frontend spans: `diff.index`,
`diff.materialize`, `diff.scroll`. New daemon-side timing on `/review/files`,
`/review/outline`, `/review/patch`, `/review/search`, plus repo-mutex wait time
and git subprocess wall time.

**Attribution comes before any fix.** Two independent causes are credible
(unvirtualized sidebar tree; O(diff-size) daemon work on a tick) and neither has
been measured. Phase 0 must answer, on the fixture with the review pane closed:

- what fraction of a frozen interval is spent in daemon request latency versus
  main-thread React work;
- how long `/review/files` actually takes at 1M lines, and how often it is
  called;
- how long the repo read lock is held, and whether writers are starving.

The fix order follows those numbers. If one cause dominates overwhelmingly, the
other is still fixed — both violate the law — but it stops being urgent.

**Budgets**, measured in a production build under `make dev-desktop` (never the
production install, per the isolation rule), 3 runs, median:

| Metric | Budget |
|---|---|
| Review pane open → first rows painted (1M-line fixture) | < 500ms |
| Sustained scroll through the 1M-line fixture | ≥ 55fps, no frame > 32ms |
| JS heap after scrolling the full 1M-line diff end to end | < 250MB, and flat — no growth across repeat passes |
| Editor keystroke latency with the fixture workspace active, review pane closed | indistinguishable from an empty workspace |
| `/review/files` on a git tick, cache hit, 1M-line fixture | independent of diff size (see note) |
| Daemon RSS while serving the 1M-line fixture | no diff-sized spike — bounded, not ~40MB/call |
| Repo read-lock hold time, any single acquisition | < 100ms |
| Entry chunk (gzip), after Shiki | ≤ 840,000 B (the standing budget) |

Three rows prove the law rather than the feel: JS heap flatness (invariant 1),
`/review/files` independence from diff size (invariant 3), and daemon RSS
(invariant 3). The rest prove it feels right.

**Budget correction after Phase 1 (measured).** The `/review/files` row
originally read "< 50ms". That number was picked before anything was measured
and is **not reachable**, so it has been replaced by the property that actually
matters. Phase 1 took the call from 489.8 ms/op to ~94 ms/op (`lock.read.hold`
mean 203.9 → 33ms, max 370.9 → 45.9ms, comfortably inside its 100ms budget).
The residue is not diff work — it is process-spawn floor:

| spawn | cost | why it stays |
|---|---|---|
| `merge-base` ×2 | ~27ms | `mergeBaseWithBase` probes origin/base AND local base and takes whichever is closest to HEAD; preferring origin blindly re-inflates the diff by un-pushed local base commits |
| `status --porcelain` | ~27ms | the dirty set the split depends on |
| `diff --name-status` | ~27ms | the file set; must never be stale |
| `rev-parse HEAD` | ~13ms | the exact cache key |

Every one is O(1) in diff size, and `git rev-parse HEAD` — which does no real
work — costs ~13ms on its own, so ~13ms is simply what a git subprocess costs on
this machine under load. Getting under 50ms would mean removing spawns that are
each load-bearing for correctness. **Invariant 3 is satisfied; the absolute
figure is spawn-bound and the budget was wrong, not the implementation.**

The cheapest remaining win is dropping `rev-parse` by reading `# branch.oid`
out of `status --porcelain=v2 --branch`, which is already fetched and discarded.
That is ~13ms and was deliberately deferred: it means touching the status
parser, a hot shared path with a white-box test suite pinned to line-based
parsing.

**No-collateral-damage gate.** The objective explicitly includes not
regressing the rest of the app. Before each phase merges, the existing perf
baselines in `perf-baselines.md` — terminal echo, warm workspace switch, entry
chunk — are re-run on a *small* repo and must not regress.

## Testing

- **Go.** Table tests for the outline scanner, patch truncation, and the search
  scanner against fixture patches. For Layer 0: the ref-key cache's
  invalidation (a ref change busts it, an unrelated fs event does not),
  `exec.GitStream` not buffering (assert on a bounded read, not on wall time),
  the non-network timeout firing as a classified error, and cancellation of a
  superseded request actually killing the subprocess. Per the standing rule,
  every backend bug found gets a black-box `TestRegression_*` in `api/tests`.
  No timing-based synchronization in any test — block on real signals.
- **Web.** Unit tests in `web/src/__tests__/` mirroring `web/src/`, `@/` imports.
  Priority targets: the window controller's materialize/evict decisions (pure
  function over `{viewport, items, budget}`), the thread ↔ annotation mapping in
  both directions, and the placeholder-height builder.
- **Live.** A designed manual pass in the Tauri app against the fixture before
  any claim that this works — including creating a thread on a file that is
  materialized, scrolling far enough to evict it, and returning to find the
  thread intact.

## Risks

**Shiki is a third highlighting stack** alongside Monaco and tree-sitter.
Mitigate with `shiki/core` plus lazily-registered languages and the worker pool;
gate on the entry-chunk budget above. If the budget cannot be met, the fallback
is `preferredHighlighter: 'shiki-js'` with a reduced language set — diffs
degrade to plain text for exotic languages rather than the bundle regressing.

**Estimated heights drift** as real content replaces placeholders, moving the
scrollbar under the user. Mitigate by anchoring scroll position to the top
visible file across re-measure, and by making the outline exact rather than
±-count-derived (which is why Layer 2 exists at all).

**Eviction thrash** if the eviction band is too tight relative to scroll
velocity. The band is a tuned constant, validated against the fixture's sustained
scroll metric.

**Hunk staging may not map cleanly** onto the library's accept/reject config —
called out in "Other diff surfaces" with a fallback and an explicit
verify-before-delete ordering.

**The committed-counts cache can serve stale ± counts** if it is keyed on
something that fails to change when the committed diff does. The key is
`(ref, headSHA)`: a commit, amend, reset, rebase or ref move all change
`headSHA`, and a change of base changes `ref`. Working-tree edits deliberately
do NOT invalidate it — they are covered by the dirty-path query instead, which
is what makes the split work at all.

The failure mode to guard is a committed change that leaves `headSHA` equal,
which should be impossible by construction. A stale count in the sidebar is a
visible correctness bug, so this is the one place in the design where
correctness outranks the perf win: when in doubt, recompute.

**Attribution may contradict the plan.** Phase 0 could show the freeze is
dominated by something not listed here at all. The phase order is written to be
re-orderable on that evidence rather than defended.

## Phasing

Each phase is independently shippable and independently verifiable, and each
gets its own implementation plan and PR.

0. **Fixture, baselines, attribution.** Generator script, frontend and daemon
   instrumentation, recorded before-numbers for every budget, and a definitive
   answer to which cause dominates the freeze. Nothing is fixed in this phase.
   It is the phase the rest of the plan is ordered by.

1. **Always-on cost — the freeze.** Both always-on O(diff) paths, ordered by
   what Phase 0 found:
   - *Daemon:* `--numstat` off the tick path behind the ref-keyed cache,
     `exec.GitStream`, non-network git timeout, context cancellation of
     superseded requests.
   - *Frontend:* virtualize `ChangedFilesTree`, lift the per-row
     `useSettingsStore` subscription to the tree root.

   This phase targets the reported symptom directly and ships without any
   renderer change.

2. **Diff API.** Outline, patch, and search endpoints on the Layer 0
   foundation, with tests. Old per-line JSON still served; nothing deleted yet,
   so the frontend keeps working throughout.

3. **Renderer.** `CodeView` + window controller + threads on annotations; all
   other diff surfaces moved; old stack and the per-line JSON deleted. The
   hunk-staging mapping is verified before the zone layer is removed.

Phase 1 is where the freeze is expected to end. Phases 2–3 are what make the
1M-line bar hold and what deliver the `@pierre/diffs` renderer.
