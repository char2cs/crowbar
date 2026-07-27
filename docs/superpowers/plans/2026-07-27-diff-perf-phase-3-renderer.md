# Diff Perf Phase 3 — Windowed Renderer — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Point the Branch Review pane at the windowed API and `@pierre/diffs`, so a child-workspace review of a 1M-line branch holds constant memory instead of 1,162 MB.

**Architecture:** The pane stops loading the 158 MB `/review` composite. It builds one `CodeView` item per file from `/review/files` + `/review/outline` (geometry only, no content), and fetches `/review/patch?path=` for files near the viewport, evicting distant ones under an LRU. Threads become `lineAnnotations` rendering the existing `ReviewThreadItem`. Find-in-diff calls `/review/search`.

**Tech Stack:** React 19, `@pierre/diffs` 1.2.12 (installed, spiked), Vitest.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-07-27-diff-subsystem-at-scale-design.md`. Measurements: `perf-baselines.md`.
- **The bar to beat, measured on `review-demo` (child of protected `main`, 406 files / 1,005,251 insertions):** `/review` payload 158 MB, 1,441,452 line objects, webview peak **1,162 MB**, **544 MB retained after closing the tab**, ~10s to render.
- **Primary use case is a CHILD workspace reviewed against its PARENT branch** — the GitHub-like step. A whole branch, not one commit and not uncommitted work. `resolveBase` also supports no-parent → repo default branch; both must keep working.
- **Subagents must NOT run git write commands** and must NOT drive the Tauri app. The coordinating session commits and does all live verification.
- No timing-based synchronization in tests. No sleeps, no polling, no `Eventually`.
- Web: `~/.bun/bin/bun` (NOT on PATH). Never `bunx tsc`; use `~/.bun/bin/bun tsc --noEmit`.
- Tests mirror `web/src/` under `web/src/__tests__/`, `@/` imports, kebab-case files.
- Narrow store selectors only; `getState()` only in handlers/effects.
- jsdom lacks constructable stylesheets — already shimmed globally in `src/__tests__/setup.ts`. jsdom also has no layout, so virtualised components need the `getBoundingClientRect` stub pattern used in `changed-files-tree.scale.test.tsx`.

---

### Task A: Windowed data layer

**Files:**
- Create: `web/src/features/git/api/review-window-api.ts`
- Create: `web/src/features/git/lib/patch-window.ts`
- Create: `web/src/__tests__/features/git/lib/patch-window.test.ts`
- Create: `web/src/__tests__/features/git/api/review-window-api.test.ts`

**Interfaces produced** (Tasks B–D consume these exactly):

```ts
// review-window-api.ts
export interface HunkShape { oldStart: number; oldLines: number; newStart: number; newLines: number }
export interface FileOutline { path: string; oldPath?: string; hunks: HunkShape[]; isPartial: boolean; isBinary: boolean }
export async function getReviewOutline(wsId: string): Promise<FileOutline[]>
export async function getReviewPatch(wsId: string, path: string, maxLines?: number): Promise<{ patch: string; truncated: boolean }>
export interface SearchHit { path: string; side: 'old' | 'new'; lineNumber: number; preview: string }
export async function searchReviewDiff(wsId: string, q: string, opts?: { regex?: boolean; caseSensitive?: boolean; limit?: number }): Promise<{ hits: SearchHit[]; truncated: boolean }>

// patch-window.ts — PURE, no React, no fetch
export interface WindowInput { visible: { first: number; last: number }; total: number; materialized: readonly string[]; paths: readonly string[]; lineCounts: Readonly<Record<string, number>> }
export interface WindowPlan { fetch: string[]; evict: string[] }
export function planWindow(input: WindowInput): WindowPlan
export const LOOKAHEAD_FILES = 6
export const EVICT_BEYOND_FILES = 20
export const MAX_MATERIALIZED_FILES = 40
export const MAX_MATERIALIZED_LINES = 60_000
```

**Why a pure planner:** the materialise/evict decision is where this phase is either correct or thrashing, and it is the one part that can be tested exhaustively without a DOM, a network, or a virtualiser.

- [ ] **Step 1: Write failing tests for `planWindow`**

Cover: nothing materialised → fetches the visible band plus lookahead; already-materialised files are not refetched; files beyond `EVICT_BEYOND_FILES` are evicted; the eviction band is strictly wider than the lookahead band so a file oscillating at the boundary is not refetched every frame (assert this explicitly with two adjacent scroll positions); `MAX_MATERIALIZED_FILES` is respected, evicting furthest-from-viewport first; `MAX_MATERIALIZED_LINES` is respected even when the file count is under its cap (one 50k-line file plus a 20k-line file must evict); a visible file is NEVER evicted even if it alone exceeds the line budget (correctness beats the budget — the user is looking at it); empty input is a no-op.

- [ ] **Step 2: Run and confirm red.** `cd web && ~/.bun/bin/bun run vitest run src/__tests__/features/git/lib/patch-window.test.ts`

- [ ] **Step 3: Implement `planWindow`,** then the API client. The client mirrors `review-api.ts` conventions (`apiFetch`, `workspaceBase`). `getReviewPatch` must read `text/plain`, not JSON, and read truncation from the `X-Crowbar-Diff-Truncated` header — a readable leading header, deliberately not a trailer, because `fetch()` cannot read trailers.

- [ ] **Step 4: Verify.** Both test files green; `~/.bun/bin/bun tsc --noEmit` clean.

---

### Task B: The CodeView review surface

**Files:**
- Create: `web/src/features/git/components/diff/review-code-view.tsx`
- Create: `web/src/__tests__/features/git/components/diff/review-code-view.test.tsx`

**Consumes:** everything from Task A. **Produces:** `<ReviewCodeView wsId files outline isActivePane />` where `files: GitDiff[]` (the summary list) and `outline: FileOutline[]`.

Requirements:

- One `CodeViewDiffItem` per file, built from the outline's hunk geometry so each file reserves correct scroll space **before** its patch is fetched. `computeEstimatedDiffHeights` needs only hunk line counts, never content.
- **A `isPartial` file's geometry is a LOWER bound** (the outline caps at 1000 hunks per file). Top the estimate up from the summary's ±counts, or that file under-reserves space.
- Materialise via `CodeViewHandle.updateItem` as files enter the band; revert to placeholder on eviction. Drive the band from `onScroll`.
- Wrap in `WorkerPoolContextProvider` so Shiki highlighting runs off the main thread.
- Set `tokenizeMaxLineLength` / `tokenizeMaxLength` so a minified 657k-character line cannot hang tokenisation.
- A truncated patch renders what arrived plus a "show all" affordance that refetches uncapped. **The fixture's monster file is a SINGLE 420k-line hunk, so under the default cap NOTHING fits and the response is a 142-byte header-only patch** — that must read as "show all", never as an empty file.
- Binary files: render the existing `ImageDiffViewer` for images, and a plain "binary file" row otherwise. Never feed a binary patch to the diff renderer.

- [ ] **Steps 1-4:** failing tests → red → implement → green + tsc. Test with the `getBoundingClientRect` stub. Assert: item count equals file count; only banded files carry content; scrolling changes which are materialised; a partial file's reserved height exceeds its outline-derived height.

---

### Task C: Threads as annotations

**Files:**
- Create: `web/src/features/git/components/diff/use-review-annotations.tsx`
- Create: `web/src/__tests__/features/git/components/diff/use-review-annotations.test.tsx`
- Modify: `web/src/features/git/components/diff/review-code-view.tsx`

**Consumes:** Task B. Threads come from `branchReview.threads` (store slice, kept live by `useWorkspaceThreadsStream`) — **independent of the diff payload**, so nothing about thread data changes.

- Map `ReviewThread → DiffLineAnnotation<ReviewThread>`: `side: t.side === 'old' ? 'deletions' : 'additions'`, `lineNumber: t.lineNumber`, `metadata: t`.
- `renderAnnotation` renders the **existing** `ReviewThreadItem` unchanged. It is portalled into light DOM, so Tailwind and CSS-var tokens apply — verified by the spike.
- Creation: `enableLineSelection` + `onSelectedLinesChange` → `{start, end, side}` → existing `openThread({filePath, line, startLine, endLine, side})`. Hover "+" via `renderGutterUtility`.
- Reply/resolve/edit/delete keep using the existing `review-api.ts` calls untouched.
- **A thread on a file that is not materialised has nowhere to render.** Show a per-file thread count on the file header from the threads store, and make navigating to a thread force materialisation of its file before `scrollTo({type:'line'})`.
- Round-trip test: thread → annotation → back, both directions, including `old`/`new` side mapping.

---

### Task D: Server-backed find-in-diff

**Files:**
- Create: `web/src/features/git/components/diff/review-search-bar.tsx`
- Create: `web/src/__tests__/features/git/components/diff/review-search-bar.test.tsx`

- Debounce 200ms into `searchReviewDiff`. Render a hit list (path, line, preview). Selecting a hit materialises the file and scrolls to the line. Next/prev traverse hits.
- Show truncation honestly when the server caps at `limit` — "first 200 matches", not a silent cut.
- Invalid regex surfaces the 400 as an inline message, never a crash.

---

### Task E: Flip and delete (coordinating session only)

- `review-diff-tab.tsx` renders `ReviewCodeView`, fed by `/review/files` + `/review/outline`. It stops reading `branchReview.diffCache`.
- `branch-review-pane.tsx` stops calling `getReview` for the diff; it keeps description/mergeStrategy/conversations if those still come from `/review`.
- Delete: `git-diff-editor-stack.tsx`, `git-diff-editor-surface.tsx`, `git-diff-viewer.tsx`, `use-review-comment-layer.tsx`, `use-hunk-staging-zones.tsx`, `diff-search-bar.tsx`, `use-diff-search.ts`, `diff-search-context.ts`, `utils/diff-search.ts`, `diff-editor-content.ts`, `git-diff-cache.ts`, `diff-viewer-scale.ts`, `working-tree-multi-diff.ts`, `use-diff-editor-buffer.ts`, and their tests.
- Remove `setBranchReviewDiff` / `diffCache` from the workspace store if nothing else reads them — **grep first**; the sidebar's `use-sidebar-changed-files.ts` reads `diffCache` today and must move to the summary.
- Backend: drop the per-line `Lines` from the `/review` composite only once nothing reads it.
- **Live verification (mandatory, MCP):** on `review-demo`, open the pane and confirm it renders; scroll deep; open a thread; create one on a line range; resolve it; search; toggle split/unified; expand a truncated monster file. Measure webview RSS at peak and after closing the tab — the numbers to beat are 1,162 MB and 544 MB.

---

## Definition of done

- [ ] `cd api && go test -tags noEmbed -race ./...` green; `cd web && ~/.bun/bin/bun run vitest run` green; `bun tsc --noEmit` clean; lint no new findings.
- [ ] Branch Review renders `review-demo` (406 files / 1M lines) without loading `/review`'s 158 MB.
- [ ] Webview RSS **bounded and roughly flat** across scrolling the whole diff, and it does not retain the diff after the tab closes.
- [ ] Threads: render, create on a range, reply, resolve — verified live.
- [ ] Search returns hits and navigates to them — verified live.
- [ ] Old stack deleted; no dead references.
- [ ] Existing `perf-baselines.md` M-series re-checked on a small repo: no regression.
