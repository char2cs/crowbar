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
| `registry.ts` | 220 | `COMMANDS: Command[]` (**20** commands — this row said 19 until P3.70's compiler rejected a `[Command; 19]` binding; verified independently) + `getCommand`, `CATEGORY_ORDER` — static data + lookup |
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

**Correction to the 62-line figure above, found while building the
reconciliation below: it isn't a clean "dead-in-scope" number.** Of the two
files it names, only `normalize-diff.ts` (38 lines) is a plausible member of
the ~3,170 Tier A core bucket at all (§1's own "genuine, portable" bullets
don't name it, but its 38 lines are the only unaccounted balance in the
git-model subtotal — see above). `diff-buffer-path.ts` (24 lines) was **never
in the 3,170 bucket to begin with**, by this document's own §1 prose ("What is
not git-model logic … tab/buffer identity logic … not git model"); it was
ported despite being explicitly ruled out, which is a scope error independent
of whether the ported code is reachable. So "62 lines of the 3,170 were dead"
overstates the denominator-specific damage: at most 38 of the ~3,170 (1.2%)
were both (a) inside the bucket and (b) dead; the other 24 were never inside
the bucket, dead or not.

### Denominator reconciliation (P3.71) — settling the ~3,170-vs-9,447 discrepancy

`native/QUEUE.md`'s P3.69 entry flagged, and refused to quote past, a
denominator that "moved by 3×": ~3,170 (this file's own "Tier A core"
headline, above) against **9,447** (LIVE + CONDITIONAL from §0's tally, 5,751
+ 3,696). This section settles it, checking the four candidate explanations
QUEUE.md's own brief for this item listed, in turn, rather than picking one.

**Double-counting, checked first and ruled out.** Every file path in every
"Where it lives" table across all seven areas plus theme tokens (89 rows,
9,585 lines by direct re-extraction — one row off the tally's stated 90,
immaterial, the line total matches exactly) was collected and compared
pairwise across areas. **Zero paths repeat.** No file is counted twice under
two different area headings; the discrepancy is not double-counting.

**Not "~3,170 was scoped to git model alone."** The headline's own per-area
breakdown (above) already shows all seven areas plus theme tokens
contributing to the ~3,170 — git model (~609), keymap (516), settings (629),
file-tree (~718), workspace (261), review threads (~306), theme tokens
(~130). It was never narrower than "all seven areas," just narrower in *what
counts as core within each*.

**Not "~3,170 only counted tested files."** The opposite is closer to true:
`effective-keymaps.ts` and `workspace-scope-url.ts` — zero dedicated tests
each — were counted in Tier A core anyway (Findings 8, and QUEUE.md's own
recommendation to dispatch keymap resolution next despite this). Test
coverage was never the filter.

**Not "~3,170 was simply wrong."** Cross-referencing every Tier A core file
against its P3.69 liveness verdict (table below) shows **at least 3,131 of
the ~3,169 core lines (98.8%) are LIVE or CONDITIONAL** — real, reachable
code, not a stale or invented figure. The one open question is a single
ambiguous 38-line file in git model (§ above — "the most textually plausible
reconstruction, not a proven one"), the same ambiguity already on record, not
a new one.

**The real explanation is scope *and* method, together, and both are
legitimate — the two figures answer different questions on purpose:**

1. **Scope.** ~3,170 is the **Tier A core** bucket only — the subset of all
   seven areas' surveyed surface classified as genuine, portable, gpui-free
   `crowbar-core` domain logic (the "Split by bucket" table, above). 9,447 is
   LIVE + CONDITIONAL summed over **every** bucket in the same seven areas:
   Tier A core **and** Phase 4 state (~2,760 lines — `crowbar-state`'s job,
   not `crowbar-core`'s), **and** the `crowbar-diff`-adjacent logic bucket
   (316 lines — a different crate, §12), **and** presentation (~460 lines —
   not portable at all), **and** out-of-scope webview/D6 mechanisms (~882
   lines — deleted by design, ported to *nothing*). Most of the 9,447 was
   never a candidate for the Tier A denominator; it was surveyed because the
   file happened to sit under one of the seven feature directories, not
   because it was ever classified as `crowbar-core` work.
2. **Counting method for mixed files.** ~3,170 uses a **reduced estimate of
   the pure-logic region only** for six mixed files (e.g. `review-code-
   view.tsx`'s Tier A content is counted as ~368 of its 1,179 lines). The §0
   tally that produces 9,447/9,585 uses the **whole-file** count for the same
   files (all 1,179 of `review-code-view.tsx`), stated explicitly at the top
   of §0's tally note. Both are legitimate measurements of different things,
   kept separate rather than blended, per this document's own standing
   practice.

**Reconciliation table.** Two views of the same 89 rows: (A) the whole
survey, all buckets, whole-file basis — what §0's tally measures; (B) the
Tier A core subset only, the ~3,170 that was and should continue to be
labelled the Tier A target — cross-referenced here for the first time against
its own P3.69 liveness verdicts, file by file.

**(A) Whole survey, all buckets (matches §0's tally exactly):**

| area | total lines | LIVE | CONDITIONAL | DEAD |
|---|---|---|---|---|
| Git model + diff algebra | 2,001 | 229 | 1,638 | 134 |
| Keymap resolution | 733 | 729 | 0 | 4 |
| Settings schema | 2,277 | 1,390 | 887 | 0 |
| File-tree model | 2,002 | 1,256 | 746 | 0 |
| Workspace scoping | 690† | 690 | 0 | 0 |
| Review threads | 900 | 505 | 395 | 0 |
| Theme tokens | 982 | 952 | 30 | 0 |
| **Total** | **9,585** | **5,751** | **3,696** | **138** |

†**A second line-counting bug, found while building this table.** §6's own
prose states "**544 lines total** across these 12 files," but the 12 rows in
§6's own "Where it lives" table sum to **690** (87+28+31+32+16+98+90+89+90+47
+46+36 = 690), not 544 — a 146-line arithmetic error in the original survey,
propagated into the "~8,048 lines surveyed" headline below. It does **not**
touch the ~3,170 Tier A core figure, which was computed independently from a
named 5-file subset (261 lines, verified separately) rather than from this
mis-summed area total. Left as further evidence for the method warning this
whole item was issued under: even a survey that logged its work in six
committed increments got one `+` wrong.

**(B) Tier A core only (⊂ the ~3,170), cross-referenced against P3.69's
verdicts, file by file:**

| area | Tier A core lines | LIVE | CONDITIONAL | DEAD / ambiguous |
|---|---|---|---|---|
| Git model + diff algebra | 609 | 151 (`branch-action.ts` 49, `git-status-to-changed-files.ts` 45, `build-git-folder-tree.ts` 57) | 420 (`review-file-summary-to-git-diff.ts` 41, `git-diff-helpers.ts` 11, `review-code-view.tsx` embedded ~368) | ≤38 — the 6th, unnamed whole file; 9 subsets of the area's 13 rows sum to 241, so this is not uniquely provable, but `normalize-diff.ts` (exactly 38 lines, independently confirmed DEAD) is the most textually plausible candidate |
| Keymap resolution | 516 | 516 (all 5 files: `types.ts`, `registry.ts`, `keybinding-presets.ts`, `chord.ts`, `effective-keymaps.ts`) | 0 | 0 |
| Settings schema | 629 | 554 (`types/settings.ts` 81, `types/feature.ts` 3, `default-settings.ts` 98, `typography-defaults.ts` 25, `settings-normalization.ts` 249, `font-family-resolution.ts` 40, `markdown-font-size.ts` 26, `ui-font-size.ts` 32) | 75 (`settings-import-export.ts`) | 0 — **unlike git model, this 9-file reconstruction is unique**: no other pair of the area's remaining 17 rows sums to the required 28-line balance |
| File-tree model | 718 | 718* (4 whole: `visible-file-tree-rows.ts` 238, `file-tree-gitignore.ts` 237, `file-explorer-tree-utils.ts` 96, `file-tree-utils.ts`'s `findFileInTree` 22; 2 embedded: `file-tree-git-status.ts` ~110, `file-tree-density.ts` ~15) | 0 | 0 |
| Workspace scoping | 261 | 261 (all 5 files: `workspace-scope.ts`, `workspace-scope-url.ts`, `placeholder.ts`, `branch-workspace.ts`, `keep-alive-policy.ts`) | 0 | 0 |
| Review threads | 306 | 216 (`branch-review-slice.ts` 156 + `review-api.ts`'s `mapThread` embedded ~60, both reached via the always-subscribed thread stream) | 90 (`use-review-annotations.tsx` embedded pure helpers, CONDITIONAL — sole importer is the CONDITIONAL `review-code-view.tsx`) | 0 |
| Theme tokens | 130 | 130 (`resolve-css-color.ts` embedded — LIVE via the always-mounted terminal's `use-terminal-theme.ts`) | 0 | 0 |
| **Total** | **~3,169** (≈3,170) | **2,546** | **585** | **≤38** |

\*File-tree model caveat, carried forward rather than smoothed over:
`file-explorer-tree-utils.ts` contributes 96 of these 718 lines and is
verdict LIVE — but only through `getExplorerTargetPath`, the one export this
document's own "genuine, portable" bullet for the file **never names**. The
four functions the bullet *does* name (`filterHiddenFiles`,
`addNewItemToTree`, `removeEditingItemsFromTree`, `getAncestorDirectoryPaths`)
are dead-on-arrival or shadowed by locally-redeclared duplicates elsewhere
(§5's Liveness table). File-level LIVE does not mean the specific content
counted toward Tier A core is the reachable part — flagged here because this
area has not been dispatched yet, so the mistake `normalize-diff.ts` already
made in git model is still avoidable here.

**LIVE + CONDITIONAL across Tier A core: 3,131 of ~3,169 (98.8%) minimum,
3,169 (100%) maximum** depending on the single unresolved git-model
ambiguity. **The ~3,170 figure was never wrong in the way a "moved by 3×"
denominator implies — it is a narrowly-scoped, almost-entirely-reachable
figure, measuring a different and much smaller thing than 9,447 does.**

### The defensible figure(s) — two scopes, not averaged

**Narrow scope — "Tier A core": `crowbar-core` domain logic only, the
original and correct meaning of "Tier A remaining" in this project's crate
boundaries (§4.2 assigns Phase 4 state to `crowbar-state`, diff-window/search
to `crowbar-diff`, tokens to `crowbar-ui`). Figure: ~3,170 lines**,
essentially unchanged and now cross-checked: ≥98.8% of it is confirmed
LIVE/CONDITIONAL, ≤1.2% is one ambiguous file already known to be dead
regardless of which bucket it's attributed to. **This is the number
`native/QUEUE.md`'s progress table should keep quoting as "N lines of a
~3,170-line target."**

**Broad scope — "everything reachable across the seven Tier-A-adjacent
feature areas' full file trees, every bucket." Figure: 9,447 lines**
(LIVE + CONDITIONAL) or 9,585 including the 138 confirmed-dead lines. This
figure is real and correctly measured (§0), but **it is not the Tier A
denominator** — quoting it as one folds ~2,760 lines of a different crate's
work (`crowbar-state`), 316 lines of another (`crowbar-diff`), ~460 lines of
non-portable presentation, and ~882 lines that will be ported to *no* crate
(D6 persistence, webview-only FOUC mechanisms this port deletes by design)
into a number that is supposed to represent one crate's remaining surface. If
this number is ever quoted going forward, it needs its own label — "total
reachable React logic across the seven surveyed areas" — never "Tier A."

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

---

## 8. Export-level liveness (P3.73) — the file-level verdict is not enough to scope a port

### Why this exists

§0 (P3.69) gave every row above a **file-level** LIVE/CONDITIONAL/DEAD verdict.
One row proved that is not sufficient to scope a port: `file-explorer/utils/
file-explorer-tree-utils.ts` (§5) is file-level **LIVE**, and **4 of its 5
exports are dead or test-only** — `filterHiddenFiles` and
`removeEditingItemsFromTree` have zero references anywhere including tests;
`addNewItemToTree` and `getAncestorDirectoryPaths` are each independently
redeclared locally in the file that needs them, so the exports are never
called; `getAncestorDirectoryPaths` even has a dedicated test exercising the
unreachable copy. Worse, this document's own "genuine, portable" prose for
that file (§5, above) describes exactly those four dead exports and never
names the one live one (`getExplorerTargetPath`). A worker scoping a port off
the file-level table alone would have ported ~4/5 dead lines. This section
re-audits **every exported value/function/type** in the three still-unported
areas named in the item brief — §2 Diff algebra, §5 File-tree model, §7
Review threads — restricted to rows already marked LIVE or CONDITIONAL in §0
(DEAD rows, e.g. `diff-search.ts`, are excluded per the brief; they are
already correctly scoped to zero).

**This section is committed incrementally, one area at a time, per this
document's own interruption protocol** (see §0's headline "committed
incrementally per the interruption protocol as each finished") — each of §2,
§5, §7 lands as its own commit below, then a final commit adds the
cross-area line-count reconciliation and scope recommendations.

### Method

**Tool: the TypeScript compiler API (`typescript@5.9.3`), not regex.** A
single `ts.Program` is built over all of `web/src`'s ~1,019 root files
(`tsconfig.json`'s own `paths`/`baseUrl` resolve the `@/` alias). For each
target file: `checker.getExportsOfModule` enumerates its real exports; for
each export, every `Identifier` node in every non-declaration source file
(including the declaring file itself) is checked with
`checker.getSymbolAtLocation`, following alias chains
(`checker.getAliasedSymbol`, which is what resolves an `export * from` shim
or a renamed `import { x as y }` back to the original declaration) — a hit
counts only if the **resolved symbol's declaration position** matches the
target's, not if the text merely matches. This is what tells a real import
apart from a same-named local redeclaration (the `addNewItemToTree` shape):
both look identical as text; only one resolves to the target symbol.

**The crux of an export-level audit, stated explicitly so the next worker
does not have to re-derive it: self-file references must be counted, not
skipped, but two different self-file shapes must be told apart.**

- **An export with zero references anywhere, including inside its own
  file, is dead.** `chord.ts`'s `MOD_ORDER` (outside this item's three
  areas, used here only as a calibration case) is the clean illustration:
  declared once, then only ever appearing again in `export { MOD_ORDER }` —
  which resolves to the same symbol as a real call site would under naive
  symbol-matching, self-file included, but is a re-export specifier, not an
  execution. Zero real uses, full stop.
- **An export consumed only by a sibling export in the same file is
  live.** `use-review-annotations.tsx`'s `useReviewAnnotations` hook calls
  its own file's `groupAnnotationsByPath`, `countThreadsByPath`,
  `threadToAnnotation`, `annotationToThread`, `toThreadSide`, and
  `isDraftThread` helpers directly, with no import involved anywhere in the
  program. **A cross-file reference search that skips the declaring file —
  the natural first instinct, since "importers are what matter" — reports
  all six as DEAD or TEST-ONLY. They are CONDITIONAL**, alive through the
  hook that calls them, exactly as alive as if some other file had imported
  them directly. Skipping self-file references was the single biggest
  source of false-DEAD verdicts in this pass; see §7 below for how many of
  `use-review-annotations.tsx`'s 11 exports that would have cost.
- **Getting these two apart requires distinguishing three things at every
  self-file occurrence, not two:** (1) declaration-site and
  import/export-specifier positions (`export function foo(){}`'s own name,
  `export { foo }`'s name) are never uses, regardless of file; (2) a use
  whose nearest enclosing named declaration is the target's **own**
  declaration is the function calling itself (recursion) — evidence of
  nothing, since nothing is shown to have invoked it the first time
  (`file-explorer-tree-utils.ts`'s `filterHiddenFiles`/`addNewItemToTree`/
  `removeEditingItemsFromTree` all show exactly this shape: a lone self-file
  hit that is pure recursion, correctly excluded, still DEAD); (3) a use
  whose enclosing declaration is a **different**, sibling export in the same
  file is real evidence of life, contingent on that sibling itself being
  reachable (`groupAnnotationsByPath` inside `useReviewAnnotations`, above).

**A fourth, narrower gap found by explicit sweep, not by the script:**
identifier-text pre-filtering (used for performance, over symbol-resolving
every identifier in every file for every export) cannot see a **renamed**
import (`import { setMergeStrategy as patchMergeStrategy }`) — the call site
reads `patchMergeStrategy(...)`, so a search seeded on the name
`setMergeStrategy` never reaches it. `grep -rnE "import\s*\{[^}]*\bas\b[^}]*\}
\s*from"` across all of non-test `web/src` found every renamed named import
in the codebase (~50 hits); manually cross-checking each against the ~120
export names audited here found exactly **one** collision (`review-api.ts`'s
`setMergeStrategy`, aliased in `merge-popover.tsx`), confirmed by reading the
call site (`await patchMergeStrategy(wsId, next)`, `merge-popover.tsx:60`) —
a real call, not a false alarm. Stated limit of the method, not hidden: this
gap was closed by an exhaustive grep sweep for the one known failure shape,
each hit verified by hand, not by trusting either tool alone.

**Failure mode of the method, stated rather than assumed away:** this is a
static import-graph + symbol-binding analysis. It cannot see
`new Worker(new URL(...))`-style dynamic loading (none of the 27 files in
this pass load that way — checked by hand) or any other
non-statically-analyzable path, and a "real use" hit proves the compiler can
reach a call/type site from a reachable declaration, not that the line
executes under some specific input at runtime. Every DEAD/TEST-ONLY/
ambiguous verdict below was additionally confirmed by reading the source
directly, not accepted on the tool's count alone.

### Controls

Four — one known-live, one known-dead (per the brief), plus the two
calibration cases for the self-file distinction above:

| control | expected | corrected method's result |
|---|---|---|
| `getExplorerTargetPath` (`file-explorer-tree-utils.ts`) — brief's own stated LIVE control | LIVE | **LIVE** — `use-file-explorer-sync.ts`: `useMemo(() => getExplorerTargetPath(activeBuffer), …)`, non-test, non-self |
| `filterHiddenFiles` (same file) — brief's own stated DEAD control | DEAD | **DEAD** — zero non-test, non-self hits; the one self-file hit (line 16) is the function calling **itself** (recursion), correctly excluded |
| `MOD_ORDER` (`chord.ts`, outside the three areas — calibration case) | DEAD | **DEAD** — zero real uses anywhere including its own file; the only self-file hit is `export { MOD_ORDER }`, a binding position, correctly excluded |
| `groupAnnotationsByPath` / `isDraftThread` (`use-review-annotations.tsx` — calibration case, the mirror of the `filterHiddenFiles` shape) | LIVE (self-file, cross-export) | **LIVE** — both called inside `useReviewAnnotations` (a *different* export in the same file); correctly counted, not excluded, since the enclosing declaration differs from the target's own |

All four pass. The last two are the load-bearing pair: get self-file
handling wrong in either direction and one of the two flips to the wrong
verdict.

### §2 Diff algebra — per-export table

§2 has no "Where it lives" table of its own (see its prose above); its
content is three rows of §1's table: `git-diff-helpers.ts`,
`lib/patch-window.ts`, and the ~368-line embedded pure region of
`components/diff/review-code-view.tsx`. `diff-search.ts` is excluded — it is
the one DEAD row in this cluster (§0/§1), out of scope per the brief.

**`features/git/utils/git-diff-helpers.ts`** (11 lines, gate: `git-diff-image.tsx`, itself only reachable inside the CONDITIONAL `review-code-view.tsx`):

| export | verdict | evidence |
|---|---|---|
| `getFileStatus` | CONDITIONAL | called in `git-diff-image.tsx` |
| `getImgSrc` | CONDITIONAL | called in `git-diff-image.tsx` (presentation — formats a `data:` URI; not logic, see Deliverable 3) |

**`features/git/lib/patch-window.ts`** (244 lines, gate: `review-code-view.tsx`'s `planWindow` call):

| export | verdict | evidence |
|---|---|---|
| `planWindow` | CONDITIONAL | called in `review-code-view.tsx`; 28-case test suite |
| `LOOKAHEAD_FILES` | CONDITIONAL | used inside `planWindow`'s own body (self-file, cross-export: a module constant consumed by the file's one real function, not the declaration site) |
| `EVICT_BEYOND_FILES` | CONDITIONAL | used inside `planWindow`'s body |
| `MAX_MATERIALIZED_FILES` | CONDITIONAL | used inside `planWindow`'s body |
| `MAX_MATERIALIZED_LINES` | CONDITIONAL | used inside `planWindow`'s body **and** directly re-exported as `review-code-view.tsx`'s `REVIEW_TOKENIZE_MAX_LENGTH` |
| `PATCH_LINE_CAP` | CONDITIONAL | used inside `planWindow`'s body **and** directly in `review-code-view.tsx` |
| `WindowInput` (type) | CONDITIONAL | `planWindow`'s parameter type |
| `WindowPlan` (type) | CONDITIONAL | `planWindow`'s return type |

Zero dead exports. Every one of the 8 exports is reachable through the same
single gate (`planWindow`'s one call site).

**`features/git/components/diff/review-code-view.tsx`** — full export list (not just the diff-algebra-relevant subset, since the file is one of "those files" per the brief), gate: dynamic `import('./diff/review-code-view')` from `review-diff-tab.tsx`, mounted only inside a commit-diff/branch-review pane:

| export | verdict | evidence |
|---|---|---|
| `partitionReviewFiles` | CONDITIONAL | called inside `ReviewCodeView`'s own render body (self-file, cross-export) |
| `buildPlaceholderFileDiff` | CONDITIONAL | called inside `ReviewCodeView`'s render body |
| `ReviewCodeView` (the component) | CONDITIONAL | imported by `review-diff-tab.tsx` |
| `REVIEW_TOKENIZE_MAX_LINE_LENGTH` | CONDITIONAL | used inside `ReviewCodeView`'s body |
| `REVIEW_TOKENIZE_MAX_LENGTH` | CONDITIONAL | used inside `ReviewCodeView`'s body |
| `ReviewFileKind` (type) | CONDITIONAL | field type inside `ReviewFileEntry`, consumed by `partitionReviewFiles` |
| `ReviewFileEntry` (type) | CONDITIONAL | `partitionReviewFiles`'s return-element type |
| `ReviewCodeViewHandle` (type) | CONDITIONAL | imperative-handle type, imported by `review-diff-tab.tsx` |
| `ReviewCodeViewProps` (type) | CONDITIONAL | the component's own prop type |

Zero dead exports here either. **But a real finding: this document's own
prose (§1) names 9 "pure functions" in this file's embedded region
(`partitionReviewFiles`, `buildPlaceholderFileDiff`, `distributeContext`,
`buildPlaceholderHunks`, `buildTailHunk`, `trimToPatchCap`, `reserveAtMost`,
`parseSingleFilePatch`, `patchCacheKey`) as if they were all equally
reachable, importable units. Only 2 of the 9 (`partitionReviewFiles`,
`buildPlaceholderFileDiff`) carry an `export` keyword — verified directly
(`grep -n "^export " review-code-view.tsx`).** The other 7 are module-private
helpers, reachable only by calling into the two exported entry points (or by
`ReviewCodeView`'s render body directly), not individually importable. They
are not dead — `buildPlaceholderFileDiff` calls `trimToPatchCap`/
`reserveAtMost`/`distributeContext`/`buildPlaceholderHunks`/`buildTailHunk`,
and `ReviewCodeView`'s render calls `parseSingleFilePatch`/`patchCacheKey`
directly — but a porter reading this document's "9 pure functions" bullet
and trying to `import buildPlaceholderHunks` the way one would
`buildPlaceholderFileDiff` would find no such export.

**§2 line-count summary:** 11 + 244 + 368 (embedded region, this document's
own estimate, §1) = **623 lines, all 623 CONDITIONAL, 0 dead, 0 test-only.**
This area passes clean at export granularity — the one real risk is not a
dead export, it is the 7-of-9 non-exported-helper naming mismatch above,
which is a *porting-unit* risk, not a *line-count* risk (all 623 lines are
still genuinely reachable and worth porting; a porter just cannot cherry-pick
the 7 private helpers by name and must treat the ~368-line embedded region as
one inseparable unit). Full line-count reconciliation across all three areas,
and scope recommendations, follow in the commits below.

### §5 File-tree model — per-export table

All 19 rows in §5's own Liveness table are LIVE or CONDITIONAL (zero DEAD
rows at file level), so every file in §5's "Where it lives" table is in
scope here. Per §5's own methodological note, the *nested* path
(`file-explorer/file-explorer/{lib,stores,utils}/*.ts`) is treated as real;
the outer `file-explorer/{lib,stores,utils}/*.ts` files are 1-line
`export * from '../file-explorer/…'` shims — confirmed again here rather
than re-trusted, and confirmed that at least two production importers
(`sidebar-carousel.tsx`, `use-workspace-effects.ts`) actually import through
the **outer shim path** for `file-explorer-tree-store.ts`'s
`useFileTreeStore` — the shim is not merely theoretical, it is the path at
least two real call sites use, and the compiler's alias-resolution (used
throughout this method) follows it transparently to the same underlying
declaration either way.

**`file-explorer/lib/visible-file-tree-rows.ts`** (238 lines):

| export | verdict | evidence |
|---|---|---|
| `buildVisibleFileTreeRows` | LIVE | `use-file-explorer-visible-rows.ts` (LIVE hook, every render) |
| `computeFileTreeSearchHits` | CONDITIONAL | called unconditionally every render inside `file-explorer-tree.tsx`'s `useMemo`, but is a no-op (`if (!q) return []`) until the tree-search box has a query — same "wiring runs always, substance is interaction-gated" shape this document already uses for `use-file-explorer-drag-drop.ts` |
| `filterFileTreeForFffHits` | CONDITIONAL | same `useMemo`, same gate |
| `getStickyAncestorRow` (singular) | **TEST-ONLY** — new finding | zero non-test callers; `file-explorer-tree.tsx` calls only the **plural** `getStickyAncestorRows` (confirmed by reading `file-explorer-tree.tsx`'s imports); the singular exists only to be tested |
| `getStickyAncestorRows` (plural) | LIVE | called directly in `file-explorer-tree.tsx`; also called by the singular's own body (irrelevant to its own verdict) |
| `getGuideAncestorRows` | LIVE | called directly in `file-explorer-tree.tsx` |
| `VisibleFileTreeRow`, `BuildVisibleFileTreeRowsOptions`, `FilterFileTreeForSearchResult`, `FileTreeSearchHit` (types) | LIVE/CONDITIONAL | signature types of the functions above, same gates respectively |

**`file-explorer/lib/file-tree-gitignore.ts`** (237 lines) — all 7 exports LIVE (`collectGitIgnoreFileReferences` called directly in `file-explorer-tree.tsx`; the rest called from `use-file-explorer-gitignore.ts`'s unconditional-every-mount `useEffect`). Zero dead exports.

**`file-explorer/lib/file-tree-git-status.ts`** (122 lines) — all 6 exports LIVE (`getFileTreeGitStatusDecoration` verified called inside `createFileTreeGitStatusLookup`'s own body, a sibling export, itself called directly in `file-explorer-tree.tsx`; the other 3 functions and 2 types all called/used directly in `file-explorer-tree.tsx`/`file-explorer-tree-item.tsx`). Zero dead exports.

**`file-explorer/lib/env-template.ts`** (90 lines, gate: right-click "create `.env` file" context-menu action):

| export | verdict | evidence |
|---|---|---|
| `isEnvFileName` | CONDITIONAL | called in `use-file-explorer-context-menu.tsx` |
| `buildEnvTemplateContent` | CONDITIONAL | called in `use-file-explorer-context-menu.tsx` |
| `normalizeEnvTargetFileName` | **TEST-ONLY** — new finding | zero non-test, zero self-file callers, verified directly by reading the file: nothing in `env-template.ts` or `use-file-explorer-context-menu.tsx` calls it. It reads like the filename-sanitizer the "create custom `.env.X`" flow *should* call on user-typed input — worth flagging to the team as a possible wiring gap, not just "don't port it" |
| `EnvTemplateTarget` (type) | CONDITIONAL | `ENV_TEMPLATE_TARGETS`'s element type |
| `ENV_TEMPLATE_TARGETS` | CONDITIONAL | used in `use-file-explorer-context-menu.tsx`'s submenu build |

**`file-explorer/lib/file-tree-density.ts`** (38 lines) — 6 exports, **5 LIVE + 1 CONDITIONAL, zero dead**: `isFileTreeDensity`/`DEFAULT_FILE_TREE_DENSITY` LIVE (verified called inside `normalizeFileTreeDensity`'s own body, itself called from `settings-normalization.ts`'s boot chain); `FileTreeDensity` (type) LIVE (used by both the always-mounted `file-explorer-tree-item.tsx` and the CONDITIONAL `file-tree-settings.tsx` — mixed, but LIVE via the stronger caller, same convention this document uses elsewhere for mixed files, e.g. `store.ts` in §4); `FILE_TREE_DENSITY_CONFIG` LIVE (called directly in three always-mounted-tree files). `FILE_TREE_DENSITY_OPTIONS` alone is CONDITIONAL (Settings → File Tree tab only).

**`file-explorer/utils/file-explorer-tree-utils.ts`** (96 lines) — the file this whole item is named after, re-verified with the corrected method:

| export | verdict | evidence |
|---|---|---|
| `filterHiddenFiles` | **DEAD** | zero non-test references; self-file hit is pure recursion (line 16, calls itself) |
| `addNewItemToTree` | **DEAD** | zero non-test references; self-file hit is pure recursion (line 38); **separately**, `use-file-explorer-inline-editing.ts` declares its own local `const addNewItemToTree = (...) => …` (verified by reading the file, line 84) that shadows the name — the exported original is never called from there either |
| `removeEditingItemsFromTree` | **DEAD** | zero non-test references; self-file hit is pure recursion (line 51) |
| `getAncestorDirectoryPaths` | **TEST-ONLY** | zero non-test, zero self-file references; `__tests__/features/file-explorer/file-explorer-tree-utils.test.ts` exercises it directly; **separately**, `file-tree-gitignore.ts` declares its own local `function getAncestorDirectoryPaths(...)` (line 206) that does the real ancestor-walk work for gitignore resolution — a second, independent implementation of the same idea, the one that's actually live |
| `getExplorerTargetPath` | LIVE | called in `use-file-explorer-sync.ts`'s every-render `useMemo` |

**One export live, four dead-or-test-only — confirms the finding this item was opened to fix, re-derived from a corrected, controls-passing method rather than re-quoted from §5.**

**`file-explorer/stores/file-explorer-tree-store.ts`** (146 lines) — sole export `useFileTreeStore`, LIVE (6 non-test importers incl. the always-mounted `sidebar-carousel.tsx`). The mutator bodies this document's prose praises as "pure `Set<string>→Set<string>` transitions" are *properties of the store's returned object*, not separate module exports — there is exactly one export-level unit here to audit, and it is live.

**`file-explorer/stores/file-explorer-clipboard-store.ts`** (110 lines) — `ClipboardEntry`/`FileClipboardState`/`PastedEntry` (types) CONDITIONAL (self-used in `useFileClipboardStore`'s own definition); `useFileClipboardStore` CONDITIONAL (read via `.getState()` inside a cut/copy/paste action handler, not subscribed for every-render display — §5's own established gate). Zero dead exports.

**Hooks** (`use-file-explorer-drag-drop.ts` 315, `use-file-explorer-inline-editing.ts` 231, `use-file-explorer-gitignore.ts` 79, `use-file-explorer-sync.ts` 50, `use-file-explorer-visible-rows.ts` 87, `use-file-explorer-context-menu.tsx` 612) — each exports exactly one hook; all confirmed called directly in `file-explorer-tree.tsx`. Verdicts match §5's file-level table exactly (gitignore/sync/visible-rows LIVE; drag-drop/inline-editing/context-menu CONDITIONAL). Zero dead exports — these are Phase-4 glue regardless of liveness (see Deliverable 3), but none of them are *dead* glue.

**`features/files/lib/file-tree-api.ts`** (141 lines) — 11 exports, all LIVE or CONDITIONAL: `toAppFile` LIVE (called inside `fetchFileTree`'s own body, line 36, a sibling export); `fetchFileTree`/`filesWsEndpoint`/`createFileNode`/`deleteFileNode`/`findNode`/`mergeChildren`/`renameFileNode`/`copyFileNode`/`FileNodeDTO` all LIVE via `use-workspace-effects.ts` (always-mounted); `writeFileContent` CONDITIONAL-only (its sole caller is `file-upload.ts`'s CONDITIONAL `pickAndUploadFiles`). 10 LIVE + 1 CONDITIONAL, zero dead exports.

**`features/files/lib/file-upload.ts`** (60 lines) — both exports CONDITIONAL: `pickAndUploadFiles` (right-click "Upload Files"; its call site is actually in `sidebar-carousel.tsx`'s `handleUploadFile` callback, wired down as a prop through `FileExplorerTree` to `useFileExplorerContextMenu`'s `onUploadFile` — a **correction** to this document's own §5 prose, which says the sole importer is the context-menu hook; the hook only *invokes* the callback, the import and top-level call live in `sidebar-carousel.tsx`) and `readFileAsBase64` (called inside `pickAndUploadFiles`'s own body, a sibling export). Zero dead exports.

**`features/file-system/controllers/file-tree-utils.ts`** (22 lines) — sole export `findFileInTree`. **Verdict correction: §5's file-level table calls this LIVE ("called directly in `file-explorer-tree.tsx:614`"); at call-site granularity it is CONDITIONAL.** Both of its two real call sites are gated: `use-file-explorer-inline-editing.ts` (already CONDITIONAL) and, in `file-explorer-tree.tsx`, inside `collectLoadedFilesInDirectory` — which is itself only called from `handleOpenAllFilesInDirectory`, the handler for the **"Open All Files in Directory"** context-menu action (verified by reading `file-explorer-tree.tsx:603-729` directly). §5's own file-level LIVE verdict took the existence of a direct call in the tree component at face value without checking that call's enclosing context was itself gated — precisely the class of error §0's own "Sidebar panels are an off-viewport carousel" finding already warns about, now caught at a deeper call depth.

**`features/file-system/controllers/file-utils.ts`** (5 lines) — `getFileName` and `getFilenameFromPath` are the same function under two names (`export const getFilenameFromPath = getFileName`, a literal alias). Both LIVE (`getFilenameFromPath` called unconditionally in `editor-status-actions.tsx`'s render). Zero dead exports.

**`features/file-system/types/app.ts`** (38 lines) — 3 exports, **2 LIVE + 1 CONDITIONAL, zero dead**: `AppFile` and `FileEntry` LIVE (each flows into at least one already-LIVE consumer above); `ContextMenuState` CONDITIONAL (its sole use is `use-file-explorer-context-menu.tsx`'s own CONDITIONAL hook, above — not LIVE, despite sitting in the same file as two LIVE types).

**§5 line-count summary (full reconciliation in the final commit below):** DEAD/TEST-ONLY lines total **95** — 71 in `file-explorer-tree-utils.ts` (`filterHiddenFiles` 18 + `addNewItemToTree` 21 + `removeEditingItemsFromTree` 11 + the private `getParentPath` helper 7, itself only reachable from the now-TEST-ONLY `getAncestorDirectoryPaths` + `getAncestorDirectoryPaths` 14), 7 in `visible-file-tree-rows.ts` (`getStickyAncestorRow`), 17 in `env-template.ts` (`normalizeEnvTargetFileName`). Every other file in §5 is 100% LIVE/CONDITIONAL at export level.

### §7 Review threads — per-export table

All rows in §7's Liveness table are LIVE or CONDITIONAL (zero DEAD).
`review-thread-item.tsx` is a component (§7's own convention: "not counted"
toward the area's line total) — audited for completeness, excluded from the
line-count denominator, consistent with §7's own convention.

**`features/workspace/stores/slices/branch-review-slice.ts`** (156 lines) — 8 exports, **all LIVE**, re-verified by reading the file directly: `MergeStrategy`/`ReviewMessage`/`ReviewThread`/`ReviewConversation` are all used as field/parameter types inside `BranchReviewState`/`BranchReviewSlice` (lines 32-68), both of which are the shape of `createBranchReviewSlice`'s return value; `createBranchReviewSlice` itself is called directly in `workspace-store.ts` (every workspace). Note: the 12 "pure `ReviewThread[]→ReviewThread[]` transition" mutators this document's prose praises (`addReviewThread`, `upsertReviewThread`, etc., §7 above) are **properties of `createBranchReviewSlice`'s returned object literal**, not separate module exports — same shape as `file-explorer-tree-store.ts` above (§5). `resolveReviewThread` is marked `@deprecated` in its own doc-comment ("Kept for backward compat... Use setReviewThreadResolved instead") — still part of the live interface, but a porter should confirm nothing still calls it before carrying it forward (a store-method-level question this export-level method cannot answer, since it is not itself an ES-module export).

**`features/git/api/review-api.ts`** (288 lines) — 18 exports, **all LIVE or CONDITIONAL after one correction**:

| export | verdict | evidence |
|---|---|---|
| `mapThread` | LIVE | called in `use-workspace-threads-stream.ts` (always-mounted) |
| `listThreads` | LIVE | called in `use-workspace-threads-stream.ts` |
| `ThreadDTO`, `ThreadReplyDTO` | LIVE | `ThreadDTO` used directly in `use-workspace-threads-stream.ts`; `ThreadReplyDTO` is `ThreadDTO`'s nested reply-element type |
| `getReview` | CONDITIONAL | `branch-review-pane.tsx` |
| `getReviewFiles` | CONDITIONAL | `use-review-files-summary.ts` → `review-diff-tab.tsx` |
| `mergeIntoParent` | CONDITIONAL | `merge-popover.tsx` |
| `setMergeStrategy` | CONDITIONAL — **corrected from an initial false TEST-ONLY reading** | `merge-popover.tsx:60`, imported renamed (`setMergeStrategy as patchMergeStrategy`) — see the renamed-import method gap above; a plain identifier-text search misses this entirely |
| `openThread`, `replyToThread`, `setThreadResolved`, `deleteThread`, `deleteMessage`, `editMessage` | CONDITIONAL | all called in `use-review-annotations.tsx` |
| `ReviewState` (type) | CONDITIONAL | `getReview`'s return type |
| `ReviewFileSummary` (type) | CONDITIONAL | used in `review-file-summary-to-git-diff.ts` (§1: CONDITIONAL) |
| `OpenThreadInput`, `ReplyToThreadInput` (types) | CONDITIONAL | `openThread`/`replyToThread`'s parameter types |

Zero dead exports (after the rename correction). **Same non-exported-helper
shape as `review-code-view.tsx` (§2), found independently here**: the doc's
prose above (§7) says "`review-api.ts`'s `mapThread`/`mapReply`/
`mapConversation`" as if all three were exported. Only `mapThread` carries
`export`; `mapReply` (line 52, called inside `mapThread`) and
`mapConversation` (line 87, called inside `getReview`) are module-private.
Not dead — both are genuinely reachable through their respective exported
callers — but, again, not individually importable the way the prose implies.

**`features/git/components/diff/use-review-annotations.tsx`** (395 lines) — 11 exports, **all CONDITIONAL, zero dead** — this is the file the self-file method correction above exists for. `isDraftThread`, `toThreadSide`, `threadToAnnotation`, `annotationToThread`, `groupAnnotationsByPath`, `countThreadsByPath` are each called inside `useReviewAnnotations`'s own body (self-file, cross-export); `toAnnotationSide` additionally has a direct external caller (`review-code-view.tsx`); `useReviewAnnotations` itself, `ReviewAnnotation` (type), `DRAFT_THREAD_ID`, `ReviewAnnotationLayer` (type) are all likewise reachable. **Without the self-file correction, a naive tool would have reported 6 of these 11 exports as DEAD or TEST-ONLY** — the mirror-image failure to `file-explorer-tree-utils.ts`'s real dead exports (§5): false negatives (wrongly-DEAD) are exactly as costly to a scoping decision as false positives (wrongly-LIVE, the `addNewItemToTree` shape), because both cause a porter to skip code that should ship or ship code that shouldn't.

**`features/workspace/stores/hooks/use-workspace-threads-stream.ts`** (61 lines) — sole export `useWorkspaceThreadsStream`, LIVE (called in `use-workspace-effects.ts`, always-mounted).

**`features/git/components/review-thread-item.tsx`** (424 lines, component, not counted toward the line total per §7's own convention) — both exports CONDITIONAL (`ReviewThreadItem` called in `use-review-annotations.tsx`; `ReviewThreadItemProps` its own prop type). Zero dead.

**§7 line-count summary:** 156 + 288 + 395 + 61 = **900 lines** (matches §7's
own stated total exactly), **all 900 LIVE or CONDITIONAL, zero dead, zero
test-only.** This is the one area of the three where the export-level pass
found nothing to prune — the two risks found here are not line-count risks:
the non-exported-helper naming mismatch (`mapReply`/`mapConversation`) and
the one renamed-import blind spot (`setMergeStrategy`), both already folded
into the table above. Full cross-area line-count reconciliation and scope
recommendations follow below.

### Deliverable 2 — portable LIVE+CONDITIONAL lines vs. total lines, per area

"Portable" here means "attributable to an export that is LIVE or
CONDITIONAL" (i.e. reachable) as opposed to DEAD or TEST-ONLY — this is the
axis this item exists to check (reachability), not a re-run of the original
doc's separate logic-vs-presentation/Phase-4 classification (already done
per-file above and in §1/§5/§7's prose). The gap below is exactly what a
file-level scoping (stop at §0's verdicts) would have over-ported.

| area | total lines (files in scope) | DEAD/TEST-ONLY lines | LIVE+CONDITIONAL lines | over-port risk |
|---|---|---|---|---|
| §2 Diff algebra | 623 (3 files: 11 + 244 + 368-embedded) | 0 | 623 | **0%** |
| §5 File-tree model | 2,717 (19 files, exact `wc -l`†) | 95 | 2,622 | **3.5%** |
| §7 Review threads | 900 (4 non-component files, matches §7's own total) | 0 | 900 | **0%** |
| **All three areas** | **4,240** | **95** | **4,145** | **2.2%** |

†**§5's total-lines reconciliation.** §5's own prose states "**2,002** lines"
as its area total — that figure sums only the 15 of 19 rows that carry a
line count in §5's own "Where it lives" table (`238+237+122+90+38+96+146+
110+315+231+79+50+87+141+22 = 2,002`, verified by direct re-addition). Four
rows were left uncounted in that table: `use-file-explorer-context-menu.tsx`
(612 lines, marked "not counted, component-adjacent"), `file-upload.ts` (60),
`file-utils.ts` (5), and `types/app.ts` (38) — `612+60+5+38 = 715`, and
`2,002 + 715 = 2,717`, exactly this section's recount. Not an arithmetic
error (unlike the §6 146-line miscount §0 already found) — a deliberate
exclusion of four small/component-adjacent files from the original headline
sum. Since the brief asks for **every** row marked LIVE/CONDITIONAL, all 19
are counted here; `95/2,717 = 3.5%` is the honest area-level figure. Using
§5's own narrower 2,002 instead, the same 95 dead/test-only lines (none of
which sit in the 4 excluded files) would read `95/2,002 = 4.7%` — the
direction of the correction is the same either way, only the percentage
moves.

**The risk is concentrated, not diffuse — reported per file because an
area-level percentage alone hides where the risk actually is:**

| file | total | dead/test-only | waste % |
|---|---|---|---|
| `file-explorer-tree-utils.ts` | 96 | 71 | **74%** |
| `env-template.ts` | 90 | 17 | **19%** |
| `visible-file-tree-rows.ts` | 238 | 7 | **3%** |
| every other file in all 3 areas (23 of 26 files) | 3,816 | 0 | **0%** |

(26 = 3 files in §2 + 19 in §5 + 4 non-component files in §7; 23 = 26 minus
the 3 named above.)

A file-level scoping of §5 would have looked clean by area (96.5%
LIVE+CONDITIONAL) while still shipping 71 fully-dead lines from one
96-line file — the same near-miss the original `tier-a-denominator.md`
already made once, at file granularity, for `normalize-diff.ts` and
`diff-buffer-path.ts` (§0/§1).

### Deliverable 3 — scope recommendation per area

**§2 Diff algebra — port:**
- `getFileStatus` (`git-diff-helpers.ts`) — genuine classification logic.
- `planWindow` + `LOOKAHEAD_FILES`/`EVICT_BEYOND_FILES`/`MAX_MATERIALIZED_FILES`/`MAX_MATERIALIZED_LINES`/`PATCH_LINE_CAP`/`WindowInput`/`WindowPlan` (`patch-window.ts`, all 8 exports, all CONDITIONAL) — to `crowbar-diff`, not `crowbar-core`, per this document's own §1/§2 crate-boundary finding (unchanged by this pass — liveness confirms it is worth porting *somewhere*, not which crate).
- The full ~368-line embedded region of `review-code-view.tsx` **as one unit** — `partitionReviewFiles` and `buildPlaceholderFileDiff` plus their 7 non-exported private helpers (`trimToPatchCap`, `reserveAtMost`, `distributeContext`, `buildPlaceholderHunks`, `buildTailHunk`, `patchCacheKey`, `parseSingleFilePatch`) cannot be split into "port these 2, skip those 7" — the 7 are unreachable except through the 2. Re-type against `crowbar-proto`'s `Hunk` instead of `@pierre/diffs`'s (already flagged, §1) — a required dependency substitution, not optional cleanup.

**§2 — skip:**
- `getImgSrc` (`git-diff-helpers.ts`) — CONDITIONAL-live, but presentation (a `data:` URI formatter), not logic.
- `diff-search.ts` — DEAD, excluded from this pass per the brief, unchanged.
- `ReviewCodeView`/`ReviewCodeViewHandle`/`ReviewCodeViewProps` — the component itself and its React-facing types; GPUI does not port a React component 1:1.

**External dependency:** none new. `@pierre/diffs`'s `Hunk`/`FileDiffMetadata` type entanglement, already named in §1, is the only one in this area.

---

**§5 File-tree model — port:**
- `buildVisibleFileTreeRows`, `getStickyAncestorRows` (**plural only** — the singular `getStickyAncestorRow` is test-only, do not port it as a separate function; if a single-ancestor accessor is wanted, call the plural and take the last element, exactly as the dead singular already does), `getGuideAncestorRows`, and — modelling "tree search is active" as an explicit state, not an always-on default — `computeFileTreeSearchHits`/`filterFileTreeForFffHits`, plus all 4 supporting types, from `visible-file-tree-rows.ts`.
- All of `file-tree-gitignore.ts` (7 exports, 237 lines, all LIVE) — see the `ignore`-crate decision below.
- All of `file-tree-git-status.ts` (6 exports, 122 lines, all LIVE) — but split `getFileTreeGitStatusDecoration`'s `colorClassName` (a hardcoded Tailwind string) from its `statusLetter`/`label` classification at the type level; the former becomes a `crowbar-ui::Color` seal, not a string, per this document's own bucket-4 finding (§5 above).
- `normalizeFileTreeDensity`, `isFileTreeDensity`, `DEFAULT_FILE_TREE_DENSITY`, `FileTreeDensity` (type), and `FILE_TREE_DENSITY_CONFIG`'s `rowHeight` field only (its `rowClassName` field is presentation, skip) from `file-tree-density.ts`. `FILE_TREE_DENSITY_OPTIONS` is Settings-tab-only presentation copy — skip.
- **`getExplorerTargetPath` only** from `file-explorer-tree-utils.ts` — the file's one live export. Do not port `filterHiddenFiles`, `addNewItemToTree`, or `removeEditingItemsFromTree` (dead). Do not port the exported `getAncestorDirectoryPaths` either — it is test-only; if this logic is needed, port `file-tree-gitignore.ts`'s own local `getAncestorDirectoryPaths` (line 206), the copy that is actually live, and drop the redundant dead twin rather than carrying two implementations of the same idea into the port.
- `findFileInTree` from `file-system/controllers/file-tree-utils.ts` — port, but model it as CONDITIONAL (behind "Open All Files in Directory" and inline-rename/create), not as always-reachable core logic, correcting §5's file-level LIVE verdict.
- `getFileName`/`getFilenameFromPath` (`file-utils.ts`) — port once, under one name; they are the same function.
- `AppFile`, `FileEntry`, `ContextMenuState` (`types/app.ts`) — the shared record types every function above needs.
- `file-tree-api.ts`'s transport functions and `FileNodeDTO` (`fetchFileTree`, `filesWsEndpoint`, `createFileNode`, `renameFileNode`, `deleteFileNode`, `copyFileNode`, `findNode`, `mergeChildren`, `toAppFile`, `FileNodeDTO` — 10 of the file's 11 exports) — to `crowbar-client`, not `crowbar-core`, per this document's own existing classification; liveness confirms all 10 are worth porting somewhere. `writeFileContent` (the 11th) is CONDITIONAL-only (upload path) but equally a transport function — same crate, same recommendation.

**§5 — skip:**
- All 7 hook/store files (`file-explorer-tree-store.ts`, `file-explorer-clipboard-store.ts`, and the 5 `use-file-explorer-*` hooks, 1,478 lines combined) — Phase-4/glue by this document's own bucket rule, regardless of the fact that none of their exports are individually dead. GPUI replaces this hook-wiring shape wholesale (per this document's established D2 pattern); the *mutator bodies* inside `file-explorer-tree-store.ts` are worth extracting as plain functions (already flagged, §5 above), but that is a refactor of Phase-4 code, not a Tier A port item.
- `env-template.ts` in full — small (90 lines), genuinely CONDITIONAL logic, but niche (one context-menu action) and one of its 5 exports (`normalizeEnvTargetFileName`) looks like a wiring bug rather than dead-by-design (flag to the team; do not silently drop the finding along with the line).
- `file-upload.ts` — `readFileAsBase64`/`pickAndUploadFiles` are `FileReader`/`<input type=file>`-bound; native file-picker is `crowbar-platform` territory (same convention as `diagnostics-export.ts`, §4), not `crowbar-core`.

**External dependency decision — `file-tree-gitignore.ts`'s `ignore@5.3.2`:**
this file uses the npm package for two things: (a) `.gitignore`-syntax
pattern parsing — glob with `**`, leading-`/` anchoring, trailing-`/`
directory-only matches, `#` comments, `\`-escapes — and negation (`!pattern`)
via a `matcher.test(path)` call returning `{ignored, unignored}`; and (b) one
matcher **per directory** that has a `.gitignore` (`createFileTreeGitIgnoreRules`
builds a `ruleSets` array, one `ignore()` instance per file). The Rust
`ignore` crate (docs.rs/ignore, ripgrep's own crate) implements the identical
`.gitignore` semantics through `ignore::gitignore::GitignoreBuilder`/
`Gitignore`, whose `matched()` returns `Match::{Ignore, Whitelist, None}` — a
direct structural match for `{ignored, unignored}`. **Recommendation: use the
Rust `ignore` crate's `Gitignore` type, one instance per directory, mirroring
this file's own `ruleSets` shape — do not hand-roll a matcher.** What the
crate does **not** provide, and what this file's own original contribution
is, is the **cascade**: `isPathGitIgnoredByFileTreeRules` walks every
ancestor directory (`getAncestorDirectoryPaths`, the live local copy, not the
dead exported one) and tests each ancestor's own rules *before* testing the
target path itself, because a directory ignored by a parent rule ignores
everything under it regardless of its own `.gitignore` content. That
ancestor-first cascade algorithm has no crate equivalent and must be
reimplemented in Rust exactly as it exists here, driving one `Gitignore`
matcher per directory rather than one matcher for the whole tree.

---

**§7 Review threads — port, no skips found:**
this is the one area where the export-level pass found nothing to prune —
all 900 lines across all 4 files are LIVE or CONDITIONAL at export
granularity, confirming §7's own "genuine, portable" prose was correct in
substance (though not in exact function-naming — see the `mapReply`/
`mapConversation` finding above).
- `ReviewMessage`/`ReviewThread`/`ReviewConversation` (types) + `createBranchReviewSlice` as one unit (`branch-review-slice.ts`) — the 12 individual "pure mutator" functions this document's prose names are properties of that one factory's return value today, not separately exported; extracting them into standalone `ReviewThread[] → ReviewThread[]` functions during the port (as the doc already recommends) is a refactor choice a porter is free to make, not something this audit can verify export-by-export since they are not distinct compiler-visible symbols yet. Confirm before porting that `resolveReviewThread` (marked `@deprecated` in its own source) still needs to ship, or whether it can be dropped in favor of `setReviewThreadResolved`.
- `mapThread` + its private `mapReply` helper, and `getReview` + its private `mapConversation` helper (`review-api.ts`) — port each pair as one unit, same reasoning as `review-code-view.tsx` above. All of `review-api.ts`'s other 16 exports — `listThreads`, the 9 CRUD/mutation transport functions (`getReviewFiles`, `mergeIntoParent`, `setMergeStrategy`, `openThread`, `replyToThread`, `setThreadResolved`, `deleteThread`, `deleteMessage`, `editMessage`), and its 6 supporting types (`ThreadDTO`, `ThreadReplyDTO`, `ReviewState`, `ReviewFileSummary`, `OpenThreadInput`, `ReplyToThreadInput`) — go to `crowbar-client`, not `crowbar-core`, per this document's existing convention (transport, not model) — liveness confirms every one of the file's 18 exports is worth porting somewhere, `setMergeStrategy` included despite its near-miss TEST-ONLY misread.
- 10 of `use-review-annotations.tsx`'s 11 exports — its pure-helper half: 7 functions (`isDraftThread`, `toAnnotationSide`, `toThreadSide`, `threadToAnnotation`, `annotationToThread`, `groupAnnotationsByPath`, `countThreadsByPath`) plus their 3 supporting types (`ReviewAnnotation`, `DRAFT_THREAD_ID`, `ReviewAnnotationLayer`) — genuinely reachable, genuinely gpui-free (modulo the already-flagged `@pierre/diffs` `DiffLineAnnotation<T>` type entanglement, §7 above, which needs the same `crowbar-diff`-native retyping as §2's placeholder algebra). The 11th export, `useReviewAnnotations` itself (the hook), is Phase-4/presentation — not ported as such.

**External dependency:** none new beyond the already-flagged `@pierre/diffs` annotation-type entanglement.

### Verdict tally, this pass

Counted by re-tallying every per-export table above directly (not carried
over from the export-liveness script's raw category labels, which predate
the LIVE-vs-CONDITIONAL gate-tracing done by hand per file) — an earlier
draft of this table underused that re-tally and undercounted every area's
export total; the figures below are the corrected count, and the process
that caught the mismatch is worth keeping: sum the per-file export counts
named in each area's own tables above and check they equal the total. `§7`'s
`review-thread-item.tsx` (component, 2 exports) is excluded here, matching
the same "not counted" convention §7 already applies to its line total.

| area | exports audited | LIVE | CONDITIONAL | DEAD | TEST-ONLY |
|---|---|---|---|---|---|
| §2 Diff algebra (3 files) | 19 | 0 | 19 | 0 | 0 |
| §5 File-tree model (19 files) | 69 | 42 | 21 | 3 | 3 |
| §7 Review threads (4 non-component files) | 38 | 13 | 25 | 0 | 0 |
| **Total** | **126** | **55** | **65** | **3** | **3** |

Zero UNCERTAIN verdicts — every export resolved to a definite answer with a
concrete caller and, for every DEAD/TEST-ONLY verdict, a direct read of the
source confirming it (not just the tool's count taken on faith, per the
`setMergeStrategy` near-miss above). The 6 DEAD+TEST-ONLY exports are
exactly the 3 files named in Deliverable 2's per-file waste table
(`file-explorer-tree-utils.ts`'s 3 DEAD + 1 TEST-ONLY, `visible-file-tree-
rows.ts`'s 1 TEST-ONLY, `env-template.ts`'s 1 TEST-ONLY) — the export-count
and line-count views of this pass agree on where the risk sits.
