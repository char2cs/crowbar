# Git Review Redesign — GitHub-PR-style branch review

**Date:** 2026-06-20
**Branch:** `refactor/entity-scoped-api-ws`
**Status:** Design approved (visual), ready for implementation plan

---

## 1. Problem

Crowbar currently has **two parallel, half-built git diff systems** and a flat,
unloved git sidebar:

- **System A — "Branch Review" pane** (`branchReview` buffer): About/Git/Diff
  sub-tabs, a merge-strategy *selector* that executes nothing, a flat thread
  panel where you anchor a comment by *typing* a file path + line number, and a
  virtualized multi-file diff viewer marked "unwired".
- **System B — "Uncommitted Changes" diff buffer** (`diff` buffer): opened from
  a flat changed-files list, rendered through a Monaco per-file editor stack,
  with its own throwaway Alt-click comment prototype (local state, pixel-estimated
  line, never persisted).

Two renderers, two comment systems, two data paths, and a flat file list. The
target is a single, coherent, **GitHub-PR-style review experience**.

### What already exists (this is mostly consolidation, not a from-scratch build)

The Go backend already implements most of the model end-to-end:

- **`GET /review`** assembles a composite read model: full multi-file diff +
  threads + merge strategy.
- **First-class threads**, entity-scoped at
  `/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/threads`
  (POST open `{filePath,line,side,body}`, POST `:id/replies`, PATCH `:id`
  `{isResolved}`, GET list **dual-served as a live WebSocket stream**),
  event-sourced, SQLite-projected, broadcast on every mutation.
- **Merge-into-parent** fully wired: `POST /workspaces/:wsId/merge-into-parent
  {strategy}` → async (202) → three engine strategies (**Merge / Squash /
  Rebase-then-FF**) with guards (`ErrParentLocked`, leaf-only `ErrRebaseNonLeaf`)
  and conflict handling (child → `pr-conflicts`, full conflict engine present).
- **Merge eligibility** (`canMergeLocally`, `parentBranch`) rides on every
  `WorkspaceDTO`.
- The diff parser yields rich `MultiFileDiff` (per-file hunks, per-line old/new
  numbers, stable `hunkId`s).
- A **changed-files directory tree** (`status/git-status-panel.tsx`) is fully
  built but **dead code**; the **virtualized multi-file viewer**
  (`git-diff-multi-file.tsx`) exists but never scrolls to its target file.

The redesign **picks the canonical pieces, wires them into one flow, deletes the
duplicates, and fills a few small backend gaps.**

---

## 2. Goals / Non-goals

### Goals
1. **One Git sidebar** showing a **blended directory tree** of every file that
   differs from the parent — committed *and* uncommitted — reusing the
   file-explorer tree primitives.
2. **One unified review tab**: clicking a file opens a single full-width tab with
   every changed file's diff stacked, **scrolled to the clicked file**.
3. **Inline, line-anchored comment threads** (single line + range), GitHub-style,
   that expand into human↔agent conversations, with **Markdown (Write/Preview)**
   and **real GitHub identities**.
4. **A merge section** in the sidebar: when the parent workspace is **local and
   not protected**, merge via **Merge / Squash / Rebase & Merge**.
5. **Commit inline**: commit uncommitted work from the sidebar so it becomes part
   of the merge.

### Non-goals (explicitly deferred)
- **GitHub PR comment sync** (mirroring/round-tripping comments to an open
  github.com PR). Designed-around but out of POC scope; a clean fast-follow.
- Side-by-side ("split") diff is a toggle on the unified renderer, not a
  separate system; unified is the default and the v1 focus.
- No new auth system; identity comes from the existing `gh` CLI + git config.

---

## 3. Core decisions (locked)

| Area | Decision |
|---|---|
| **Review scope** | **Blended** — one tree shows everything differing from parent (committed + uncommitted), with inline commit affordances. |
| **Tree treatment** | One directory tree; **only uncommitted files carry an amber `uncommitted` pill** (committed is the implied default). M/A/D status + `+/−` counts per row. |
| **Diff tab** | **Full-width, no in-tab file tree.** File navigation via a toolbar file-dropdown + the app Git sidebar. Unified default, Split toggle. |
| **Comments** | Inline, line-anchored, **single + range**. Posts **immediately** (no batched "submit review"). Resolve/reopen collapses. Drifted anchors become **Outdated** (kept, not deleted). |
| **Comment body** | **Markdown** with **Write/Preview** tabs + formatting toolbar, rendered via the existing `react-markdown + remark-gfm` renderer (`panes/lib/markdown.tsx`). **Fenced-code highlighting** needs **shiki wired in** (it's a dependency but currently unused by the renderer), else code blocks ship CSS-styled-but-untokenized for v1. |
| **Identity** | Human authors show **GitHub name + `@login` + avatar** (resolved via `gh api user`, cached); fallback to `git config user.name` + initials avatar. Agents show name + **AGENT** badge + agent avatar. Identity is **stored on the message at post time**. |
| **Diff renderer** | **`react-diff-view`** (purpose-built for review UIs), **not Monaco**. Inline threads via its `widgets` slot (keyed by `getChangeKey`); **our existing tree-sitter tokens** are fed in via a custom `tokenize` + `renderToken` so colors match the editor exactly (no refractor/prism); split/unified via `viewType`; the "+" gutter affordance via `renderGutter`/`gutterEvents` (which expose `{change, side}`). **It does not virtualize — we own that** (virtualize the file stack; lazy-render very large files). Retires both the Monaco diff stack and the bespoke `git-diff-text`/`git-diff-line` renderer; theme parity is preserved because we reuse the same tree-sitter tokenizer + `--syntax-*` palette the editor uses. |
| **Review diff base** | `git diff <mergeBase(parentBranch, HEAD)>` evaluated **in the worktree** (working-tree-inclusive), so it shows exactly this workspace's changes (committed + uncommitted) since it forked. Per-file uncommitted flag from `git status`. |

---

## 4. Architecture

### 4.1 Surfaces
- **Git sidebar panel** (`features/git`, mounted in `sidebar-carousel`): branch
  header → parent, ahead/behind + sync, `Changes | History` tabs, the blended
  changed-files tree, a pinned commit bar, and the merge section.
- **Unified review tab** (`branchReview` buffer, rendered in `pane-container`):
  full-width stacked multi-file diff with inline threads, a toolbar
  (file-dropdown, `+/−`, Unified/Split, Viewed), per-file headers (path, stats,
  `uncommitted` pill, Viewed).
- **Thread** (inline component reused in the diff): anchor header, human/agent
  messages (Markdown), reply composer (Write/Preview), resolve control.

### 4.2 Data flow
1. Sidebar reads `gitStatus` (existing, websocket-synced) **and** the review diff.
2. Clicking a file sets `branchReview.activeFileKey` and opens/focuses the single
   `branchReview` tab (dedup on `wsId`); the multi-file viewer effect calls
   `virtualizer.scrollToIndex(idx)` for that key.
3. The review tab loads `GET /review` (diff + threads + strategy) and subscribes
   to the **`/threads` WebSocket** for live thread updates (human + agent).
4. Comment writes go to the first-class `/threads` REST endpoints (optimistic),
   reconciled by the WS stream.
5. Merge reads eligibility off the `WorkspaceDTO`; `POST /merge-into-parent`
   triggers the async merge; outcome/conflict arrives on the workspace WS stream.

---

## 5. Backend changes (Go)

Small, additive — the heavy lifting exists.

1. **Working-tree-inclusive review diff.** Add an engine method
   `DiffAgainstRef(worktreePath, ref) → MultiFileDiff` (`git diff <ref>`), and
   change `branchReviewUsecase.Get` to use `ref = mergeBase(base, HEAD)` instead
   of the 3-dot `RangeDiff`. Stamp each `FileDiff` with an **`uncommitted`**
   boolean derived from `git status` (file has working-tree changes). *(Keep
   `RangeDiff` for any committed-only callers.)*
2. **Range anchors on threads.** Add `StartLine`/`EndLine` (or a `LineRange`) to
   `domain.ReviewThread` + `OpenReviewThread` command + `OpenThreadInput` + the
   `/threads` POST DTO. Single-line = `StartLine == EndLine`. Render unchanged
   for single-line.
3. **Author identity + isAgent on the live thread wire.** The gap is at the
   **HTTP-handler layer only**: `ThreadDTO`/`ThreadReplyDTO` already carry an
   `author` string (always empty today) but **no `isAgent`**, and the `/threads`
   `POST`/`reply` handlers never pass `Author`/`IsAgent` into the repo. The
   `reviewthread.OpenInput` struct and the asynx commands already accept+persist
   both. Fix: handlers accept/stamp **author identity**
   (`{ login, displayName, avatarUrl, isAgent }`), and the DTOs **add `isAgent`**
   (+ richer author fields). No domain/repo change for authorship.
4. **Identity resolver.** Add a small usecase that returns the current human
   identity: `gh api user` (`login`, `name`, `avatar_url`), cached; fallback to
   `git config user.name`/`user.email` + a deterministic initials avatar. Agent
   identity is supplied by the agent runtime (name + `isAgent: true`); for the
   POC, agent posting is stubbed/manual but the field is honored.
5. **Outdated detection.** Persist a short **anchor snapshot** (the anchored
   line's content + a few lines of context) with each thread so the FE/back can
   mark a thread `outdated` when the recomputed diff no longer matches. (May live
   FE-side first; schema field is cheap to add.)
6. **No backend work needed** for merge execution, eligibility, conflict
   handling, or the diff DTO shape — all present.

---

## 6. Frontend changes (React / `features/git`)

### 6.1 Sidebar (`git-panel.tsx`, new tree)
- Replace the flat `git-changes-panel.tsx` `FileRow` list with a **blended
  directory tree**. Salvage `status/git-status-panel.tsx`'s tree builder
  (`buildGitFolderTree`) **or** drive `FileExplorerTree` from a synthetic
  `FileEntry[]` built from the review diff's file list — reusing
  `FileExplorerIcon`, `SidebarTreeRow`, and the visible-rows virtualizer.
  **Caveat:** `FileExplorerTree` reads a single module-level `useFileTreeStore`
  for expansion (no caller-facing override — `expandedPathsOverride` is an
  internal search-only hook arg), so a review tree reusing it would **share**
  expand state with the main explorer. Either **parameterize the expand store**
  or favor the salvaged `buildGitFolderTree` (which keeps its own collapsed-set
  state). Canonical tree is `file-explorer/file-explorer/components/file-explorer-tree.tsx`
  (the sibling path is a re-export shim).
- Rows: `M/A/D` status, name, amber `uncommitted` pill where applicable,
  `+/−` counts. Click → open/scroll the review tab (§6.2).
- Branch header: `branch → parent`, ahead/behind, **sync** button (fix the stale
  `Not implemented: git_pull` Tauri stub — backend `Pull`/`Push` work).
- Pinned **commit bar** (amber when uncommitted exist): message + `Commit N`,
  using the existing commit API.
- **Merge section** (§6.4).

### 6.2 Unified review tab (`branch-review-pane.tsx` → single page)
- Collapse the About/Git/Diff sub-tabs into **one full-width page**: toolbar +
  a **virtualized stack of `react-diff-view` `<Diff>`s, one per file** (we own the
  file-stack virtualization since the library renders all lines itself; repurpose
  `git-diff-multi-file`'s tanstack stack as that shell).
- **Diff input:** map the backend `MultiFileDiff` (already-parsed hunks/lines)
  into `react-diff-view`'s `hunks`/`File` shape (or expose a raw unified-diff
  passthrough and use its `parseDiff`). Prefer mapping to avoid re-parsing.
- **Tokens:** a custom `tokenize` runs the existing tree-sitter `tokenizeByLine`
  (`use-git-diff-highlight`) and emits `react-diff-view` tokens; `renderToken`
  applies the same `.token-*` classes → `--syntax-*`.
- **Scroll-to-file:** `virtualizer.scrollToIndex(indexOf(activeFileKey))`, driven
  by a new `branchReview.activeFileKey` action set on sidebar clicks (the tab
  dedups on `wsId`, so re-clicking must scroll, not reopen).
- Per-file headers: path, `+/−`, `uncommitted` pill, **Viewed** checkbox.
- Retire the `diff` buffer + `DiffPane` + the Monaco `git-diff-editor-stack`/
  `surface` for review. (The working-tree-only `diff` buffer flow is subsumed by
  the blended review tab.)

### 6.3 Inline comments
- **Anchor** threads via `react-diff-view`'s `widgets` map, keyed by
  `getChangeKey(change)`; `side` (old/new) comes from the change type and maps to
  our `left`/`right`. A **range** thread anchors at its last line's change key and
  stores `{start,end}`; render the spanned lines via a decoration/`pickRanges`
  highlight.
- **"+" affordance** + **range drag/shift-select** via `renderGutter` /
  `gutterEvents.onClick` (hover state available through `inHoverState`).
- Reuse `review-thread-item.tsx` (already renders human/agent rows + reply) as
  the inline thread body rendered inside the widget; **retire** the form-based
  `review-thread-panel.tsx` and the `diff-pane.tsx` ephemeral composer.
- **Composer:** Write/Preview tabs + formatting toolbar; **Preview** renders via
  `panes/lib/markdown.tsx` (react-markdown + remark-gfm). For highlighted fenced
  code in the preview, **wire shiki** (already a dependency, currently unused by
  the renderer) into a code-block component; otherwise v1 ships CSS-styled code
  blocks. `@mention` hint (agent wiring later). `⌘↵` to post.
- **Identity:** render GitHub name/`@login`/avatar from the message's stored
  identity; AGENT badge + agent avatar for agents; the agent **commit chip**
  when an agent message references a pushed commit.
- **Threads API:** realign `review-api.ts` to the first-class **`/threads`**
  endpoints + subscribe to the `/threads` WS. Today the FE methods are stale on
  two axes — an extra `/review` segment and `reply` vs `replies`: fix to
  `openThread` → POST `${workspaceBase}/threads`, `replyToThread` → POST
  `${workspaceBase}/threads/:id/replies` (plural), `setThreadResolved` → PATCH
  `${workspaceBase}/threads/:id`. (`getReview`/`setMergeStrategy` correctly stay
  under `/review`.) Add a **reopen** path — the slice's `resolveReviewThread` is
  one-way today, but the backend supports resolved→open. Optimistic
  add/reply/resolve reconciled by the WS stream.
- **Outdated:** when a thread's anchor no longer matches the recomputed diff,
  render it collapsed with an `OUTDATED` badge + its original hunk; never drop it.

### 6.4 Merge section
- Strategy segmented control (`Merge / Squash / Rebase`) → `PATCH /review`
  (existing `setMergeStrategy`).
- Primary **"Merge into <parent>"** → `POST /merge-into-parent {strategy}`;
  handle the 202 + workspace-WS outcome.
- **States** (all from data we have): **eligible** (`canMergeLocally` true) →
  enabled + "local & unprotected"; **uncommitted present** → disabled, "commit
  first"; **protected/remote parent** (`canMergeLocally` false) → disabled +
  "open a pull request ↗"; **post-conflict** (child `pr-conflicts`) → "Resolve N
  conflicts" routed through the existing conflict engine
  (`/git/operation/continue|abort`).

---

## 7. File-level plan (keep / build / retire)

**Keep / extend**
- `review-api.ts` (realign to `/threads`, add merge call), `branch-review-slice.ts`
  (+`activeFileKey`, +range, +identity), `git-diff-multi-file.tsx` (repurpose its
  tanstack stack as the **per-file virtualization shell** around `react-diff-view`),
  `use-git-diff-highlight` (tree-sitter tokenizer, adapted to emit `react-diff-view`
  tokens), `review-thread-item.tsx`, `review-about-tab` merge selector,
  `file-explorer-tree*` + `FileExplorerIcon`, `git-status-api` staging, the
  `/git/status` websocket sync.

**Build**
- Add the **`react-diff-view`** dependency; a `MultiFileDiff → hunks` mapper; a
  custom `tokenize`/`renderToken` bridging tree-sitter → `--syntax-*`; `widgets`/
  `renderGutter` wiring for threads + the "+" affordance; file-stack virtualization.
- Blended changed-files tree component (sidebar) with dedicated expand store.
- Scroll-to-file wiring; range select + thread overlay.
- Write/Preview Markdown composer (+shiki for code fences); identity rendering;
  merge-section state machine; thread reopen path.
- Backend: `DiffAgainstRef` + working-tree-inclusive review diff + `uncommitted`
  flag; range anchors; identity + `isAgent` on `/threads` DTO + resolver; outdated
  snapshot.

**Retire**
- `git-changes-panel.tsx` flat `FileRow`; the bespoke `git-diff-text`/`git-diff-line`
  renderer (replaced by `react-diff-view`); `diff-pane.tsx` ephemeral comments;
  `git-diff-editor-stack/surface` + `monaco-diff-editor-view` (verify unused) for
  review; `review-thread-panel.tsx` form; the `diff` buffer review path; the
  `branchReview` About/Git/Diff sub-tab shell; stale `git_pull/push/fetch` Tauri
  stubs.

---

## 8. Testing
- Per `CLAUDE.md`: tests mirror under `web/src/__tests__/...`, `@/` imports.
- FE units: tree builder (paths → tree, uncommitted flag), scroll-to-file index
  resolution, range-anchor mapping (line ↔ `{filePath, start, end, side}`),
  outdated detection, Markdown render, identity fallback.
- BE (per black-box convention, `TestRegression_*` in `api/tests`, integration
  tag): `DiffAgainstRef` working-tree-inclusive output + `uncommitted` flag;
  `/threads` POST/reply carrying author identity + isAgent + range; merge-strategy
  + merge-into-parent paths already covered — extend for protected/uncommitted
  gating decisions surfaced to the DTO.
- Live Tauri verification (per project convention) before claiming any UI "works".

## 9. Risks / open items
- **Thread drift in the blend** is the main risk — anchors move as you edit
  uncommitted lines. Mitigation: anchor snapshot + Outdated state (§5.6, §6.3).
- **`react-diff-view` does not virtualize** — it renders all lines of every
  `<Diff>`. We virtualize the **file stack** (mount `<Diff>` per visible file) and
  must **lazy-render very large files** (hunk pagination) to stay smooth; the
  comment `widgets` (variable-height rows) must coexist with that.
- **Agent posting** is honored in the data model but the producer (agent runtime)
  lands in the agentic phase; POC ships human threads + the AGENT-rendering path.
- **Identity caching/staleness** (avatar/login) — cache with a TTL; store on the
  message so historical attribution is stable.

## 10. Deferred (post-POC)
- GitHub PR comment sync (read + write + resolve against an open PR).
- Split-view polish; per-hunk staging inside the review tab; richer commit UX
  (per-file/-hunk staging) beyond "commit N".
