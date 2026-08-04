# Tier A denominator — `crowbar-core`, measured from the React source

Companion to QUEUE.md's "The real Tier B denominator" — same rigour, different
target. Spec §16 Phase 3: *"Tier A (`core`, `proto`, `client`, theme tokens —
gated by ported tests) and Tier B."* `crowbar-core`'s `Cargo.toml` claims **"all
Crowbar domain logic: git model, diff algebra, keymap resolution, settings
schema, file-tree model, workspace scoping, review threads."** Today it is
`color.rs` + `lib.rs`, 349 lines, 100% coverage over a crate that holds none of
that domain logic (QUEUE.md, 2026-08-03).

**Status: COMPLETE.** All seven named areas plus theme tokens measured,
committed incrementally per the interruption protocol as each finished. The
headline denominator is at the end of this file. **Amended 2026-08-04 (P3.69)
to add a liveness verdict to every row — see the next section for why, and
"Liveness" below each area's file table for the per-row verdicts.**

Method, per area: (1) where the logic lives today in `web/src`, with line
counts: (2) what if anything is already ported into `crowbar-proto` /
`crowbar-client`; (3) whether it is expressible with zero reference to a view,
store or framework (gpui-free, D2); (4) the bucket — Tier A core / Phase 4
state / already done / presentation / out of scope; (5) existing test files
and case counts, since §16 gates Tier A on ported tests.

---

## 0. Liveness verification (P3.69) — why this exists, method, controls

**This survey shipped without ever asking whether anything reaches the files
it lists.** P3.67 took six file names straight from §1's table below and
dispatched them for porting. Two — `utils/normalize-diff.ts` and
`utils/diff-buffer-path.ts` — have **zero non-test importers**: nothing in the
shipping app can reach them. They were ported, tested to 100% coverage, and
merged before anyone checked (`native/QUEUE.md`, "I dispatched two DEAD
files"). The user's standing directive is *"only port components that ARE IN
USE on the production app."* Line counts and shape do not answer that
question; only tracing reachability does. This section is the fix: every row
in every table below now carries a verdict.

### The four verdicts

- **LIVE** — something reachable from the app entry point (`web/src/main.tsx`
  → its boot-time calls, or → `routeTree.gen.ts` → the mounted route tree)
  uses it, with no dialog/pane/route selection required beyond whatever the
  app already shows by default. Evidence is a concrete import/call chain.
- **CONDITIONAL** — reachable only once the user (or a build flag) selects a
  *named* state the port has to model as a distinct thing: opening a specific
  dialog (Settings), a specific pane type (commit/branch review diff), a
  right-click context-menu action, a settings toggle, or a build-time flag
  (`VITE_USE_MOCK`). Still in scope for porting — the condition is a state the
  port's own UI has to reproduce, not a reason to skip the logic.
- **DEAD** — nothing reaches it by any import spelling. Do not port (and, for
  the two already-ported cases, do not silently keep pretending otherwise —
  see `native/mapping/core-git.md`).
- **UNCERTAIN** — none needed this pass. Every one of the 90 rows below
  resolved to a definite verdict with a concrete file/line/chain citation; see
  the tally at the end of this section.

**A note on where this draws the LIVE/CONDITIONAL line, and why it differs
from `native/mapping/liveness-audit.md`'s convention for UI surfaces.** That
audit (component primitives — buttons, dialogs, rows) treated "open the
Settings dialog" or "click a commit in History" as **LIVE**, reasoning that an
always-available UI action is ordinary use, the same bucket as any button
click. This survey's rows are *logic functions*, not UI primitives, and for a
logic function the question that actually matters is different: **does this
code run without the user doing anything beyond what the default IDE chrome
already does, or does it require navigating to a distinct, nameable state
first?** Opening Settings, opening a commit-diff pane, or right-clicking a
file are each a state the *port* has to build as an explicit thing (a dialog
open/closed flag, a pane-kind enum, a context-menu model) — exactly the
category CONDITIONAL exists to flag, per this item's brief. So this survey
uses a narrower LIVE than the surface audit does. Both are internally
consistent; they are answering slightly different questions, and the
divergence is called out here rather than silently reconciled, per this
document's own standing practice (see Finding 5, below the headline table,
for the earlier instance of the same kind of tension).

### Method

For every file named in every "Where it lives" table below: search
`web/src` (excluding `__tests__/`, stated per the brief's requirement) for
every spelling that could import it —

1. `@/`-aliased absolute (`@/features/git/utils/foo`)
2. relative, any depth (`./foo`, `../foo`, `../../utils/foo`)
3. bare re-export through an intermediate `index.ts` or shim file (the
   `features/file-explorer/{lib,stores,utils}/*.ts` one-line
   `export * from '../file-explorer/...'` shims, per Finding 6 below, are
   exactly this case — a naive same-directory grep misses the real importer
   sitting one level deeper)
4. dynamic `import()` (`review-code-view.tsx` is lazy-loaded this way from
   `review-diff-tab.tsx`)

then, for whatever imports it, trace **one more hop** toward the app root to
find the condition (if any) that gates that importer, using the same
four-spelling search at each hop. This was done two ways, cross-checked
against each other:

- **Manual, per-file `grep`**, tracing each hop by hand and reading the
  gating code (a `Tabs defaultValue=`, an `isOpen` prop's store default, a
  `useEffect` vs. an event-handler `.getState()` call) — this is what
  produced every verdict and chain cited below.
- **An automated compile-time import-reachability BFS** (`reach.py`,
  scratch script, not committed — see below), seeded at `main.tsx` **and**
  `routeTree.gen.ts` (TanStack Router's codegen; a route can be
  root-reachable without `main.tsx` naming it directly), that resolves all
  four spellings above via `os.path` resolution (alias, relative, directory
  `index.*`) and follows every `from`/`import(` edge — 654 files visited
  (642 of 653 non-test `.ts`/`.tsx` files in `web/src`, plus CSS `@import`
  chains, which the same regex happens to also catch). This is a **second,
  independent test of the DEAD verdicts specifically**: a file absent from
  this reachable set cannot be LIVE or CONDITIONAL by any import spelling,
  full stop, which is a stronger claim than "I did not personally find an
  importer." Of the 653 files, only 11 are unreachable; 3 of those 11 are
  rows in this survey (`normalize-diff.ts`, `diff-buffer-path.ts`,
  `diff-search.ts` — see below); the other 8 are outside this survey's scope
  (ambient `.d.ts` files that need no import to be real, a Web Worker loaded
  via `new Worker(new URL(...))` syntax the regex doesn't parse — confirmed
  by hand that it does have a real caller — a dev-only oracle-extraction
  script, and one more dead component (`diff-review-header.tsx`) noted here
  but not chased further, out of scope for this item).
- **Known limitation of the BFS, stated rather than hidden:** it cannot see
  `new Worker(new URL('./x.ts', import.meta.url))` (regex matches `from`/
  `import(`, not `new Worker`) or any other non-static-analyzable loading
  path. None of this survey's 90 rows load that way — checked — but the tool
  is not a universal liveness oracle, only an import-graph one.

### Controls

A method that reports everything LIVE, or everything DEAD, is not evidence
(per this project's standing rule). Four controls, two in each direction:

| control | expected | method's result |
|---|---|---|
| `lib/branch-action.ts` — already independently confirmed live in `native/QUEUE.md`'s own P3.67 correction table (1 importer) | LIVE | **LIVE** — `branch-section.tsx:7`, which mounts unconditionally inside `GitPanel` (`sidebar-carousel.tsx:168`, itself always-rendered regardless of the active sidebar tab — see below) |
| `lib/workspace-scope-url.ts` — the survey's own §6 already calls `workspaceBase` "one of the most load-bearing functions in this whole survey" | LIVE | **LIVE** — 20 non-test importers across every git/file/agent API module; re-confirmed independently rather than taken on the original prose's word |
| `utils/normalize-diff.ts` — already independently confirmed dead in `native/QUEUE.md`'s P3.67 correction table (0 importers) | DEAD | **DEAD** — 0 importers by any spelling; absent from the BFS reachable set |
| `utils/diff-buffer-path.ts` — same table, same finding | DEAD | **DEAD** — 0 importers by any spelling; absent from the BFS reachable set |

All four controls returned the expected verdict. A fifth, unplanned control
fell out of the work itself: the BFS's reachable-set check and the manual
per-file grep were run independently and agreed on all 90 rows without a
single conflict requiring arbitration — the strongest evidence available that
neither method has a systematic blind spot the other shares.

### One methodological finding that nearly reproduced this item's own mistake

Sidebar panels (`sidebar-carousel.tsx`) are an **off-viewport scroll-snap
carousel**, not conditionally-mounted tabs: `<GitPanel />` and
`<FileExplorerTree />` are both rendered unconditionally at all times (just
panned off-screen via CSS when another tab is active — confirmed by reading
the JSX directly, not inferred). A first pass assumed "select the Git tab"
was a CONDITIONAL gate, by analogy with `liveness-audit.md`'s treatment of
`checkbox`/`textarea` (gated on opening a commit popover *inside* the git
panel). That would have been wrong: the git panel itself, and everything it
renders by default (`branch-section.tsx`, `changed-files-tree.tsx`), executes
on every render with a workspace open, regardless of which carousel page is
scrolled into view. Caught by reading `sidebar-carousel.tsx:126-169` directly
instead of assuming from the "carousel" framing. This is the same class of
error the brief warns about — a plausible-sounding gate that is not actually
what the code does — just caught before it produced a wrong verdict rather
than after.

### Verdict tally (all 90 rows, every "Where it lives" table below)

| verdict | rows | lines (of rows with a stated line count) |
|---|---|---|
| **LIVE** | **61** | **5,751** (+2 rows with no stated line count) |
| **CONDITIONAL** | **25** | **3,696** (+3 rows with no stated line count) |
| **DEAD** | **4** | **138** |
| **UNCERTAIN** | **0** | — |
| **Total** | **90** | **9,585** |

(Row and line totals include every file table across all seven areas plus
theme tokens, using the exact per-row line counts already printed in each
"Where it lives" table — including full-file counts for mixed files like
`review-code-view.tsx`'s 1,179 and `patch-window.ts`'s 244, which is *larger*
than the doc's own "~8,048 lines surveyed" headline figure below, because
that figure deliberately used reduced embedded-region estimates for mixed
files instead of whole-file counts. Both figures are legitimate; they are
answering different questions, and both are kept rather than silently
reconciled into one number that would overclaim precision it doesn't have.)

See "The headline denominator" → "Liveness reconciliation" at the end of this
file for what this means for the ~3,170-line Tier A core figure specifically.

---

## 1. Git model

### Where it lives

`web/src/features/git/`, non-component, non-store:

| file | lines | shape |
|---|---|---|
| `types/git-types.ts` | 78 | `GitFile`, `GitStatus`, `GitCommit`, `GitDiffLine`, `GitDiff`, `GitHunk`, `GitRemote`, `GitStash`, `GitBlame*` — hand-written DTOs |
| `types/git-diff-types.ts` | 41 | `ParsedHunk` (dead — see below), `MultiFileDiff`, two component-prop interfaces |
| `lib/branch-action.ts` | 49 | `resolveBranchAction` — pure decision function |
| `utils/git-status-to-changed-files.ts` | 45 | status → sidebar-tree projection, dedup |
| `utils/review-file-summary-to-git-diff.ts` | 41 | review-summary → sidebar-tree projection |
| `utils/build-git-folder-tree.ts` | 57 | flat `GitFile[]` → folder tree |
| `utils/git-diff-helpers.ts` | 11 | `getFileStatus` (logic) + `getImgSrc` (presentation, mixed in one file) |
| `utils/normalize-diff.ts` | 38 | defends against a stale-persisted-shape bug (see below) |
| `utils/diff-buffer-path.ts` | 24 | parses the synthetic `diff://…` buffer-path scheme |
| `utils/diff-search.ts` | 72 | regex search over reconstructed unified diff text |
| `utils/git-diff-cache.ts` | 122 | in-memory TTL cache, keyed `repo:file:staged` |
| `lib/patch-window.ts` | 244 | windowed-review materialisation planner (pure by design) |
| `components/diff/review-code-view.tsx` | 1,179 | **component file, but 368 of the 1,179 lines (roughly lines 93–460) are pure functions**: `partitionReviewFiles`, `buildPlaceholderFileDiff`, `distributeContext`, `buildPlaceholderHunks`, `buildTailHunk`, `trimToPatchCap`, `reserveAtMost`, `parseSingleFilePatch`, `patchCacheKey` |

**822 lines of `utils/`+`lib/`, plus ~368 pure lines embedded in a 1,179-line
component** = **~1,190 lines of git/diff logic outside stores and components proper.**

### Liveness

Method and controls: see §0. `GitPanel` mounts unconditionally inside
`sidebar-carousel.tsx:168` regardless of the active carousel page (confirmed
by reading the JSX — this is *not* gated on "Git tab selected"); its default
tab (`Tabs defaultValue="changes"`, `git-panel.tsx:59`) renders
`ChangedFilesTree` immediately. Opening a commit/branch-review pane, by
contrast, is a distinct pane-kind the port must model — CONDITIONAL.

| file | verdict | evidence |
|---|---|---|
| `types/git-types.ts` | LIVE | Used directly by `git-status-to-changed-files.ts`, `build-git-folder-tree.ts`, `branch-section.tsx`, `changed-files-tree.tsx` — all on the default-mounted `GitPanel` chain. (Also flows into the CONDITIONAL review path; the type itself is exercised by both.) |
| `types/git-diff-types.ts` | CONDITIONAL | Its two component-prop interfaces are used only by `git-diff-image.tsx` (image-diff viewer, only reachable inside `review-code-view.tsx`); `MultiFileDiff` is used by `review-api.ts`'s `getReview`/`getReviewFiles` (review-pane-only); `ParsedHunk` is declared, never constructed anywhere — dead type within a CONDITIONAL file. |
| `lib/branch-action.ts` | LIVE | Control — see §0. `branch-section.tsx:7`, unconditional inside default-mounted `GitPanel`. |
| `utils/git-status-to-changed-files.ts` | LIVE | `use-sidebar-changed-files.ts` → `git-panel.tsx` (default-mounted). **Correction to `native/QUEUE.md`'s own P3.67 table**: that table states 2 non-test importers; independently re-grepped here and found only 1 (`use-sidebar-changed-files.ts`; the second hit in the original count was almost certainly the test file). Immaterial to the verdict — still LIVE — but stated because this item's whole point is not taking prior counts on faith. |
| `utils/review-file-summary-to-git-diff.ts` | CONDITIONAL | `use-review-files-summary.ts` → `review-diff-tab.tsx`, mounted only inside a commit-diff/branch-review pane (`commit-diff-pane.tsx`, `branch-review-pane.tsx`). |
| `utils/build-git-folder-tree.ts` | LIVE | `changed-files-tree.tsx:9-10`, the default tab content of the default-mounted `GitPanel`. |
| `utils/git-diff-helpers.ts` | CONDITIONAL | Sole importer `git-diff-image.tsx`, rendered only for image files inside `review-code-view.tsx` (CONDITIONAL, see below). |
| `utils/normalize-diff.ts` | **DEAD** | Control — see §0. 0 non-test importers; absent from the BFS-reachable set. Already ported; see `native/mapping/core-git.md` §2 and the note in "The headline denominator" below. |
| `utils/diff-buffer-path.ts` | **DEAD** | Control — see §0. 0 non-test importers; absent from the BFS-reachable set. Already ported; see `native/mapping/core-git.md` §3. |
| `utils/diff-search.ts` | **DEAD** — new finding, not previously flagged | 0 non-test importers for `computeDiffMatches`/`MAX_DIFF_MATCHES`/`DiffSearchMatch`/`DiffSearchResult` anywhere in `web/src`; absent from the BFS-reachable set. The natural consumer, `review-search-bar.tsx` (imported by the live `review-diff-tab.tsx`), never imports this module — it implements search some other way. **Not ported** (it was never dispatched — it sat in the separate `crowbar-diff`-logic 316-line bucket, not the 3,170-line Tier A core bucket — so there is no Rust module to correct for this one, only the survey's own prose in §2 below, which discussed it as real logic without ever checking liveness). |
| `utils/git-diff-cache.ts` | CONDITIONAL | Sole importer `editor-app-store.ts`; `gitDiffCache.invalidate(...)` fires only inside the save-file action, i.e. only once a file buffer is open (not the default new-tab state). |
| `lib/patch-window.ts` | CONDITIONAL | `planWindow` used only by `review-code-view.tsx` (below). |
| `components/diff/review-code-view.tsx` | CONDITIONAL | Dynamically imported (`import('./diff/review-code-view')`) from `review-diff-tab.tsx`, mounted only via `commit-diff-pane.tsx` / `branch-review-pane.tsx` (a commit-diff or branch-review pane open). The ~368 embedded pure-function lines this survey counts toward Tier A core execute only when this component does. |

### ‼️ Finding: `features/git/utils/git-diff-parser.ts` does not exist

§10.1 names it explicitly: *"unless the daemon already returns unified diff, in
which case `features/git/utils/git-diff-parser.ts` ports directly and no
algorithm is needed. **Check first.**"* I checked. No file by that name exists
anywhere in `web/src`, and no hand-rolled unified-diff string parser exists
either. What actually happens:

1. The daemon **does** return structured diff data — `GitDiff.lines:
   GitDiffLine[]` — never a raw patch string for the sidebar/status path. See
   `normalize-diff.ts`'s whole reason for existing: a persisted tab's stale
   payload once arrived with `lines` missing, so the fix is a defensive
   reshape, not a parser.
2. Where a **raw patch string** does need parsing (the Branch Review surface,
   which streams patch text lazily), the app delegates to the third-party
   `@pierre/diffs` library's `parsePatchFiles` — see `parseSingleFilePatch` in
   `review-code-view.tsx:400`, a 10-line wrapper around it, not a hand-rolled
   parser.

So §10.1's conditional resolves to "no algorithm needed" — correctly — but the
*mechanism* it names is wrong: there is no `git-diff-parser.ts` to port, and the
real patch-string parsing the app does today is a third-party dependency
(`@pierre/diffs`, being replaced wholesale by `crowbar-diff` per §5.2) rather
than portable first-party code.

### What is genuine, portable git-model logic

- **`resolveBranchAction`** (`lib/branch-action.ts`) — a precedence-ordered pure
  decision function (`commit` > `resolve` > `pull-request` > `merge` >
  `sync-only`) over `{hasUncommitted, hasParent, canMergeLocally, status, ahead,
  behind}`. Textbook Tier A: no DOM, no store, no framework.
- **`gitStatusToChangedFiles`** / **`reviewFilesSummaryToChangedFiles`** — two
  independent projections from two different daemon summary shapes into the
  same `GitDiff[]` sidebar-tree shape, each with its own status-interpretation
  rules (`'untracked' → is_new`, staged/uncommitted flag handling). Real,
  small, duplicated-but-distinct git-status semantics.
- **`buildGitFolderTree`** — flat changed-file list → folder tree (path
  segmentation, dedup, sort). Same shape as file-tree model (§5 below); this is
  the git-scoped variant of it.
- **`getFileStatus`** in `git-diff-helpers.ts` — a third, smaller
  restatement of the same is_new/is_deleted/is_renamed → label mapping
  already done twice above. Three near-duplicate implementations of "classify
  a file's change kind" is itself a finding: a single `crowbar-core` type
  should collapse all three.
- **`partitionReviewFiles`** (in `review-code-view.tsx`) — classifies each
  changed file as diff/image/binary from outline + summary. Pure, gpui-free.
- **The placeholder-geometry algebra** (`buildPlaceholderFileDiff`,
  `buildPlaceholderHunks`, `distributeContext`, `buildTailHunk`,
  `trimToPatchCap`, `reserveAtMost`) — genuinely the closest thing to "diff
  algebra" in the app: it reconstructs per-hunk row-count estimates from
  outline geometry (`oldStart/oldLines/newStart/newLines`) and the file's ±
  summary counts, entirely without line content. **But its input/output types
  (`Hunk`, `FileDiffMetadata`) are `@pierre/diffs` library types**, which are
  not carried into the port (§5.2 replaces `@pierre/diffs` with native
  `crowbar-diff`). The *algorithm* — how to distribute a context estimate
  across hunks bounded by `min(oldLines,newLines)`, how to trim/scale a
  placeholder to a patch cap — is portable in concept; the code is not,
  because its types belong to a library being deleted.

### What is not git-model logic

- **`git-diff-cache.ts`** — a hand-rolled TTL/LRU-ish in-memory cache keyed on
  `repo:file:staged`, with content-hash invalidation. This is exactly the
  caching architecture §9.3/D6 replaces: *"against a local daemon on a unix
  socket there is no latency to hide … `Entity<T>` fed by WS is the cache."*
  Not domain logic — a performance shim for a network-latency problem the
  native client does not have. **Out of scope, cite D6/§9.3.**
- **`patch-window.ts`'s `planWindow`** — deliberately documented as "no React,
  no fetch, no timers," and it is the single most rigorously pure file in the
  whole area (25 ported-able test cases). But it is a **viewport
  materialisation scheduler** (what to fetch/evict given scroll position and
  memory budgets), not diff algebra. §4.2 gives `crowbar-diff` its own
  logic partition (§12: `"diff(logic)" ≥98%`) precisely for pure logic that is
  diff-*rendering*-adjacent rather than diff-*structure* domain model. This
  belongs to `crowbar-diff`, not `crowbar-core` — an important distinction the
  brief's bucket list does not spell out: "diff algebra" in `crowbar-core`
  means hunk/line structure, not the windowing that consumes it.
- **`diff-search.ts`** — regex search over reconstructed unified text for the
  review surface's search bar. Pure and gpui-free, but it is view-search logic
  (imports `features/editor/utils/search`), the same `crowbar-diff`-logic
  bucket as `patch-window.ts`, not core git-model/diff-algebra.
- **`diff-buffer-path.ts`** — parses a synthetic `diff://staged/<path>` buffer
  identifier used for editor-tab addressing. Pure, but it's tab/buffer
  identity logic (editor/tabs feature), not git model.
- **`ParsedHunk`** (in `git-diff-types.ts`) is declared and never constructed
  anywhere in `web/src` — dead type.
- **`ImageContainerProps`/`ImageDiffViewerProps`** (same file) are component
  prop types — presentation, belongs with the component.
- **`getImgSrc`** (`git-diff-helpers.ts`) — formats a `data:` URI; presentation.

### Already done in `crowbar-proto`

`native/crates/crowbar-proto/src/generated/domain_git.rs` (316 lines) **already
has generated DTOs** for `Branch`, `Commit`, `Stash`, `Identity`,
`ConflictHunk`/`ConflictResolution`, `DiffLine`/`DiffLineType`, `FileDiff`,
`FileOutline`, `GitFileStatus`, `Hunk`, `HunkShape`, `MergeStrategy`,
`MultiFileDiff`, `ReviewFileSummary`, `SearchHit` — a near-exact match for
`web/src/features/git/types/git-types.ts` and `git-diff-types.ts` (119 lines
combined). **The two hand-written TS type files are not new Tier A work — they
duplicate what `crowbar-proto` already generated from the Go handlers (§9.2).**
The domain logic (branch-action, the two changed-files projections,
folder-tree building, the placeholder algebra's *concept*) is the part with no
existing counterpart.

### gpui-free?

Yes, for the genuine-logic set (`branch-action.ts`, both `*-to-changed-files`
projections, `build-git-folder-tree.ts`, `partitionReviewFiles`, the
placeholder-geometry functions rewritten against `crowbar-proto`'s `Hunk`
instead of `@pierre/diffs`'s). No DOM, no store, no React import in any of
them except `diff-buffer-path.ts` and `diff-search.ts`, which are not core
material anyway (see above).

### Tests

| test file | cases | covers |
|---|---|---|
| `utils/build-git-folder-tree.test.ts` | 19 | folder-tree building |
| `utils/normalize-diff.test.ts` | 6 | stale-shape defence |
| `utils/review-file-summary-to-git-diff.test.ts` | 6 | summary → sidebar projection |
| `utils/git-status-to-changed-files.test.ts` | 4 | status → sidebar projection |
| `utils/diff-search.test.ts` | 7 | (crowbar-diff logic, not core) |
| `lib/branch-action.test.ts` | 6 | `resolveBranchAction` |
| `lib/patch-window.test.ts` | 25 | (crowbar-diff logic, not core) |
| `components/diff/review-code-view.test.tsx` | 15 total, **5** (`partitionReviewFiles` ×2, `buildPlaceholderFileDiff` ×3) test the pure functions; the other 10 are component/windowing behaviour |

**Git-model Tier A test total: 19+6+6+4+6+5 = 46 cases across 6 areas** (folder
tree, normalize-diff, two projections, branch-action, placeholder algebra).
Zero test files exist for `git-diff-helpers.ts`'s `getFileStatus` — it is
exercised only incidentally through the two projection tests above.

## 2. Diff algebra

**Liveness note (P3.69, see §0 for method):** this section has no "Where it
lives" table of its own — everything it discusses is a row in §1's table
above, and the liveness verdicts live there. One correction specifically:
item 3 below (`diff-search.ts`) is discussed here as "pure, well-tested"
crowbar-diff-scoped logic with no liveness caveat attached — independently
re-checked for this item and found to be **DEAD** (0 non-test importers,
absent from the reachability BFS; see §1's table). It was never dispatched
for porting (unlike `patch-window.ts`, its sibling in the same 316-line
bucket, which is CONDITIONAL-live via `review-code-view.tsx`), so there is no
Rust code to correct — but the survey's own characterization of it as real,
reachable logic was never checked against the app, the same gap this whole
item exists to close.

**Finding: as a distinct area, "diff algebra" barely exists as first-party
code.** The daemon does the actual diffing (git itself, via the Go layer) and
returns structured `FileDiff`/`Hunk`/`DiffLine` shapes already generated into
`crowbar-proto` (`domain_git.rs`). §10.1's own conditional — "unless the daemon
already returns unified diff, in which case [it] ports directly and no
algorithm is needed" — resolves to **no algorithm needed**, confirmed above.
What the React app implements instead, under the `features/git/` files
surveyed in §1, are three things that are easy to mistake for "diff algebra"
but are not the same claim:

1. **File-status classification** (is_new/is_deleted/is_renamed → label) —
   genuine, tiny, git-model logic, counted in §1.
2. **Placeholder hunk-geometry estimation** for the windowed review renderer —
   real pure math, but its types belong to `@pierre/diffs` (being deleted) and
   its *purpose* is virtualiser sizing, i.e. `crowbar-diff`-crate logic, not
   `crowbar-core`.
3. **Viewport windowing/materialisation** (`patch-window.ts`) and **diff-text
   search** (`diff-search.ts`) — pure, well-tested, but `crowbar-diff`-crate
   logic, not `crowbar-core`.

So `crowbar-core`'s "diff algebra" is real but small: essentially the
file-status/change-kind classification already counted under git model, plus
whatever hunk/line data-shape work is needed to consume `crowbar-proto`'s
`FileDiff`/`Hunk`/`DiffLine` types directly (largely already solved by
`crowbar-proto` existing). The bulk of what *looks* like diff algebra in the
React app — windowing, search, placeholder sizing — belongs to `crowbar-diff`'s
own logic partition per §4.2/§12, not to `crowbar-core`. **This is the report's
first correction to the brief's bucket list**: "diff algebra" in the crate
description undersells how much of the React app's diff-adjacent pure logic is
actually scoped to a *different* crate (`crowbar-diff`) that also has a
gpui-free logic gate.

## 3. Keymap resolution

### Where it lives

All of it is under `web/src/features/keymaps/` (733 lines total, matching §3.1
exactly — no keymap logic found anywhere outside this directory):

| file | lines | shape |
|---|---|---|
| `types.ts` | 52 | `Command`, `CommandCategory`, `KeymapPreset(Id)`, `KeymapOverrides`, `EffectiveBinding` — the schema |
| `registry.ts` | 220 | `COMMANDS: Command[]` (19 commands) + `getCommand`, `CATEGORY_ORDER` — static data + lookup |
| `defaults/keybinding-presets.ts` | 49 | `KEYMAP_PRESETS` (`default`, `compact`) + `getPreset`, `isKeymapPresetId` |
| `utils/chord.ts` | 124 | chord grammar: parse/stringify/normalize/format + 2 `KeyboardEvent`-consuming functions |
| `utils/effective-keymaps.ts` | 71 | **the resolution algorithm**: `resolveBinding`, `getEffectiveBindings`, `getEffectiveChordMap`, `findConflictingCommands` |
| `stores/store.ts` | 100 | zustand store: active preset + user overrides, localStorage-persisted |
| `hooks/use-effective-keymap.ts` | 17 | thin `useMemo` wrapper over `getEffectiveChordMap` |
| `hooks/use-command-shortcut.ts` | 4 | **stub** — `return undefined` |
| `hooks/use-save-keyboard.ts` | 31 | `useEffect` + `window.addEventListener('keydown', …)` |
| `hooks/use-sidebar-tab-keyboard.ts` | 40 | same pattern |
| `hooks/use-workspace-switcher-keyboard.ts` | 25 | same pattern |

### Liveness

Method and controls: see §0. `use-effective-keymap.ts`'s `useEffectiveChordMap`
is called directly from `new-tab-view.tsx` (the default pane state, live per
`native/mapping/crowbar-wordmark.md`'s own chain) and from
`workspace-view.tsx`'s two always-mounted keyboard hooks — both root-reachable
without opening anything. That single fact carries almost the whole area
LIVE, since `effective-keymaps.ts` pulls in `registry.ts`, `chord.ts`,
`keybinding-presets.ts`, `types.ts`, and `stores/store.ts` directly.

| file | verdict | evidence |
|---|---|---|
| `types.ts` | LIVE | Imported by `effective-keymaps.ts` (below), reached from `new-tab-view.tsx`. |
| `registry.ts` | LIVE | `COMMANDS` imported directly by `effective-keymaps.ts`. |
| `defaults/keybinding-presets.ts` | LIVE | `getPreset`/`isKeymapPresetId` imported directly by `effective-keymaps.ts` and `stores/store.ts`. |
| `utils/chord.ts` | LIVE | `normalizeChord` imported by both `effective-keymaps.ts` and `stores/store.ts`. |
| `utils/effective-keymaps.ts` | LIVE | Called from `use-effective-keymap.ts`, which `new-tab-view.tsx:11` (default pane state) and `use-pane-keyboard.ts`/`use-sidebar-tab-keyboard.ts`/`use-workspace-switcher-keyboard.ts` all import directly. |
| `stores/store.ts` | LIVE | Imported by `use-effective-keymap.ts` (above) for active preset + user overrides. |
| `hooks/use-effective-keymap.ts` | LIVE | Direct importers include `new-tab-view.tsx` (default pane state) and `agent-chat-pane.tsx`. |
| `hooks/use-command-shortcut.ts` | **DEAD** | Verified directly (`cat`'d the file): unconditionally `return undefined`. Its one importer, `editor-status-actions.tsx`, is itself live (always-mounted editor toolbar), but calling a function that only ever returns `undefined` executes no real behaviour — a live call site around a dead-by-construction stub, matching the doc's own original "stub" note. |
| `hooks/use-save-keyboard.ts` | LIVE | `workspace-view.tsx:12`, unconditional inside the always-mounted `WorkspaceHost` chain. |
| `hooks/use-sidebar-tab-keyboard.ts` | LIVE | `workspace-view.tsx:14`, same chain. |
| `hooks/use-workspace-switcher-keyboard.ts` | LIVE | `context-pill.tsx:15`, mounted by default (`{!hasNavScreen && <ContextPill/>}`, `hasNavScreen` defaults false). |

### What is genuine, portable keymap-resolution logic

- **`types.ts` + `registry.ts` + `keybinding-presets.ts`** — the schema itself:
  a finite command list with default chords, categories, and a
  precedence-ordered preset system. 100% data + pure lookups. This is
  literally "keymap resolution"'s input model.
- **`chord.ts`'s grammar functions** — `parseChord`, `stringifyChord`,
  `normalizeChord`, `formatChord` are pure string algebra over a documented
  grammar (`[mod+][shift+][alt+]<key>`). Fully gpui-free.
- **`effective-keymaps.ts`** — the actual resolution algorithm the crate
  description names: `resolveBinding` merges default → preset → user override
  by precedence, normalizing every chord so comparison is canonical;
  `findConflictingCommands` does conflict detection across the resolved set.
  Pure, gpui-free, and the smallest, most literal match to "keymap resolution"
  in the whole survey.

### What is entangled or not core

- **`chord.ts`'s `chordFromEvent`/`eventMatchesChord`** take a DOM
  `KeyboardEvent` directly. The grammar they call into (`parseChord`,
  `stringifyChord`) is portable; the event-field extraction
  (`e.metaKey`/`e.ctrlKey`/`e.shiftKey`/`e.altKey`/`e.key`) is not — GPUI
  delivers its own `KeyDownEvent`/`Modifiers` shape (see the gpui skill), so
  this is a reimplementation-at-the-boundary, not a port, of 2 of the file's 6
  exports.
- **`store.ts`** — a zustand store persisting to `localStorage` under
  `crowbar:settings:keybindingPreset`/`…UserOverrides`. Reactive-state +
  persistence shell: Phase 4 (`Entity<T>`), and the persistence mechanism
  itself is D6 territory (deleted, not ported — see §4 below for where user
  keybinding overrides would need to land in the daemon-side `/v0/settings/ui`
  scheme if kept at all).
- **The four hooks** (`use-effective-keymap`, `use-save-keyboard`,
  `use-sidebar-tab-keyboard`, `use-workspace-switcher-keyboard`) are
  `useEffect` + `window.addEventListener('keydown', …)` wiring that dispatches
  on a resolved chord. This is not "keymap resolution" so much as the
  reactive-subscription layer §7 governs — and in GPUI it doesn't port at all
  in this shape: GPUI has its own native action/keybinding dispatch system
  (see the gpui skill), so this glue is replaced, not translated.
  `use-command-shortcut.ts` is a dead stub (4 lines, returns `undefined`).

### gpui-free?

The schema (`types.ts`, `registry.ts`, `keybinding-presets.ts`) and the
resolution algorithm (`effective-keymaps.ts`) — yes, entirely. The chord
grammar (`parseChord`/`stringifyChord`/`normalizeChord`/`formatChord`) — yes.
The event-matching half of `chord.ts` — no, tied to `KeyboardEvent`; the
*concept* (compare mod/shift/alt/key against a parsed chord) survives, the
code does not verbatim.

### Already done in `crowbar-proto`/`crowbar-client`

None. No keymap-related type appears in `crowbar-proto`'s generated set — this
is pure frontend-local state with no daemon wire representation today (user
overrides live in `localStorage`, not behind any `/v0/*` endpoint).

### Tests

| test file | cases | covers |
|---|---|---|
| `chord.test.ts` | 7 | chord grammar |
| `registry.test.ts` | 9 | `COMMANDS` data (pinned chord assignments) |
| `hooks/use-sidebar-tab-keyboard.test.ts` | 7 | hook/DOM wiring, not core |
| `hooks/use-save-keyboard.test.ts` | 6 | hook/DOM wiring, not core |
| `hooks/use-workspace-switcher-keyboard.test.ts` | 4 | hook/DOM wiring, not core |

**‼️ Finding: `effective-keymaps.ts` — the file that most literally *is*
"keymap resolution" — has zero dedicated tests.** No test file references
`resolveBinding`, `getEffectiveBindings`, `getEffectiveChordMap`, or
`findConflictingCommands` anywhere in `web/src/__tests__`. Neither does
`keybinding-presets.ts` or `store.ts`. Since §16 gates Tier A on *ported*
tests, this area's most important single file arrives at the port with no
test suite to port — new tests would have to be authored, not translated, a
materially different (and more expensive) kind of Tier A work than the areas
that do have a suite already.

**Keymap-resolution Tier A test total: 7 (chord) + 9 (registry) = 16 cases.**
The 17 hook-test cases are Phase 4/glue, not core.

## 4. Settings schema

### Where it lives

`web/src/features/settings/` outside `components/`:

| file | lines | shape |
|---|---|---|
| `types/settings.ts` | 81 | `Settings` interface — **~50 fields**, the schema itself |
| `types/feature.ts` | 3 | `CoreFeaturesState` |
| `types/search.ts` | 20 | `SettingSearchRecord`/`SearchResult`/`SearchState` — presentation |
| `config/default-settings.ts` | 98 | `defaultSettings` + `getDefaultSetting`/`getDefaultSettingsSnapshot` |
| `config/typography-defaults.ts` | 25 | font/size constants |
| `config/search-index.ts` | 387 | static UI-copy data for settings search (labels/descriptions/keywords) — presentation |
| `lib/settings-normalization.ts` | 249 | **validation/clamping/migration for ~15 fields** |
| `lib/font-family-resolution.ts` | 40 | font-family parse/normalize/resolve-against-available |
| `lib/markdown-font-size.ts` | 26 | clamp/snap one numeric field |
| `lib/ui-font-size.ts` | 32 | clamp/snap/scale one numeric field |
| `lib/settings-import-export.ts` | 75 | versioned export envelope, import validation |
| `lib/settings-download.ts` | 39 | `buildSettingsExportFile` (pure) + `downloadSettingsFile` (DOM) |
| `lib/settings-bootstrap.ts` | 55 | orchestrates persistence+normalization+side-effects at startup |
| `lib/settings-persistence.ts` | 130 | **localStorage-backed** store shim, D6 territory |
| `lib/settings-effects.ts` | 198 | DOM class/attribute application (theme, transparency) |
| `lib/appearance-bootstrap.ts` | 197 | pre-hydration FOUC-prevention cache, DOM-applied |
| `lib/settings-row-search.ts` | 16 | string match, `ReactNode`-typed param |
| `lib/settings-tab-visibility.ts` | 16 | tab filtering by search match — presentation |
| `lib/diagnostics-export.ts` | 18 | Tauri IPC command wrapper (§3.5) |
| `utils/theme-upload.ts` | 154 | theme-file validation + CSS-variable generation |
| `store.ts` | 171 | zustand store |
| `stores/agent-providers-store.ts` | 85 | zustand store, async fetch |
| `stores/font-store.ts` | 156 | zustand store, async fetch + cache |
| `stores/types/font.ts` | 6 | `FontInfo` type |

**2,277 lines total** across these 22 files (matches `wc -l`, measured
directly).

### Liveness

Method and controls: see §0. `main.tsx:11-12` calls `initializeSettingsStore`
(`store.ts` → `settings-bootstrap.ts`) and `ensureStartupAppearanceApplied`
(`appearance-bootstrap.ts`) **directly at boot**, before any dialog exists to
open — this is root-reachable in the strongest sense, not merely "inside
default IDE chrome." `settings-bootstrap.ts`'s `resolveInitialSettings`/
`initializeSettingsState` call `default-settings.ts`, `settings-persistence.ts`
(load/save), `normalizeSettings` (`settings-normalization.ts`), and
`applySettingsSideEffects` (`settings-effects.ts`) unconditionally; `settings-
normalization.ts` in turn imports `theme-registry.ts`, `file-tree-density.ts`,
`font-family-resolution.ts`, `markdown-font-size.ts`, `ui-font-size.ts`, and
`typography-defaults.ts` directly — all LIVE by the same boot-time chain.
Everything else in this area lives only inside the Settings dialog
(`isSettingsOpen: false` by default, confirmed in `ui-state-store.ts:82`) or a
specific tab within it — CONDITIONAL, per §0's rule.

| file | verdict | evidence |
|---|---|---|
| `types/settings.ts` | LIVE | The `Settings` type flowing through the boot chain above; imports `types/feature.ts`'s `CoreFeaturesState` directly. |
| `types/feature.ts` | LIVE | Sole importer `types/settings.ts` (above). |
| `types/search.ts` | CONDITIONAL | Used only by `store.ts`'s settings-search slice (`SearchResult`/`SearchState`), exercised only when the Settings dialog's search box is used. |
| `config/default-settings.ts` | LIVE | Called directly in `settings-bootstrap.ts`'s boot path. |
| `config/typography-defaults.ts` | LIVE | Imported by `default-settings.ts`, `settings-normalization.ts`, `appearance-bootstrap.ts`, `ui-font-size.ts`, `markdown-font-size.ts` — all boot-chain LIVE files. |
| `config/search-index.ts` | CONDITIONAL | `settingsSearchIndex` used only by `store.ts`'s search slice (search box). |
| `lib/settings-normalization.ts` | LIVE | Called directly in `settings-bootstrap.ts`'s boot path. |
| `lib/font-family-resolution.ts` | LIVE | Imported directly by `settings-normalization.ts` (boot). |
| `lib/markdown-font-size.ts` | LIVE | Imported directly by `settings-normalization.ts` (boot). |
| `lib/ui-font-size.ts` | LIVE | Imported directly by `settings-normalization.ts` (boot). |
| `lib/settings-import-export.ts` | CONDITIONAL | Both call sites — `settings-download.ts` and `store.ts`'s `updateSettingsFromJSON` — are invoked only from `developer-settings.tsx` (Settings → Developer tab's export/import buttons). |
| `lib/settings-download.ts` | CONDITIONAL | Sole importer `developer-settings.tsx` (Settings → Developer tab, download button). |
| `lib/settings-bootstrap.ts` | LIVE | Called directly from `main.tsx:11` via `store.ts`. |
| `lib/settings-persistence.ts` | LIVE | `loadSettingsFromStore`/`saveSettingsToStore` called directly inside `settings-bootstrap.ts`'s boot path. (Liveness ≠ port-worthiness: this is still the D6-deleted persistence mechanism per the doc's original classification — LIVE today, out of scope regardless.) |
| `lib/settings-effects.ts` | LIVE | `applySettingsSideEffects` called directly inside `settings-bootstrap.ts`'s boot path. |
| `lib/appearance-bootstrap.ts` | LIVE | Called directly from `main.tsx:12`. |
| `lib/settings-row-search.ts` | CONDITIONAL | Sole importer `settings-section.tsx`, only relevant while the Settings dialog's search box is in use. |
| `lib/settings-tab-visibility.ts` | CONDITIONAL | Importers `settings-vertical-tabs.tsx`/`settings-dialog.tsx`, same search-box scope. |
| `lib/diagnostics-export.ts` | CONDITIONAL | Sole importer `developer-settings.tsx` (Settings → Developer tab, export button). |
| `utils/theme-upload.ts` | CONDITIONAL | Dynamically imported only from `appearance-settings.tsx`'s upload action (Settings → Appearance tab). |
| `store.ts` | LIVE — mixed | The base store is read outside the Settings dialog entirely: `FpsOverlay` (unconditionally mounted, `ide-shell.tsx:276`) reads `useSettingsStore((s) => s.settings.showFpsOverlay)`, and `tab-bar.tsx` (always-visible tab strip) reads it too. But this one file also carries the search slice and `updateSettingsFromJSON` (both CONDITIONAL, see `types/search.ts`/`settings-import-export.ts` above) — a mixed file in the same shape the doc's own "brief's fourth bucket" pattern already names elsewhere. |
| `stores/agent-providers-store.ts` | LIVE — mixed | Two importers: `providers-settings.tsx` (Settings tab, CONDITIONAL) and `use-workspace-agent-chats-stream.ts`, which `agent-chats-panel.tsx` calls directly. `agent-chats-panel.tsx` is rendered unconditionally at `sidebar-carousel.tsx:122` (`data-oracle-id="carousel-panel-chats"`), the same always-mounted-carousel-page pattern already established for `GitPanel`/`FileExplorerTree` in §1 — confirmed by reading the JSX, not assumed. Verdict is LIVE via that chain even though the Settings-tab chain is separately CONDITIONAL. |
| `stores/font-store.ts` | CONDITIONAL | Importers `terminal-settings.tsx` and `font-selector.tsx` (used by `appearance-settings.tsx`/`editor-settings.tsx`) — all Settings-dialog-tab scope. |
| `stores/types/font.ts` | CONDITIONAL | `FontInfo` type flows only through `font-store.ts` (above). |

### What is genuine settings-schema logic

- **`types/settings.ts`** — the `Settings` interface itself: ~50 fields
  spanning general/editor/terminal/UI/theme/layout/language/file-tree
  settings. This is exactly "settings schema" as named.
- **`config/default-settings.ts`** — the default-value table + accessors.
- **`lib/settings-normalization.ts`** — the validation half of "settings
  schema + validation": per-field clamping (`normalizeEditorLineHeight`,
  `normalizeFileTreeIndentSize`, `normalizeWorkspaceKeepAliveMinutes`),
  enum-membership checks (`renderWhitespace`, `editorEngine`,
  `externalEditor`), a dead-theme-id fallback, and cross-field rules (e.g.
  `externalEditor !== 'none'` overrides `editorEngine`; a `custom` engine with
  no command string falls back to `monaco`). 249 lines, entirely pure,
  entirely gpui-free except one call into `themeRegistry.getTheme` (itself a
  pure lookup, not a component).
- **`lib/font-family-resolution.ts`, `markdown-font-size.ts`,
  `ui-font-size.ts`** — smaller, single-field validators the normalization
  file composes. Pure.
- **`lib/settings-import-export.ts`** — versioned export/import envelope
  (`format`/`version`/`exportedAt`), and import validation that re-runs
  `normalizeSettings` over whatever the file contained. Pure (`isRecord`,
  `pickSettings`, `getSettingsCandidate`, `createSettingsExportPayload`,
  `parseSettingsImportJson`) — genuinely "safe to unit test without a DOM," as
  `settings-download.ts`'s own comment for the sibling function puts it.
- **`lib/settings-download.ts`'s `buildSettingsExportFile`** — the
  serialize-to-JSON half is pure; `downloadSettingsFile` (Blob/anchor-click)
  is not and is out of scope regardless (native file-save is a platform
  concern, §3.5-adjacent).

### What is not settings-schema logic

- **`lib/settings-persistence.ts`** — a `localStorage`-backed key/value shim
  under the `crowbar:settings:` prefix, with a `Store` interface
  (`get`/`set`/`save`/`onKeyChange`) mimicking Tauri's old settings-store API.
  **This is exactly what D6 deletes.** §9.3's `GET/PUT /v0/settings/ui`
  (already built, QUEUE.md item 0.6, verified live) is the daemon-side
  replacement — but note it stores **opaque JSON**, so it does not itself
  carry the `Settings` schema; the schema still has to exist client-side to
  give that opaque blob a shape. **Persistence mechanism: out of scope (D6).
  Schema itself: still needed, and is the Tier A content above.**
- **`lib/settings-effects.ts`, `lib/appearance-bootstrap.ts`** — both are
  `document`/`window`/CSS-custom-property/class-list manipulation: applying a
  theme by toggling `.dark`, setting `--app-font-family` etc., syncing
  `matchMedia('(prefers-color-scheme: dark)')`, and (in
  `appearance-bootstrap.ts`) a pre-React-hydration flash-of-unstyled-content
  cache read/applied before the app mounts. **This entire mechanism is a
  webview artifact.** A native GPUI window has no FOUC to prevent (state is
  resolved before the first frame paints) and no CSS custom properties to
  toggle (§6.1's sealed `Theme` struct replaces it entirely). Out of scope —
  not a port target, not even conceptually; it is the exact class of problem
  §2 of the spec cites as the motivation for the rewrite.
- **`lib/settings-bootstrap.ts`** — orchestration gluing
  persistence+normalization+effects at startup; Phase 4 wiring. Contains one
  small embedded rule (derive `theme` from OS `prefers-color-scheme` when
  `syncSystemTheme` is on) that could be extracted as pure logic but isn't
  today — currently inline and `window.matchMedia`-coupled.
- **`lib/settings-row-search.ts`, `settings-tab-visibility.ts`,
  `config/search-index.ts`** — the settings-search feature: 387 lines of
  static label/description/keyword copy plus two small string-match/filter
  functions, one of which (`settingRowMatchesQuery`) types its `description`
  parameter as `ReactNode`. **Presentation, belongs with the settings-search
  component**, not domain logic — this is squarely the brief's fourth bucket
  ("formatting a label" — here, indexing one).
- **`utils/theme-upload.ts`** — validates an uploaded theme JSON file, but
  its output (`convertNewFormatTheme`) manufactures literal CSS custom
  property strings (`--color-${key}`) from user data. This is precisely what
  §6.1's sealed token types exist to prevent at compile time
  ("a worker agent cannot write `rgb(0x1e1e1e)` at a call site"): the
  *validation* logic (format detection, required-field checks) is portable,
  the *output shape* is not — it would need to construct `crowbar-ui::Color`
  values via `seal`, not CSS strings. Overlaps with the theme-tokens area
  below.
- **`lib/diagnostics-export.ts`** — a direct Tauri IPC command wrapper
  (`__TAURI_INTERNALS__.invoke('diagnostics_export')`). §3.5 already assigns
  `diagnostics_export` to `crowbar-platform`, not `crowbar-core`.
- **`store.ts`, `stores/agent-providers-store.ts`, `stores/font-store.ts`** —
  three zustand stores (search-run orchestration, async provider fetch with
  request-race guarding, async font enumeration with a 24h cache). Phase 4.
- **`types/feature.ts`, `types/search.ts`** — trivial/presentation-adjacent
  types, not schema in the crate-description sense.

### gpui-free?

The schema + normalization + import/export set — yes, fully, once
`themeRegistry.getTheme` is confirmed pure (it is a registry lookup, not a
component). Everything else in the directory either touches `document`/
`window`/`localStorage` directly or is reactive-state/Phase-4 material.

### Already done in `crowbar-proto`/`crowbar-client`

None found. `Settings` has no counterpart in `crowbar-proto`'s generated set —
unsurprising, since today it is never sent to the daemon as a typed payload
(only as opaque JSON via the new `/v0/settings/ui`, and even that endpoint
didn't exist before Phase 0 item 0.6). This is genuinely new Tier A surface,
not a duplicate of already-generated DTOs (contrast with git model, §1, where
the type files mostly *were* duplicates).

### Tests

| test file | cases | covers |
|---|---|---|
| `settings-normalization.test.ts` | 15 | `normalizeSettings`/`normalizeSettingValue` |
| `settings-normalization-theme.test.ts` | 4 | theme-id fallback specifically |
| `ui-font-size.test.ts` | 5 | `ui-font-size.ts` |
| `lib/markdown-font-size.test.ts` | 6 | `markdown-font-size.ts` |
| `font-family-resolution.test.ts` | 6 | `font-family-resolution.ts` |
| `settings-import-export.test.ts` | 3 | export/import envelope |
| `settings-download.test.ts` | 2 | `buildSettingsExportFile` (+DOM path) |
| `settings-tab-visibility.test.ts` | 4 | (presentation, not core) |
| `config/search-index.test.ts` | 4 | (presentation, not core) |
| `lib/settings-effects.test.ts` | 4 | (DOM, not core) |
| `lib/diagnostics-export.test.ts` | 3 | (platform IPC, not core) |
| `stores/agent-providers-store.test.ts` | 12 | (Phase 4, not core) |
| `stores/font-store.test.ts` | 2 | (Phase 4, not core) |
| `components/*` (3 files) | 19 | component rendering, not core |

**Settings-schema Tier A test total: 15+4+5+6+6+3+2(partial) ≈ 39–41 cases**
across 6–7 files (schema validation, both single-field validators, export
envelope). This is the largest ported-able test base of any of the seven
areas measured so far.

## 5. File-tree model

### ‼️ Methodological note: a nested nearly-duplicate directory tree

`features/file-explorer/{lib,stores,utils}/*.ts` are **all 1-line re-export
shims** (`export * from '../file-explorer/lib/X'`) pointing at the real
content under `features/file-explorer/file-explorer/{lib,stores,utils}/*.ts`.
`ls`-driven counting would double the real total; every line count below is
from the real (nested) files only. This is the file-tree analogue of the
QUEUE.md lesson about not trusting a directory listing as scope.

### Where it lives

| file | lines | shape |
|---|---|---|
| `file-explorer/lib/visible-file-tree-rows.ts` | 238 | flatten nested tree → visible virtualised rows, given expand state; search/filter with ancestor-expansion; sticky/guide ancestor computation |
| `file-explorer/lib/file-tree-gitignore.ts` | 237 | cascading `.gitignore` rule resolution across nested directories (via the `ignore` npm package) |
| `file-explorer/lib/file-tree-git-status.ts` | 122 | git-status → per-row decoration, with directory-level status propagated from the highest-priority child |
| `file-explorer/lib/env-template.ts` | 90 | `.env` file template generation + comment-preserving KEY=VALUE parsing (narrow, not really tree-shaped) |
| `file-explorer/lib/file-tree-density.ts` | 38 | density enum + `normalizeFileTreeDensity` (real) + `FILE_TREE_DENSITY_CONFIG` row-height/className map (presentation) |
| `file-explorer/utils/file-explorer-tree-utils.ts` | 96 | immutable tree mutations: `filterHiddenFiles`, `addNewItemToTree`, `removeEditingItemsFromTree`, `getAncestorDirectoryPaths` |
| `file-explorer/stores/file-explorer-tree-store.ts` | 146 | zustand store — expand/select/collapse state (**D2-named**, see below) |
| `file-explorer/stores/file-explorer-clipboard-store.ts` | 110 | zustand store — copy/cut/paste, network calls |
| `file-explorer/hooks/use-file-explorer-drag-drop.ts` | 315 | drag/drop DOM handlers |
| `file-explorer/hooks/use-file-explorer-inline-editing.ts` | 231 | inline rename/create DOM+state handlers |
| `file-explorer/hooks/use-file-explorer-gitignore.ts` | 79 | wires `file-tree-gitignore.ts` to store state |
| `file-explorer/hooks/use-file-explorer-sync.ts` | 50 | reconciliation glue |
| `file-explorer/hooks/use-file-explorer-visible-rows.ts` | 87 | wires `visible-file-tree-rows.ts` to store state |
| `file-explorer/hooks/use-file-explorer-context-menu.tsx` | — (not counted, component-adjacent) | |
| `features/files/lib/file-tree-api.ts` | 141 | transport (fetch calls) + `toAppFile` DTO mapping |
| `features/files/lib/file-upload.ts` | — | transport |
| `features/file-system/controllers/file-tree-utils.ts` | 22 | `findFileInTree` — depth-first lookup |
| `features/file-system/controllers/file-utils.ts` | — | mixed |
| `features/file-system/types/app.ts` | — | `FileEntry`/`AppFile` type definitions |

### Liveness

Method and controls: see §0. `FileExplorerTree` mounts unconditionally at
`sidebar-carousel.tsx:132` (`data-oracle-id="carousel-panel-files"`, same
always-mounted-carousel-page pattern as `GitPanel` in §1) inside a `Suspense`
that never actually suspends (confirmed already in
`native/mapping/liveness-audit.md`'s `skeleton` row — no `React.lazy`/
suspending hook anywhere under `features/file-explorer/`), so it renders
synchronously on every app load with a workspace open. `file-explorer-tree.tsx`
imports all six of its interaction hooks directly and unconditionally (React's
own rule: hooks cannot be called behind an `if`), which wires them up on every
render even though several only do something meaningful once the user drags,
renames, or right-clicks.

| file | verdict | evidence |
|---|---|---|
| `file-explorer/lib/visible-file-tree-rows.ts` | LIVE | `buildVisibleFileTreeRows` called directly in `file-explorer-tree.tsx`'s render body (also via `use-file-explorer-visible-rows.ts`). |
| `file-explorer/lib/file-tree-gitignore.ts` | LIVE | `collectGitIgnoreFileReferences` imported directly into `file-explorer-tree.tsx`; `use-file-explorer-gitignore.ts`'s `useEffect` computes the ignore rule set on every mount, not behind a toggle. |
| `file-explorer/lib/file-tree-git-status.ts` | LIVE | Imported directly by `file-explorer-tree.tsx` and `file-explorer-tree-item.tsx`, applying status decoration to every row whenever a repo exists (the ordinary case). |
| `file-explorer/lib/env-template.ts` | CONDITIONAL | Sole importer `use-file-explorer-context-menu.tsx` — only exercised by a specific right-click "create/parse .env" context-menu action. |
| `file-explorer/lib/file-tree-density.ts` | LIVE | `normalizeFileTreeDensity` reached via `settings-normalization.ts` (boot, §4); `FILE_TREE_DENSITY_CONFIG` also imported directly into `file-explorer-tree.tsx`. |
| `file-explorer/utils/file-explorer-tree-utils.ts` | **LIVE — but 4 of its 5 exports are dead, a "live file with a dead export" case worth flagging on its own.** | Re-checked every exported name individually (not just the file): `getExplorerTargetPath` has one real importer, `use-file-explorer-sync.ts` (called via `useMemo` on every render, reached through the always-mounted tree). But `filterHiddenFiles` and `removeEditingItemsFromTree` have **zero references anywhere in `web/src`, including tests** — dead on arrival. `addNewItemToTree` and `getAncestorDirectoryPaths` each have a same-named function **independently redeclared locally** inside `use-file-explorer-inline-editing.ts` and `file-tree-gitignore.ts` respectively — the exported originals are never called; `getAncestorDirectoryPaths` even has its own dedicated test (`__tests__/features/file-explorer/file-explorer-tree-utils.test.ts`, 2 of its 4 cases) that exercises the exported-but-unreachable version, matching the shape `native/mapping/core-git.md` §3 already documents for `diff-buffer-path.ts` ("its only importer... is a test file"). **The doc's own "genuine, portable" prose for this file (below) describes exactly the four dead exports and never mentions the one live one.** |
| `file-explorer/stores/file-explorer-tree-store.ts` | LIVE | `useFileTreeStore` read directly in `file-explorer-tree.tsx`'s render (expand/select state) and `sidebar-carousel.tsx`. |
| `file-explorer/stores/file-explorer-clipboard-store.ts` | CONDITIONAL | Read via `.getState()` inside a specific paste/cut/copy action handler (`file-explorer-tree.tsx:986`), not subscribed for every-render display; content only populates after a cut/copy action. |
| `file-explorer/hooks/use-file-explorer-drag-drop.ts` | CONDITIONAL | Hook wiring runs every render (React rule), but its substantive behaviour fires only on an actual drag interaction. |
| `file-explorer/hooks/use-file-explorer-inline-editing.ts` | CONDITIONAL | Same shape — substantive behaviour only on a rename/create action. |
| `file-explorer/hooks/use-file-explorer-gitignore.ts` | LIVE | Its `useEffect` computes gitignore rules on every mount (see `file-tree-gitignore.ts` row). |
| `file-explorer/hooks/use-file-explorer-sync.ts` | LIVE | `useMemo(() => getExplorerTargetPath(activeBuffer), [activeBuffer])`, recomputed on every render. |
| `file-explorer/hooks/use-file-explorer-visible-rows.ts` | LIVE | Calls `buildVisibleFileTreeRows` directly in its return (see above). |
| `file-explorer/hooks/use-file-explorer-context-menu.tsx` | CONDITIONAL | Meaningful content (the menu's item list) only matters once right-clicked. |
| `features/files/lib/file-tree-api.ts` | LIVE | `fetchFileTree`/etc. called from `use-workspace-effects.ts`, itself called from `workspace-view.tsx` (always-mounted `WorkspaceHost` chain) — loading the tree for the active workspace is not optional. |
| `features/files/lib/file-upload.ts` | CONDITIONAL | `pickAndUploadFiles` wired only into `useFileExplorerContextMenu`'s `onUploadFile` — a right-click action, same scope as `env-template.ts`. |
| `features/file-system/controllers/file-tree-utils.ts` | LIVE | `findFileInTree` called directly in `file-explorer-tree.tsx:614` and `use-file-explorer-inline-editing.ts`. |
| `features/file-system/controllers/file-utils.ts` | LIVE | `getFilenameFromPath` called unconditionally in `editor-status-actions.tsx`'s render (`rootFolderPath ? getFilenameFromPath(rootFolderPath) : 'No Project'`) — the always-mounted editor toolbar's project-name display, not gated on a file being open. |
| `features/file-system/types/app.ts` | LIVE | `AppFile` flows through `file-tree-api.ts`, `file-explorer-tree-store.ts`, and the tree component itself — all LIVE above. |

### What is genuine, portable file-tree-model logic

- **`visible-file-tree-rows.ts`** — the closest thing to a canonical
  "file-tree model" in the app: `buildVisibleFileTreeRows` (nested tree + expand
  set → flat visible-row list, with compact single-child-folder collapsing),
  `computeFileTreeSearchHits`/`filterFileTreeForFffHits` (name-substring search
  with ancestor auto-expansion), `getStickyAncestorRow(s)`/`getGuideAncestorRows`
  (breadcrumb/indent-guide support for virtualized rendering). All pure,
  gpui-free, no DOM.
- **`file-tree-gitignore.ts`** — real algorithmic weight: reference collection
  (`collectGitIgnoreFileReferences`), depth-ordered rule-set construction
  (`createFileTreeGitIgnoreRules`), and cascading ignore resolution that walks
  every ancestor directory before testing the target path itself
  (`isPathGitIgnoredByFileTreeRules`) — because a directory ignored by a parent
  rule ignores everything under it regardless of its own `.gitignore`. Uses the
  npm `ignore` package for pattern matching; Rust has a well-known equivalent
  (`ignore`, from ripgrep's author) that the port would reach for instead of a
  hand-rolled matcher.
- **`file-explorer-tree-utils.ts`** — immutable tree editing: filter/insert/
  remove/ancestor-walk, all recursive pure functions over `FileEntry[]`.
  **Liveness amendment (P3.69, see "Liveness" above):** of these, only
  `getExplorerTargetPath` — not even named in this bullet — has a production
  importer. `filterHiddenFiles` and `removeEditingItemsFromTree` are called
  nowhere, including tests; `addNewItemToTree` and `getAncestorDirectoryPaths`
  each have an unrelated, independently-declared same-named function
  elsewhere that shadows them in practice. This bullet describes the file's
  intent, not what actually runs — a "live file holding dead exports" case,
  not a "genuine, portable, and reachable" one as originally framed.
- **`features/file-system/controllers/file-tree-utils.ts`'s
  `findFileInTree`** — pure depth-first lookup, 22 lines, its own bug history
  documented in the file (used to unconditionally return null).
- **`file-tree-git-status.ts`'s logic half** — `createFileTreeGitStatusLookup`
  (propagate the highest-priority status up every ancestor directory, with an
  explicit priority table) and `resolveActiveWorkspaceGitStatus` (workspace-
  scope validity guard against a documented past bug where the comparison was
  always false). Real domain rules.
- **`file-tree-density.ts`'s `normalizeFileTreeDensity`/`isFileTreeDensity`** —
  small but genuine settings-schema-adjacent validation (already called from
  `settings-normalization.ts`, §4).

### What is presentation or not core

- **`file-tree-git-status.ts`'s `getFileTreeGitStatusDecoration`** returns a
  hardcoded Tailwind `colorClassName` string (`'text-git-modified-staged'`
  etc.) alongside the genuine `statusLetter`/`label` classification — a clean
  example of the brief's fourth bucket: real logic (which status wins,
  M/A/D/U/R) fused in the same function with presentation (which CSS class).
  §6.1's sealed `Color` tokens are exactly what replaces the class-string half.
- **`file-tree-density.ts`'s `FILE_TREE_DENSITY_CONFIG`** — row heights and
  Tailwind class strings per density mode. Presentation, belongs with the row
  component.
- **`env-template.ts`** — real, tested, pure logic (KEY=VALUE parsing with
  quote/escape/inline-comment awareness), but it is `.env`-file-content
  domain, not tree-shape domain. Doesn't fit any of the seven named areas
  cleanly; flagging it as an unclassified pure-logic pocket rather than
  forcing it into "file-tree model."
- **`file-explorer-tree-store.ts`** — a zustand store for
  expanded/selected-paths state. **This is the literal case D2 names as its
  own example**: *"Selection logic, tree-expansion state, and similar get
  pulled out of components into core."* The store's mutator bodies
  (toggle/select/expand-to-path/collapse-path/expand-all) are, underneath the
  `create/immer/combine` wrapper, pure `Set<string> → Set<string>`
  transitions — genuinely portable into `crowbar-core` as plain functions,
  with only the reactive-subscription shell going to `crowbar-state`. This is
  the one file in the whole survey the spec itself pre-classifies.
- **`file-explorer-clipboard-store.ts`** — network calls
  (`renameFileNode`/`copyFileNode`) and workspace lookup; Phase 4. One small
  embedded rule (a failed cut's entries stay staged, a successful cut's don't)
  is buried in an async function rather than factored out.
- **`use-file-explorer-drag-drop.ts`, `use-file-explorer-inline-editing.ts`,
  `use-file-explorer-gitignore.ts`, `use-file-explorer-sync.ts`,
  `use-file-explorer-visible-rows.ts`** — all `useEffect`/DOM-event/store-glue
  hooks (546 lines across the two largest). Phase 4/presentation-glue, not
  core, though the two "wire the pure lib function to store state" hooks
  (`use-file-explorer-gitignore.ts`, `use-file-explorer-visible-rows.ts`)
  confirm the lib functions above are already factored out cleanly.
- **`features/files/lib/file-tree-api.ts`** — transport (`fetchFileTree`,
  `createFileNode`, `renameFileNode`, `deleteFileNode`, `writeFileContent`):
  `crowbar-client` territory, not `crowbar-core`. Its `toAppFile` mapping
  function is the only logic-shaped piece, and it maps a DTO
  (`crowbar-proto` already generates `FileNode`, see below) to a display
  model — thin, not new domain logic.

### gpui-free?

Yes for the genuine set: `visible-file-tree-rows.ts`, `file-tree-gitignore.ts`,
`file-explorer-tree-utils.ts`, `file-tree-utils.ts`'s `findFileInTree`, and
the classification half of `file-tree-git-status.ts`. No DOM, no React, no
store import in any of them (the two hooks that *use* them are the
DOM/store-entangled layer, kept separate).

### Already done in `crowbar-proto`

`native/crates/crowbar-proto/src/generated/domain.rs` already has `FileNode`
(line 30) and `FileContent` (line 22) — generated DTOs matching
`FileNodeDTO`/`AppFile`. The transport-layer DTO shapes are done; the
tree-shape *algorithms* (visible-row flattening, gitignore cascade, status
propagation) have no counterpart and are the real Tier A content here.

### Tests

| test file | cases | covers |
|---|---|---|
| `file-tree-gitignore.test.ts` | 16 | gitignore cascade |
| `visible-file-tree-rows.test.ts` | 12 | row flattening/search/ancestors |
| `file-tree-git-status.test.ts` | 9 | status decoration + propagation |
| `file-tree-search-hits.test.ts` | 5 | (overlaps `visible-file-tree-rows`'s search half — separate file) |
| `file-explorer-tree-utils.test.ts` | 4 | immutable tree edits |
| `env-template.test.ts` | 3 | (not tree-shape, see above) |
| `file-explorer/hooks/*.test.{ts,tsx}` (2 files) | 13 | hook/DOM wiring, not core |
| `file-explorer-clipboard-store.test.ts` + `clipboard-paste-mapping.test.ts` | 11 | Phase 4, not core |
| `file-explorer-tree-item.test.tsx` | 6 | component, not core |
| `file-system/controllers/file-tree-utils.test.ts` | 5 | `findFileInTree` |
| `files/lib/file-tree-api.test.ts` + `files/file-tree-api.test.ts` | 15 | transport (**two test files for one source file — a mirror-structure drift CLAUDE.md's rule would not produce; likely one is stale**) |

**File-tree-model Tier A test total: 16+12+9+5+4+5 = 51 cases** across 6 files
(gitignore, row-building/search, status, tree-edit utils, tree lookup). The
largest single-area test base measured so far, ahead even of settings.

## 6. Workspace scoping

### Where it lives

Split across `web/src/lib/` and `web/src/features/workspace/lib/` — **not
concentrated in `features/workspace/` alone**, unlike the other six areas
which each live under one feature directory:

| file | lines | shape |
|---|---|---|
| `lib/workspace-scope.ts` | 87 | `WorkspaceScope{projectId,repoId,wsId}` + registry (`Map`) + route-path parse |
| `lib/workspace-scope-url.ts` | 28 | scope → REST base-path construction, home-workspace detection |
| `lib/workspace/resolve-root-path.ts` | 31 | active workspace → on-disk root path (store-entangled) |
| `lib/workspace/placeholder.ts` | 32 | placeholder-workspace detection + reason string |
| `lib/workspace/branch-workspace.ts` | 16 | branch → owning-workspace-id lookup within a repo |
| `features/workspace/lib/keep-alive-policy.ts` | 98 | **pure** retention/eviction policy for mounted workspaces |
| `features/workspace/lib/activation-freshness.ts` | 90 | warm-reactivation freshness ledger (borderline, see below) |
| `features/workspace/lib/home-workspace-resolver.ts` | 89 | async-fetch + cache + `useSyncExternalStore` — Phase 4 |
| `features/workspace/lib/external-buffer-sync.ts` | 90 | external-disk-change reconciliation for open editor buffers |
| `features/workspace/lib/reset-workspace-scoped-stores.ts` | 47 | store-reset-on-activation glue |
| `features/workspace/lib/open-file-content.ts` | 46 | fetch+decode+open orchestration |
| `features/workspace/lib/workspace-slot-style.ts` | 36 | DOM mount-strategy styling (webview artifact) |

**544 lines total** across these 12 files.

### Liveness

Method and controls: see §0 (`workspace-scope-url.ts` is one of the four
stated controls). Everything in this area routes through `ide-shell.tsx`,
`workspace-host.tsx`, `workspace-view.tsx`, or `use-workspace-effects.ts` —
all part of the `WorkspaceHost` chain mounted unconditionally at
`ide-shell.tsx:184` (already established LIVE via the `crowbar-wordmark`
chain in `native/mapping/liveness-audit.md`). All 12 files verify LIVE; this
is the one area of the seven where every row lands the same way, because
workspace scoping is infrastructure every other area sits on top of.

| file | verdict | evidence |
|---|---|---|
| `lib/workspace-scope.ts` | LIVE | `recordWorkspaceScopeFromPath`/`setWorkspaceScope` called directly in `ide-shell.tsx:28`, on every navigation. |
| `lib/workspace-scope-url.ts` | LIVE | Control — see §0. `workspaceBase` has 20 non-test importers across every git/file/agent API module. |
| `lib/workspace/resolve-root-path.ts` | LIVE | `use-workspace-effects.ts` (always-mounted chain) and `terminal.tsx` (`TerminalHost` unconditionally mounted, `ide-shell.tsx`). |
| `lib/workspace/placeholder.ts` | LIVE | `isPlaceholderWorkspace` checked for every row in `workspace-tree-item.tsx` (the always-visible workspace list) and by `placeholder-toast-watcher.tsx`, unconditionally mounted at `ide-shell.tsx`. `placeholderReason`'s message specifically only renders for a placeholder row — a CONDITIONAL sub-case within a LIVE file, noted rather than folded away. |
| `lib/workspace/branch-workspace.ts` | LIVE | `findWorkspaceForBranch` used in `workspace-tree-item.tsx` and `repo-section.tsx`, both on the always-mounted workspace-tree chain. |
| `features/workspace/lib/keep-alive-policy.ts` | LIVE | `planRetention` called directly inside `workspace-host.tsx`'s armed-timer eviction logic — read the call site directly (`workspace-host.tsx:146-151`); fires routinely on workspace switches, not behind a rare toggle. |
| `features/workspace/lib/activation-freshness.ts` | LIVE | Called from `use-workspace-effects.ts`, `workspace-store-registry.ts`, and `workspace-view.tsx` — all on ordinary workspace-activation/-deactivation, ordinary use. (The doc's own Phase-4-vs-Tier-A boundary tension for this file, Finding 5 below, is a separate question from whether it's *reached* — it is.) |
| `features/workspace/lib/home-workspace-resolver.ts` | LIVE | `useHomeWorkspaceState`/`ensureHomeWorkspaceResolved` called directly and unconditionally in `ide-shell.tsx:84-86` (builds `homeWsIds` for the sidebar), **not only** from the home route (`routes/_shell/ide/$projectId/home.tsx`) as the file's own name might suggest — checked directly rather than assumed from the name. |
| `features/workspace/lib/external-buffer-sync.ts` | LIVE | `syncBufferWithDisk` called from `use-workspace-effects.ts` (always-mounted) and `lib/persistence/hydrate.ts`. |
| `features/workspace/lib/reset-workspace-scoped-stores.ts` | LIVE | Called directly in `workspace-view.tsx`, on every workspace switch. |
| `features/workspace/lib/open-file-content.ts` | LIVE | Called directly in `use-workspace-effects.ts`, ordinary file-open path. |
| `features/workspace/lib/workspace-slot-style.ts` | LIVE | `workspaceSlotStyling` called directly in `workspace-host.tsx` for every mounted workspace slot, including the one active workspace that always exists. |

### What is genuine, portable workspace-scoping logic

- **`workspace-scope.ts`'s pure half** — `parseWorkspaceScopeFromPath` (regex
  match of `/ide/:projectId/:repoId/:wsId` into a `WorkspaceScope`) is exactly
  "workspace/path scoping" as named. The registry half (`_scopes: Map`,
  `setWorkspaceScope`/`recordWorkspaceScope`/`getWorkspaceScope`,
  module-level `_activeWorkspaceId`) is a **plain, framework-free lookup
  table** — no React, no zustand, no gpui — explicitly documented as living
  outside the store graph *on purpose*. This is a clean, ready-made example of
  the kind of small stateful registry D2 wants moved into `crowbar-core`
  directly rather than wrapped as `Entity<T>`.
- **`workspace-scope-url.ts`** — `workspaceBase(wsId)` (scope → REST path,
  including the home-workspace special case with no `repoId`) and
  `isHomeWorkspace(wsId)`. Real scoping rules (hierarchical URL construction,
  what makes a workspace a "home" workspace), gpui-free. Note: §4.2's
  dependency table lets `crowbar-core` depend on `crowbar-client`, so this is
  a legitimate `crowbar-core` responsibility even though its output is a URL
  path string that `crowbar-client` ultimately sends over the wire — no
  crate-graph cycle.
- **`lib/workspace/placeholder.ts`** — `isPlaceholderWorkspace` (a workspace
  with no on-disk worktree, keyed on a documented *absence-of-field* signal,
  not a status enum) and `placeholderReason` (precedence: live-holder path >
  daemon-recorded error > generic retry message). Pure, gpui-free, though
  `placeholderReason`'s output is a user-facing sentence — logic and copy
  fused in one function, the brief's fourth-bucket pattern again, but small
  enough that separating them buys little.
- **`lib/workspace/branch-workspace.ts`** — `findWorkspaceForBranch`: exact
  case-sensitive branch→workspace-id lookup within a repo, explicitly
  excluding the default/main-worktree workspace. Small, pure, real git×
  workspace domain rule.
- **`keep-alive-policy.ts`'s `planRetention`** — the strongest Tier A
  candidate in this area: explicitly injected-clock, pure, documented
  invariants (active workspace always retained; window + hard-cap eviction;
  deterministic tie-breaking). Same shape as `patch-window.ts`'s
  `planWindow` (§1/§2) — a resource-retention scheduler, not merely
  subscription glue. Unlike `patch-window.ts`, there is no sibling crate this
  obviously belongs to instead (no `crowbar-workspace` crate exists), so
  `crowbar-core` is the natural home.

### What is entangled, Phase 4, or presentation

- **`resolve-root-path.ts`** — reads directly from two other zustand stores'
  `.getState()` and `window.location.hash`. Not pure; Phase 4/glue, though the
  underlying question ("what is this view's on-disk root") is a real
  workspace-scoping *concept* whose implementation today is entangled.
- **`home-workspace-resolver.ts`** — async fetch, an in-memory cache Map, and
  `useSyncExternalStore` subscription plumbing. This is the SAME
  "dependency-free module singleton" architectural pattern as
  `workspace-scope.ts` (the file's own comment says so), but doing network
  I/O + subscription rather than pure parsing — squarely Phase 4.
- **`activation-freshness.ts` — a genuine ambiguity, flagged rather than
  silently resolved.** Every function in it is technically gpui-free (Maps,
  timestamps, no framework import), which by §7.1's literal text ("if a
  store's logic can be tested without gpui, it belongs in core") would argue
  Tier A. But the brief's own bucket-3 test says *"if its substance is
  subscription, invalidation or effect ordering, it is not Tier A"* — and
  this file's entire purpose is deciding whether a re-subscribe should re-seed
  or reuse cached data, i.e. invalidation timing for the reactive graph. I
  read it as Phase 4 under the brief's substance test, but flag the tension
  with §7.1's more permissive wording explicitly: **the spec's own two
  statements of this rule do not agree at the margin**, and
  `activation-freshness.ts` sits exactly on that margin.
- **`external-buffer-sync.ts`** — real rules (own-write-echo suppression,
  clean-vs-dirty reconciliation, disk-equals-saved-content detection) but
  every function reads/writes a `WorkspaceStore`'s `.getState()`/`.setState()`
  and fires toasts. This is editor-buffer domain logic more than workspace
  *scoping*, and it is Phase-4-entangled in its current form — noted here
  because it doesn't fit any of the seven named areas cleanly and is real
  logic, not because it belongs in this bucket.
- **`reset-workspace-scoped-stores.ts`, `open-file-content.ts`** — Phase 4
  store-lifecycle and fetch-orchestration glue.
- **`workspace-slot-style.ts`** — despite being "pure" and unit-tested as
  such, its output is CSS (`display:none`/`display:contents`) and a DOM
  `inert` flag for a webview mounting strategy. Out of scope for the same
  reason as `appearance-bootstrap.ts` (§4): GPUI has no DOM to hide via CSS
  display — an inactive workspace's `Entity<T>` simply isn't rendered this
  frame. Not a port target even though the code itself is trivially pure.

### gpui-free?

Yes for the genuine set: `workspace-scope.ts`, `workspace-scope-url.ts`,
`placeholder.ts`, `branch-workspace.ts`, `keep-alive-policy.ts`. No DOM, no
React, no store import. `activation-freshness.ts` is gpui-free in the same
narrow sense but is Phase-4-shaped by substance (see above).

### Already done in `crowbar-proto`/`crowbar-client`

None. `WorkspaceScope` has no generated counterpart — unsurprising, since it
is a purely frontend-side derivation from the router URL, not a daemon
response shape.

### Tests

| test file | cases | covers |
|---|---|---|
| `lib/workspace-scope.test.ts` | 7 | registry + path parsing |
| `lib/workspace/placeholder.test.ts` | 8 | placeholder detection + reason |
| `lib/workspace/branch-workspace.test.ts` | 6 | branch→workspace lookup |
| `lib/workspace/resolve-root-path.test.ts` | 4 | (store-entangled, not core) |
| `features/workspace/lib/keep-alive-policy.test.ts` | 11 | `planRetention` |
| `features/workspace/lib/activation-freshness.test.ts` | 7 | (borderline, see above) |
| `features/workspace/lib/home-workspace-resolver.test.ts` | 5 | (Phase 4, not core) |
| `features/workspace/lib/external-buffer-sync.test.ts` | 7 | (Phase 4/editor-buffer, not core) |
| `features/workspace/lib/reset-workspace-scoped-stores.test.ts` | 3 | (Phase 4, not core) |
| `features/workspace/lib/workspace-slot-style.test.ts` | 3 | (presentation, not core) |

**‼️ Finding: `workspace-scope-url.ts` (`workspaceBase`, `isHomeWorkspace`) has
no dedicated test file** — no `workspace-scope-url.test.ts` exists anywhere
under `__tests__/`. Every workspace-scoped API call in the app runs through
`workspaceBase`, making it one of the most load-bearing functions in this
whole survey, and it is untested in isolation (only indirectly, through
whatever calls it in other suites).

**Workspace-scoping Tier A test total: 7+8+6+11 = 32 cases** across 4 files
(registry/parsing, placeholder, branch-lookup, retention policy) — plus
`activation-freshness.test.ts`'s 7 cases if the borderline call above is
read the other way (giving 39).

## 7. Review threads

### Where it lives

| file | lines | shape |
|---|---|---|
| `features/workspace/stores/slices/branch-review-slice.ts` | 156 | **the model**: `ReviewMessage`/`ReviewThread`/`ReviewConversation` types + a zustand `StateCreator` with CRUD mutators |
| `features/git/api/review-api.ts` | 288 | transport + **`mapThread`/`mapReply`/`mapConversation`**: wire (`ThreadDTO`/`ThreadReplyDTO`) → app-model reshape |
| `features/git/components/diff/use-review-annotations.tsx` | 395 | ~140 lines of pure positioning helpers + a 250-line hook |
| `features/workspace/stores/hooks/use-workspace-threads-stream.ts` | 61 | WS-stream subscription lifecycle (seed/subscribe/reconnect/cleanup) |
| `features/git/components/review-thread-item.tsx` | — | component, not counted |

**~900 lines total** across the four non-component files, of which roughly
**300–320 lines are genuine model/mapping/positioning logic** once the
transport and hook bodies are subtracted (see below).

### Liveness

Method and controls: see §0. The key check here is whether the thread *store*
and its *WS subscription* are live independent of whether a review pane is
currently open — they are: `use-workspace-threads-stream.ts` is imported by
`use-workspace-effects.ts` (the always-mounted `WorkspaceHost` chain, §6), so
every open workspace subscribes to its thread stream regardless of whether
any diff pane is visible, and `branch-review-slice.ts` is one of the slices
composed into `workspace-store.ts` for every workspace. Only the *display* of
threads (`review-thread-item.tsx`, `use-review-annotations.tsx`) is gated on
opening a review pane — matching `native/mapping/liveness-audit.md`'s own
already-published verdict for `review-thread-item.tsx` (used there as the
CONDITIONAL chain for the `alert-dialog`/`avatar`/`badge` surfaces), confirmed
consistent here rather than re-derived from scratch.

| file | verdict | evidence |
|---|---|---|
| `features/workspace/stores/slices/branch-review-slice.ts` | LIVE | `createBranchReviewSlice` composed into `workspace-store.ts` for every workspace (`workspace-store.ts:11`), independent of any pane being open. |
| `features/git/api/review-api.ts` | LIVE — mixed | `mapThread`/`listThreads` called directly from `use-workspace-threads-stream.ts` (LIVE, above) — genuinely exercised for every open workspace. The transport-only functions (`getReview`, `mergeIntoParent`, `setMergeStrategy`) are CONDITIONAL within the same file (only from `branch-review-pane.tsx`/`merge-popover.tsx`) — another mixed-file case. |
| `features/git/components/diff/use-review-annotations.tsx` | CONDITIONAL | Sole importer `review-code-view.tsx` (CONDITIONAL, §1 — only mounted inside a commit-diff/branch-review pane). |
| `features/workspace/stores/hooks/use-workspace-threads-stream.ts` | LIVE | Imported directly by `use-workspace-effects.ts` (always-mounted chain) — subscribes for every open workspace, not only while a review pane is visible. |
| `features/git/components/review-thread-item.tsx` | CONDITIONAL | Matches `native/mapping/liveness-audit.md`'s already-published verdict (its `alert-dialog`/`avatar`/`badge` rows trace this exact chain): sole importer `use-review-annotations.tsx` (CONDITIONAL, above). |

### What is genuine, portable review-thread-model logic

- **`branch-review-slice.ts`'s types** — `ReviewMessage{id,author,isAgent,
  body,createdAt}`, `ReviewThread{id,filePath,lineNumber,startLine,endLine,
  side,messages,isResolved}`, `ReviewConversation{id,title,age,isActive}`.
  This is "review-thread model" as named, almost verbatim.
- **`branch-review-slice.ts`'s mutators** — `addReviewThread`,
  `removeReviewThread`, `addReviewMessage`, `setReviewThreadResolved`,
  `upsertReviewThread` (insert-or-replace-by-id), `addReviewConversation`.
  Same D2 pattern as the file-tree-expansion store (§5) and the keymap store
  (§3): underneath the `StateCreator`/immer wrapper, every mutator is a
  trivial pure `ReviewThread[] → ReviewThread[]` transition (filter-by-id,
  find-and-mutate, upsert-by-id). Portable as plain functions with the
  reactive shell left to `crowbar-state`.
- **`review-api.ts`'s `mapThread`/`mapReply`/`mapConversation`** — the
  genuinely nontrivial piece: reshaping the wire's root-comment-at-top-level +
  `replies[]` structure (`ThreadDTO`) into the app's uniform `messages[]`
  array with the root prepended, including two real fallback rules (prefer
  `t.messageId`, else synthesize `${t.id}:root`; prefer `startLine`/`endLine`,
  else fall back to `line` for both). Pure, gpui-free, and the exact kind of
  "wire shape ≠ app shape" reconciliation a `crowbar-core` model type should
  own.
- **`use-review-annotations.tsx`'s pure helpers (lines 58–115, ~90 lines)** —
  `isDraftThread`, `toAnnotationSide`/`toThreadSide` (the `old`/`new` ↔
  renderer's two-sided line addressing map — the file's own comment warns an
  inversion here "puts every comment on the wrong half of the diff", i.e. a
  correctness-critical mapping), `threadToAnnotation`/`annotationToThread`
  (wrap/unwrap), `groupAnnotationsByPath` (bucket threads by file, sorted by
  line), `countThreadsByPath` (per-file thread counts, deliberately computed
  from the thread store alone so it is correct even for an unloaded file).
  All pure, gpui-free — **but** typed against `@pierre/diffs`'s
  `DiffLineAnnotation<T>`/`AnnotationSide`, the same library-type entanglement
  found in §1/§2's placeholder-hunk-geometry functions. The concept (how a
  thread anchors to a diff side/line, and how threads group per file) is
  portable; these specific functions are not, verbatim, because their types
  belong to a library `crowbar-diff` replaces.

### What is Phase 4 or presentation

- **`review-api.ts`'s transport functions** (`getReview`, `getReviewFiles`,
  `mergeIntoParent`, `setMergeStrategy`, `listThreads`, `openThread`,
  `replyToThread`, `setThreadResolved`, `deleteThread`, `deleteMessage`,
  `editMessage`) — `crowbar-client` territory, not `crowbar-core`.
- **`use-review-annotations.tsx`'s `useReviewAnnotations` hook (lines
  142–390, ~250 lines)** — store subscription (`useWorkspaceStoreContext`),
  draft-comment state (`useState`), submit/cancel handlers that call the
  transport functions, and JSX renderers (`renderAnnotation`,
  `renderGutterUtility`). Phase 4 + presentation, not core. `composerTitle`
  (line 391) is a one-line label formatter — presentation.
- **`use-workspace-threads-stream.ts`** — WS subscription lifecycle (seed,
  subscribe, reconnect-triggered re-seed, cleanup on unmount/wsId change).
  Textbook §7 "subscription, invalidation, effect ordering" — Phase 4 by the
  brief's own test, unambiguously (unlike `activation-freshness.ts` in §6,
  there's no ambiguity here: this file's substance is entirely the
  subscribe/reconnect/cleanup lifecycle). Notable for what it reveals about
  the daemon's push model though: threads arrive over a workspace-scoped WS
  stream with a `deleted` tombstone flag for removal and a `{reconnected:
  true}` sentinel that triggers a full re-seed — a real wire-protocol
  behaviour `crowbar-client`/`crowbar-state` will need to reproduce, even
  though the reconciliation code itself doesn't port as pure logic.

### gpui-free?

Yes for `branch-review-slice.ts`'s types + mutator bodies, `mapThread`/
`mapReply`/`mapConversation`, and `use-review-annotations.tsx`'s six pure
helpers (modulo the `@pierre/diffs` type entanglement noted above, which
affects the concrete types but not the underlying logic).

### Already done in `crowbar-proto`

`native/crates/crowbar-proto/src/generated/api_v0_dto.rs` already has
`ThreadDTO` (line 315), `ThreadReplyDTO` (line 342), and `ReviewThreadDTO`
(line 268) — the wire shapes are generated. **What is not done, and has no
counterpart, is the app-side `ReviewThread`/`ReviewMessage`/
`ReviewConversation` model and the `mapThread` reshape between the two** —
this is genuinely new Tier A content, not a duplicate, the same pattern seen
in git model (§1: types mostly duplicate proto; mapping logic mostly does
not).

### Tests

| test file | cases | covers |
|---|---|---|
| `features/workspace/stores/branch-review-slice.test.ts` | 11 | slice mutators (add/remove/upsert/resolve) |
| `features/git/api/review-api.test.ts` | 21 total, **8** test `mapThread` specifically (the `describe('mapThread', …)` block); the other 13 test transport request shapes |
| `features/git/components/diff/use-review-annotations.test.tsx` | 22 total, **8** test the pure helpers (`describe`s: "side map", "threadToAnnotation / annotationToThread", "grouping threads by file"); the other 14 test the hook/component |
| `features/workspace/stores/hooks/use-workspace-threads-stream.test.ts` | 8 | (Phase 4, not core) |
| `features/git/components/review-thread-item.test.tsx` | 19 | component, not core |
| `features/panes/components/comment-composer.test.tsx` | 7 | component, not core |

**Review-thread Tier A test total: 11+8+8 = 27 cases** across 3 files (slice
CRUD, `mapThread`, annotation-positioning helpers).

---

## Theme tokens (also named in §16 Phase 3 Tier A, alongside `core`/`proto`/`client`)

### ‼️ Finding: "theme tokens" as named cannot be `crowbar-core` work, by the spec's own §6.1

§16 lists Tier A as *"`core`, `proto`, `client`, theme tokens — gated by
ported tests"* — grouping theme tokens with the three crates that share D2's
gpui-free constraint. But §6.1 defines the token types themselves as **gpui
wrappers**:

```rust
pub struct Color(gpui::Hsla);          // inner field PRIVATE
```

A type whose only field is `gpui::Hsla` cannot exist in a crate that
`scripts/check-invariants.sh` greps for a `gpui` dependency and fails the
build on a match (§4.3 rule 1). §4.2's own crate table agrees:
`crowbar-ui` — not `crowbar-core` — is described as *"design system: `Theme`,
token newtypes, primitives over `gpui-component`"* and may depend on `core,
gpui, gpui-component`; its coverage gate is **"oracle corpus,"** not the
≥98%-lines hard-fail gate §16's phrasing implies. So "theme tokens" named
alongside `core`/`proto`/`client` in §16 is misleading as written: the sealed
token *types* are `crowbar-ui` work under a different, non-line-coverage gate.
**What legitimately belongs in `crowbar-core` is the gpui-free arithmetic
`crowbar-ui` converts at its boundary** — and `crowbar-core/src/color.rs`
already states this split explicitly in its own module doc: *"It lives here
rather than in `crowbar-ui` because it is arithmetic on four floats: no
window, no framework, no `gpui` … `crowbar-ui` converts at its boundary."*
`color.rs` is not a stray inclusion; it is the template for exactly this
area, already applied once.

### Where the React-side token-adjacent logic lives

| file | lines | shape |
|---|---|---|
| `styles/theme.css` | 479 | 264 `--` declarations (§3.3) — **static CSS data, not code** |
| `styles/zen.css` | 30 | 9 declarations |
| `styles/editor-theme.css` | 71 | 1 declaration |
| `extensions/themes/theme-registry.ts` | 123 | `ThemeRegistry` class — DOM-driven (`document.documentElement`), calls the already-out-of-scope `appearance-bootstrap.ts` |
| `extensions/themes/types.ts` | 91 | `ThemeTokens` (dead stub, never imported outside its own file) + `ThemeDefinition` (real, but embeds `React.ReactNode` for `icon`) |
| `features/editor/theme/resolve-css-color.ts` | 188 | **`cssColorToHex`/`oklchToHex`** (pure) + `resolveCssVar`/`readSyntaxPalette`/`readTerminalPalette` (DOM-entangled) |

### Liveness

Method and controls: see §0. `theme-registry.ts` is imported directly by
`settings-normalization.ts` (boot-time, §4), settling this whole area's
anchor file as LIVE by the same chain that settled most of Settings.

| file | verdict | evidence |
|---|---|---|
| `styles/theme.css` | LIVE | `index.css` `@import`s it unconditionally (`index.css:3`); `index.css` is the app's base stylesheet. Base `:root`/`.dark` selectors apply regardless of the active theme. |
| `styles/zen.css` | CONDITIONAL | Its declarations are scoped entirely under `[data-theme='zen']:not(.dark)` etc. — always shipped in the bundle, but only takes effect once the user selects the "Zen" theme specifically (confirmed by reading the file's own selectors, not the doc's prose). |
| `styles/editor-theme.css` | LIVE | `index.css:5`, unconditional; mostly `@font-face` declarations that apply regardless of theme. |
| `extensions/themes/theme-registry.ts` | LIVE | `themeRegistry` imported directly by `settings-normalization.ts` (boot chain, §4) and by the always-mounted terminal (`use-terminal-connection.ts`). |
| `extensions/themes/types.ts` | LIVE — but one dead export | `ThemeDefinition` is real and live (flows through `theme-registry.ts`, above). `ThemeTokens` is confirmed dead exactly as the doc's own prose already says — grepped independently: it appears only in its own declaration and as an unused optional field (`tokens?: ThemeTokens`) on `ThemeDefinition`, never populated or read anywhere. Another "live file, dead export" case, consistent with the doc's own prior note. |
| `features/editor/theme/resolve-css-color.ts` | LIVE | `use-terminal-theme.ts` imports it directly; the terminal is unconditionally mounted (`TerminalHost`, `ide-shell.tsx`). (Its other two consumers — `mermaid-theme.ts` for Plate markdown mermaid blocks, `monaco/define-theme.ts` for the Monaco engine — are each their own CONDITIONAL sub-case, immaterial to the file-level verdict since the terminal chain alone makes it LIVE.) |

### What is genuine, portable, gpui-free color arithmetic — and a real gap

- **`cssColorToHex`/`oklchToHex`/`gammaEncode`/`expandShortHex`/`parseAlpha`/
  `toHexByte`/`clamp255`** in `resolve-css-color.ts` — pure string/float
  parsing and an OKLCH→sRGB conversion (Björn Ottosson's reference algorithm),
  zero DOM, zero framework. This is the *other half* of the CSS-color-math
  problem `crowbar-core/src/color.rs` already solves one piece of.
- **`theme.css` uses `oklch()` 37 times and `color-mix()` 6 times** (measured
  directly with `grep -c`). `crowbar-core/src/color.rs` (309 lines, 13
  `#[test]`s, part of the crate's 100%-but-vacuous coverage per QUEUE.md)
  implements `color-mix(in srgb, …)` per CSS Color 5 §3, exactly — but **has
  no OKLCH-to-sRGB conversion**. Since the majority of `theme.css`'s actual
  color *values* are authored in `oklch()`, not `color-mix()`, **the existing
  `crowbar-core` color arithmetic cannot evaluate most of the tokens it will
  need to seal into `crowbar-ui::Color` without this second, currently-
  unported conversion.** This is a concrete, actionable gap in what little
  Tier A work already exists, not a hypothetical.
- **`ThemeDefinition`'s non-color fields** (`id`, `name`, `type`, `isDark`,
  `category`) are a legitimate small schema (theme metadata), gpui-free
  once `icon?: React.ReactNode` and the dead `tokens?: ThemeTokens` field are
  dropped.

### What is not core

- **`ThemeRegistry`** — DOM class-toggling (`data-theme`, `.dark`), calls
  into `appearance-bootstrap.ts` (§4, flagged out of scope as a webview FOUC
  mechanism). §6.1 replaces this entire mechanism: a native `Theme` struct is
  resolved once into `crowbar-ui`, not looked up via `[data-theme]` CSS
  selectors at paint time.
- **`resolveCssVar`/`readSyntaxPalette`/`readTerminalPalette`** — exist only
  because "Monaco and xterm cannot read CSS variables" (the file's own
  comment) and so must resolve them off the live DOM via
  `getComputedStyle()` and a temporary-element `var()`-resolution trick. This
  entire class of problem is a webview artifact with no native counterpart:
  `crowbar-terminal`/`crowbar-editor` read `theme.foreground`/
  `theme.syntax.keyword` directly off the sealed `Theme` struct, never off a
  computed style.
- **`ThemeTokens`** — dead. Marked `// Stub` in its own source, never
  imported outside `types.ts`. Not a port target; noted so it is not mistaken
  for an existing "strongly-typed token map" that merely needs translating —
  it was never wired up in the first place.
- **`274` (§3.3) is a count of CSS custom-property *declarations*, i.e.
  data**, not lines of logic. It sets the size of the `Theme` struct's field
  list in `crowbar-ui`, which is not this survey's crate.

### Tests

| test file | cases | covers |
|---|---|---|
| `resolve-css-color.test.ts` | 13 total, **8** (`describe('cssColorToHex', …)`) test the pure parser/converter; **5** (`describe('DOM resolver', …)`) test the DOM-entangled half |

**Theme-tokens-adjacent Tier A test total: 8 cases**, all in one file, all
already-portable color-math tests that would extend `crowbar-core/src/
color.rs`'s existing 13 `#[test]`s rather than start a new file.

---

## The headline denominator

**Status: COMPLETE.** All seven named areas plus theme tokens measured.
**Amended 2026-08-04 (P3.69): the ~3,170-line "Tier A core" figure below
predates this file's liveness pass (§0) and was computed from line counts and
classification alone — the same gap that let two dead files ship. See
"Liveness reconciliation" after the per-area breakdown for what that figure
looks like once every row carries a verdict.**
Method note on precision: whole-file line counts below are exact `wc -l`
measurements. Several files are *mixed* — genuine Tier A logic sitting beside
presentation, DOM code, or a React component in the same file (`
review-code-view.tsx`, `review-api.ts`, `use-review-annotations.tsx`,
`resolve-css-color.ts`, `file-tree-git-status.ts`, `file-tree-density.ts`,
`settings-download.ts`). For those, the line count given is an estimate of
the pure-logic region only, stated as such — flagged rather than folded
silently into an exact-looking total, per this survey's own standard for
evidence.

### Total surveyed (all seven areas + theme tokens, non-component-total)

| area | lines surveyed | files touched |
|---|---|---|
| Git model + diff algebra | ~1,190 | 9 (incl. embedded region) |
| Keymap resolution | 733 | 10 |
| Settings schema | 2,277 | 22 |
| File-tree model | ~2,002 | 18 |
| Workspace scoping | 544 | 12 |
| Review threads | ~900 | 4 |
| Theme tokens (React-side) | ~402 (+580 CSS data, not code) | 3 |
| **Total** | **~8,048 lines, ~78 files** | |

### Split by bucket

| bucket | files | lines (approx. where noted) | ported-able test cases |
|---|---|---|---|
| **Tier A core** (genuine, gpui-free, portable) | **36** (30 whole-file + 6 with an embedded pure region) | **~3,170** (2,396 exact whole-file + ~773 estimated embedded) | **221** |
| Phase 4 state (`crowbar-state`, reactive/subscription/effect-ordering) | ~30 (illustrative, not exhaustively itemized — see note) | ~2,760 | not tallied (component/hook tests, not portable as unit logic) |
| Already done (`crowbar-proto` generated DTOs) | 0 new — 3 of 7 areas' type files duplicate existing generated types | n/a (see below) | n/a |
| Belongs to a different Tier-A-adjacent crate (`crowbar-diff` logic, §12) | 2 | 316 | 32 |
| Presentation (labels, CSS classes, static copy) | ~7 | ~460 | not tallied |
| Out of scope (D6 persistence, webview-only DOM/FOUC mechanisms, deleted IPC) | 8 | ~882 | not tallied |

**Tier A core: 36 files, ~3,170 lines, 221 ported-able test cases.** This is
the number the survey exists to produce. Per area:

| area | Tier A files | Tier A lines (approx.) | Tier A test cases |
|---|---|---|---|
| Git model (incl. diff-algebra's real content) | 6 whole + 1 embedded (`review-code-view.tsx`, ~368 lines) | 241 + ~368 = ~609 | 46 |
| Diff algebra (standalone) | 0 — folded into git model above | 0 | 0 |
| Keymap resolution | 5 | 516 | 16 |
| Settings schema | 9 | 629 | 41 |
| File-tree model | 4 whole + 2 embedded (`file-tree-git-status.ts` ~110, `file-tree-density.ts` ~15) | 593 + ~125 = ~718 | 51 |
| Workspace scoping | 5 | 261 | 32 |
| Review threads | 1 whole (`branch-review-slice.ts`) + 2 embedded (`review-api.ts` ~60, `use-review-annotations.tsx` ~90) | 156 + ~150 = ~306 | 27 |
| Theme tokens (React-side color math) | 1 embedded (`resolve-css-color.ts` ~130) | ~130 | 8 |
| **Total** | **36** | **~3,170** | **221** |

**Phase 4 note:** the Phase-4 bucket total (~2,760 lines, ~30 files) is
illustrative, built from the files this survey actually opened and
classified — it is not a floor-to-ceiling audit of every hook and store
touching these seven areas (e.g. individual drag-drop/inline-editing DOM
handlers were read but not line-audited component-by-component). Where the
brief's own boundary is ambiguous (`activation-freshness.ts`, §6), the case
is counted toward Phase 4 in this table but the ambiguity is preserved in
prose.

**"Already done" has no line count because it isn't new lines — it's
evidence that three of seven areas' hand-written TS type files (`git-types.ts`
+`git-diff-types.ts`, `file-tree-api.ts`'s `FileNodeDTO`, `review-api.ts`'s
`ThreadDTO`/`ThreadReplyDTO`) restate shapes `crowbar-proto` already generated
from the Go handlers** (`domain_git.rs`, `domain.rs`, `api_v0_dto.rs`).
Keymap resolution, settings schema, workspace scoping and theme tokens have
**no** daemon-side counterpart — they are frontend-local concepts with
nothing to duplicate, which is itself worth knowing before scoping a Tier A
work item as "port the types."

### Liveness reconciliation (P3.69)

The ~3,170-line figure above was never checked against reachability — that is
this whole item's reason for existing (see §0). Two independent questions,
kept separate rather than blended into one adjusted total:

**1. How much of the ~3,170 was dead code, by the figure's own internal math?**
The git-model per-area row states "6 whole files, 241 lines." The prose's own
"genuine, portable git-model logic" bullets name exactly five things —
`resolveBranchAction`, the two changed-files projections, `buildGitFolderTree`,
and `getFileStatus` — which are `branch-action.ts`(49) +
`git-status-to-changed-files.ts`(45) + `review-file-summary-to-git-diff.ts`(41)
+ `build-git-folder-tree.ts`(57) + `git-diff-helpers.ts`(11) = **203 lines**,
38 short of 241. `normalize-diff.ts` is exactly 38 lines and is the file P3.67
dispatched alongside the five named ones. **This is offered as the most
textually plausible reconstruction of the sixth file, not a proven one** —
checked computationally, 9 different subsets of git model's 13 rows sum to
241, so the arithmetic alone does not uniquely determine it. If this
reconstruction is right, **38 of the ~3,170 lines were counted as Tier A core
and are DEAD** (`normalize-diff.ts`). `diff-buffer-path.ts` (24 lines) is
*not* part of any plausible reading of the 241 — the prose explicitly places
it under "What is not git-model logic" ("tab/buffer identity logic..., not
git model") — yet P3.67 dispatched it for porting anyway, alongside
`normalize-diff.ts`. That is a second, independent failure on top of the
liveness gap: a file the survey itself had already ruled out of scope got
ported regardless, evidently because the dispatch pulled six names from the
file-list table (§1's "Where it lives") rather than from the survey's own
classification — exactly `native/QUEUE.md`'s own diagnosis ("I took these six
straight from `tier-a-denominator.md`, which counts lines and never asked
whether anything reaches them").

**2. How much of the whole survey (not just the 3,170 bucket) is dead, now
that every row has been checked?** **138 of 9,585 lined rows (1.4%) — see §0's
tally.** This is the number that matters going forward, because it is not
retrospective bookkeeping about one already-superseded subtotal: `normalize-
diff.ts`(38) and `diff-buffer-path.ts`(24) are the two files already ported
(§0, `native/mapping/core-git.md` §§2–3); `diff-search.ts`(72) is a **new
finding** from this pass — never dispatched, sitting in the separate
`crowbar-diff`-logic 316-line bucket, but discussed in §2 as real reachable
logic without the liveness check that would have caught it; `hooks/use-
command-shortcut.ts`(4) is a dead-by-construction stub the original doc
already correctly called out as a stub but never gave a DEAD verdict to.

**The conservative, fully-defensible headline: at minimum 62 lines (2.0% of
the original 3,170) of what was labelled "Tier A core" and dispatched for
porting was dead code — both files QUEUE.md's own P3.67 section already named.
Across the full, now-liveness-checked survey, 138 of 9,585 surveyed lines
(1.4%) are dead.** Small in proportion either way — but the size was never
the point; the point is that the number was never checked, and now every row
has been.

## Findings — corrections to the brief

1. **§10.1's `features/git/utils/git-diff-parser.ts` does not exist.** No
   file by that name anywhere in `web/src`, and no hand-rolled unified-diff
   string parser exists at all. The daemon returns structured diff data
   (`GitDiff.lines`) for the sidebar/status path — no parser needed there —
   and the one place raw patch text *is* parsed (the windowed Branch Review
   surface) delegates to the third-party `@pierre/diffs` library's
   `parsePatchFiles`, not first-party code. §10.1's conditional resolves
   correctly ("no algorithm needed") but names a mechanism that isn't real.

2. **"Diff algebra" barely exists as a distinct area.** What looks like it in
   the React app splits three ways: (a) small, real file-status
   classification logic, already counted under git model; (b) placeholder
   hunk-geometry sizing math whose *concept* is portable but whose types
   belong to `@pierre/diffs`, a library being deleted; (c) viewport
   windowing (`patch-window.ts`) and diff-text search (`diff-search.ts`) that
   are genuinely pure and gpui-free but scoped to `crowbar-diff`'s own logic
   partition per §4.2/§12, not `crowbar-core` — 316 lines and 32 tests that
   are real Tier-A-*shaped* work, just not this crate's.

3. **"Theme tokens" named in §16 alongside `core`/`proto`/`client` cannot be
   `crowbar-core` work, by the spec's own §6.1.** The sealed token types are
   literal `gpui::Hsla` wrappers (`pub struct Color(gpui::Hsla)`), so they
   cannot exist in a crate `check-invariants.sh` greps for `gpui` and fails
   the build on a match. §4.2 independently assigns theme/token work to
   `crowbar-ui` (may depend on `gpui`), gated by "oracle corpus," not the
   line-coverage bar §16's phrasing implies for Tier A. What legitimately
   lives in `crowbar-core` is the *arithmetic* `crowbar-ui` converts at its
   boundary — and `crowbar-core/src/color.rs` already says so in its own
   module doc, making it the template for this split rather than a stray
   inclusion. **A concrete, currently-real gap follows from this**:
   `theme.css` uses `oklch()` 37 times and `color-mix()` 6 times, but
   `color.rs` only implements `color-mix()` — the OKLCH→sRGB conversion that
   most of the app's actual color *values* need is unported, and exists
   today as pure, tested TS (`resolve-css-color.ts`'s `oklchToHex`, 8
   ported-able test cases) that nobody has looked at yet.

4. **D2's own named example is real and findable.** §1 of the spec says pulling
   "selection logic, tree-expansion state, and similar" out of components
   into core is a consequence of D2. `features/file-explorer/file-explorer/
   stores/file-explorer-tree-store.ts` is exactly this: a zustand store whose
   mutator bodies, underneath the `create/immer/combine` wrapper, are plain
   `Set<string> → Set<string>` transitions. The same pattern repeats in
   `features/workspace/stores/slices/branch-review-slice.ts` (review-thread
   CRUD) and `features/keymaps/stores/store.ts` (override CRUD) — three
   independent zustand stores across three different areas whose *mutator
   logic* is Tier A even though the file as a whole is a Phase 4 store. This
   is a recurring shape worth naming as a pattern, not three coincidences.

5. **§7.1 and the brief's own bucket-3 test disagree at the margin, and
   `activation-freshness.ts` sits exactly on it.** §7.1: *"If a store's logic
   can be tested without gpui, it belongs in core."* The brief: *"if its
   substance is subscription, invalidation or effect ordering, it is not Tier
   A."* `activation-freshness.ts` (§6) is gpui-free by the first test and
   substantively invalidation-timing by the second. I read it as Phase 4, but
   this is a real, not manufactured, ambiguity in the governing rules and
   should be resolved explicitly rather than by whichever surveyor hits it
   first.

6. **A nested re-export shim tree exists under `features/file-explorer/`,
   the file-tree analogue of "don't trust a directory listing."**
   `features/file-explorer/{lib,stores,utils}/*.ts` are all 1-line
   `export * from '../file-explorer/…'` shims over
   `features/file-explorer/file-explorer/{lib,stores,utils}/*.ts`, which hold
   the real content. An `ls`-driven count of the outer directory would double
   every line in this area.

7. **Three areas (git model, file-tree model, review threads) mostly
   duplicate DTOs `crowbar-proto` already generated; four areas (keymap
   resolution, settings schema, workspace scoping, theme tokens) have no
   daemon-side counterpart at all.** A work item that reads "port `git-types.
   ts`" is mostly re-typing what Phase 0's codegen already produced; a work
   item that reads "port `types/settings.ts`" is genuinely new surface. These
   are different-sized and different-shaped tasks even though both are
   "types," and the crate description's flat list of seven areas doesn't
   distinguish them.

8. **Two load-bearing functions have zero dedicated tests**, despite §16
   gating Tier A on "ported tests": `features/keymaps/utils/effective-
   keymaps.ts` (the literal keymap-resolution algorithm —
   `resolveBinding`/`getEffectiveBindings`/`findConflictingCommands`) and
   `lib/workspace-scope-url.ts` (`workspaceBase`, which every
   workspace-scoped API call in the app runs through). Both would need new
   tests authored, not ported — a materially different kind of Tier A work
   than the 221-case core this survey otherwise found.

9. **The brief's fourth bucket (presentation fused with logic) shows up
   repeatedly, in the same shape each time**: a function returns a real
   classification (git file status, tree-row density) *and* a hardcoded
   CSS-class/color string in the same return value
   (`file-tree-git-status.ts`'s `getFileTreeGitStatusDecoration`,
   `git-diff-helpers.ts`'s `getFileStatus`/`getImgSrc` sharing one 11-line
   file). §6.1's sealed token types are precisely what separates these two
   concerns at the type-system level in the port; today's React code has no
   such enforcement and mixes them by convenience.

10. **This survey shipped without a liveness check, and that gap reached
    production.** §0 (P3.69) is the fix: every row in every table above now
    carries a LIVE/CONDITIONAL/DEAD/UNCERTAIN verdict, with evidence. Three
    things fell out of doing that work that would not otherwise have
    surfaced:
    - **`diff-buffer-path.ts` was dispatched for porting despite this
      survey's own §1 prose explicitly ruling it "not git-model logic."**
      `normalize-diff.ts` and `diff-buffer-path.ts` are the two files
      `native/QUEUE.md`'s P3.67 section already found dead; this pass adds
      that the second one was never even in-scope by the survey's own
      classification, only by its file-list table — the file list and the
      classification disagreed, and the dispatch followed the list.
    - **`diff-search.ts` is a third dead file** (§1/§2), never dispatched
      (it wasn't in the 3,170-line bucket to begin with) but discussed in §2
      as real, reachable logic with no liveness caveat.
    - **A live file can hold entirely dead exports**, and this survey's own
      prose was fooled by it once: `file-explorer-tree-utils.ts` (§5) is
      LIVE only because of `getExplorerTargetPath`, an export the original
      "genuine, portable" bullet for this file never names — the four
      functions that bullet *does* name (`filterHiddenFiles`,
      `addNewItemToTree`, `removeEditingItemsFromTree`,
      `getAncestorDirectoryPaths`) are either called nowhere at all or
      shadowed by an unrelated same-named function elsewhere. `getAncestor-
      DirectoryPaths` even has its own passing test exercising the
      unreachable original — the same "tested but unreachable" shape
      `native/mapping/core-git.md` documents for the already-ported
      `diff-buffer-path.ts`.
