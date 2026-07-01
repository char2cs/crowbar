# Git Review REBUILD (v2, FINAL) — native tokenized DOM renderer, no Monaco

> Supersedes the Monaco rebuild plan (rejected: Monaco can't anchor inline threads). REQUIRED SUB-SKILL: superpowers:subagent-driven-development.
> **VERIFICATION RULE (non-negotiable):** every visual phase ends with a REAL screenshot — `python3 /tmp/shot.py /tmp/<name>.png` then `Read` it — and is shown to the user for confirmation. NO DOM-attribute "verification" ever again. The shot.py bridge works without OS permission.

**Direction (user-confirmed):** the review diff renders on the app's **native tokenized DOM renderer** (`git-diff-text.tsx` / `git-diff-line.tsx`) — same tree-sitter highlighting as the editor, inline comments are plain DOM rows between lines. NO Monaco, NO react-diff-view. Plus: fix tree-sitter tokenization (assets unprovisioned), one full-width review tab (no About/Git/Diff), state-based sidebar, file-explorer git decorations.

**What's correct and stays:** backend (Plan 1); blended sidebar tree + `uncommitted` pill; the `/threads` REST+WS + identity + markdown(shiki) DATA layer (Plan 4 — backend round-trip verified); `branch-review-slice` threads/activeFileKey; `review-thread-item.tsx` + `comment-composer.tsx` + `use-current-identity.ts`; the native `git-diff-text`/`git-diff-line` renderer (still present).

## Global Constraints
- Production quality: clean, tested, `@/components/ui/*` + CSS tokens only, no hardcoded colors, kebab-case, narrow zustand selectors, `getState()` only in handlers/effects.
- **Never `npm install`** — pnpm only (npm broke react/react-dom). Tests mirror `web/src/__tests__/`; `cd web && ./node_modules/.bin/vitest run <p>`.
- Diff colors from `--git-added`/`--git-deleted`; tokens from the existing `.token-*` → `--syntax-*`; file decorations from `--git-modified`/`--git-added`/`--git-untracked`.
- Each phase: implement → `vitest` green + `tsc` no new errors → **shot.py screenshot + Read + show user**.

---

### Phase A — Fix tree-sitter tokenization (the "WHERE IS THE TOKENIZATION" bug)

Highlighting is off app-wide because `/tree-sitter/parsers/<lang>/parser.wasm` + `highlights.scm` aren't served (`public/tree-sitter/` absent → vite returns SPA HTML; `useDiffHighlighting`/the editor silently fall back to plain). Fix provisioning so the diff AND editor tokenize.
- Investigate where the parser wasm + highlight queries come from (a `tree-sitter-wasms`/grammar package in node_modules? a vendored dir? a build script?). Find the canonical source.
- Provision them under `/tree-sitter/parsers/<lang>/` for **dev (vite) and build** — e.g. a vite static-copy plugin or a `public/tree-sitter/` populated by a `pnpm` postinstall/predev script. Cover at minimum: ts, tsx, js, jsx, go, json, md, css, html (the languages in `extension-assets.ts`).
- Verify: open a `.tsx` (or the README/Go) diff in the review tab → screenshot → confirm **syntax colors render**. Show user.

---

### Phase B — Rebuild the review diff on the native DOM renderer

I deleted the virtualized multi-file stack (`git-diff-multi-file.tsx`) when I went react-diff-view. Rebuild it natively.
- Create `components/diff/review-diff-view.tsx` (replacing the react-diff-view one): a virtualized (`@tanstack/react-virtual`) stack of per-file sections, each rendering `TextDiffViewer` (`git-diff-text.tsx`, tree-sitter tokenized) for the file's `GitDiff`. Per-file header: path, `+N/−M`, the amber `uncommitted` pill (when `file.uncommitted`), a "Viewed" collapse toggle. Unified/Split toggle (git-diff-text already supports split). Auto-collapse large files (`contentVisibility:auto`).
- `FileDiffSummary` carries `uncommitted` (already added). Scroll-to-file: `virtualizer.scrollToIndex(indexOf(activeFileKey))` driven by `branchReview.activeFileKey`/`activeFileNonce`.
- Verify: screenshot — multi-file tokenized diff in the app font/size/palette, per-file headers, pills, Unified/Split. Show user. **This is the look you approved in mock #01.**

---

### Phase C — Review tab = just the diff; retire react-diff-view + dead Monaco diff

- `branch-review-pane.tsx`: delete the About/Git/Diff `Tabs`/`SUBTABS`; render the new `<ReviewDiffView multiDiff={branchReview.diffCache}/>` full-width with the existing loading/error/empty gating. Keep the `load()` (getReview). Remove `activeSubtab`/`setBranchReviewSubtab` from slice + INITIAL + persisted schema (clear dev IndexedDB; no migration).
- DELETE: `lib/to-diff-view-hunks.ts`, `lib/render-tree-sitter-token.tsx`, `lib/diff-change-key.ts`, `components/diff/__react-diff-view-smoke.tsx`, `components/review-about-tab.tsx`, `components/review-thread-panel.tsx`, `components/review-diff-tab.tsx` (folded into B/C); the dead `components/diff/monaco-diff-editor-view.tsx`. Remove `react-diff-view` from `web/package.json` + reinstall with **pnpm**. Delete the 5 react-diff-view tests. (Keep `lib/diff-highlight-shared.ts` if still imported, else delete.)
- Verify: `vitest` green; `tsc` no new errors; screenshot — one full-width tokenized review diff, no sub-tabs. Show user.

---

### Phase D — Inline comment threads as DOM rows (native, the original design)

Per the original comments seam-map: build the gutter "+" + anchored thread rows INTO `git-diff-line.tsx` / `git-diff-text.tsx` (not react-diff-view). Change key = `filePath + side('old'|'new') + line`.
- `git-diff-line.tsx`: hover gutter shows a "+"; click → an inline `CommentComposer` (Write/Preview) anchored to `{filePath, side, line}` (+ range via shift-select). Submit → `openThread(wsId, {...anchor, author: identity.login, isAgent:false, body})`.
- `git-diff-text.tsx` / `review-diff-view.tsx`: render thread widgets as full-width rows interleaved after their anchored line, matched from `branchReview.threads` (filtered by file) by `{filePath,side,line}`. Reuse `ReviewThreadItem` (markdown via MarkdownPreview, GitHub avatar+identity, AGENT badge, two-way resolve/reopen, outdated when the anchor line no longer exists). Thread state from the store (WS-synced); optimistic add.
- Pass `wsId` + filtered threads + an `onAddComment` down through the stack to each `DiffLine`.
- Verify: screenshot — hover a line shows "+"; click opens composer; a posted thread renders as a row under the line with avatar/markdown/AGENT; resolve collapses. **Post a test thread, screenshot, show user, then DELETE it from the daemon DB** (`~/.crowbar/state/view.db` + event stream) — no fabricated data left behind.

---

### Phase E — State-based sidebar merge (#10)

- `git-panel.tsx` bottom region conditional on `uncommittedCount>0`: uncommitted → ONLY `GitCommitPanel`; clean & ahead & eligible → ONLY `MergeSection`. Never both. `MergeSection` drops the disabled "commit first" button (that state shows the commit box instead). Protected/conflict swap the region per `resolveMergeState`.
- Verify: screenshot with uncommitted work (commit only), then after committing (merge only). Show user.

---

### Phase F — File-explorer git decorations (#11)

- `file-explorer-tree-item.tsx`: color the filename by git status from `useGitStore` workspace status (`createFileTreeGitStatusLookup`/`file-tree-git-status`): modified→`--git-modified`, added→`--git-added`, untracked→`--git-untracked`, deleted→`--git-deleted`; trailing `M/A/U/D`; tint parent folders containing changes.
- Verify: screenshot Files tab — colored like mock #03. Show user.

---

### Phase G — Retire the standalone "Uncommitted Changes" buffer

- `use-git-diff-handlers.ts` `handleViewFileDiff` (the only producer of the `diff://working-tree/all-files` / "Uncommitted Changes" buffer): repoint so clicking a changed file opens ONLY the review tab + `setBranchReviewActiveFile(fileKey)`. Keep the `'diff'` buffer machinery for commit/stash/tag diffs (still used). 
- Verify: screenshot — clicking a file no longer opens a second diff tab; only the review tab, scrolled to the file. Top tab bar shows no duplicate diff tabs. Show user.

## Self-review
- All 5 user corrections + tokenization + retirement covered; NO Monaco. Comments are DOM rows (the easy, original design) — no view zones.
- Verification = REAL screenshots + user confirm every visual phase. The silent-DOM-check failure cannot recur.
