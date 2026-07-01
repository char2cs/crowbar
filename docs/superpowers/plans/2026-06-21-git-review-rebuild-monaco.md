# Git Review REBUILD — Monaco diff, just-the-diff tab, state-based sidebar, file decorations

> Revised plan after the first build was rejected. REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps `- [ ]`.
> **Verification rule (non-negotiable this time):** every UI task ends with a REAL screenshot via `python3 /tmp/shot.py /tmp/x.png` + `Read` it, AND user confirmation. No DOM-attribute "verification."

**Why this exists:** the shipped build was wrong — react-diff-view looked bad (oversized, untokenized, off-palette), the legacy "Branch Review About/Git/Diff" shell AND the "Uncommitted Changes" Monaco tab were both left in place (3 diff systems), the sidebar showed commit box + disabled merge together, and the Files explorer ignored git status. User-approved mocks: `01-review-tab-fixed`, `02-sidebar-states`, `03-file-explorer-decorations`.

**Locked decisions (user-approved):**
1. The review tab is **one full-width diff on the existing Monaco renderer** (`GitDiffEditorStack`) — **no About/Git/Diff sub-tabs**. Merge strategy lives only in the sidebar; threads render inline.
2. **Inline comment threads** anchored between diff lines, via **Monaco view zones**, unified view (MVP).
3. **Retire** react-diff-view (component + 3 libs + dep + tests) **and** the standalone "Uncommitted Changes" working-tree diff buffer.
4. **State-based sidebar merge:** uncommitted → show only the commit affordance; clean & ahead → show only the merge strategy + button.
5. **File-explorer git decorations:** modified/added/untracked files colored via `--git-*` tokens + M/A/U letter; changed-folders tinted.

**What stays (already works):** backend (Plan 1), the blended sidebar tree + `uncommitted` pill, the `/threads` REST+WS + identity data layer, `branch-review-slice` threads/activeFileKey, `review-thread-item.tsx` + `comment-composer.tsx` + `use-current-identity.ts` (re-wired into the Monaco thread layer), the Monaco renderer `GitDiffEditorStack`/`monaco-diff-editor.tsx`.

## Global Constraints
- `@/components/ui/*` + CSS tokens only. Diff tint uses `--git-added`/`--git-deleted`; decorations use `--git-modified`/`--git-added`/`--git-untracked`.
- Don't re-enable Monaco `editContext` (deliberate perf fix). Don't confuse `DiffMonacoEditor` (normal editor, used) with `monaco-diff-editor-view.tsx` (real createDiffEditor, dead — delete it).
- Thread STATE lives in the store (editors unmount on scroll) and re-attaches on remount.
- Tests mirror `web/src/__tests__/`; `cd web && ./node_modules/.bin/vitest run <p>`. **Never `npm install`** — pnpm only (npm broke react/react-dom last time).
- After any FE change that's visual, capture `python3 /tmp/shot.py` and Read it before claiming anything.

---

### Phase A — Review tab = just the Monaco diff; retire react-diff-view

**A1. Point the review tab at the Monaco renderer.**
- Modify `branch-review-pane.tsx`: delete the `Tabs`/`SUBTABS` (About/Git/Diff). Keep the header + the mount `load()` (getReview → setDescription/MergeStrategy/Conversations/Diff). Body = one full-width diff: reuse `review-diff-tab.tsx`'s idle/loading/error/empty gating but render `<GitDiffEditorStack multiDiff={branchReview.diffCache} />` (the working-tree-buffer branches in the stack are inert when `commitHash !== 'working-tree'`).
- Parameterize `GitDiffEditorStack`: a `title`/`variant` prop so the breadcrumb says "Review" (not "Uncommitted Changes") and the working-tree-only effects (git-status-changed listener, refreshWorkingTreeBuffer, GitHub-commit-URL) are guarded off when `variant==='review'`.
- Remove `activeSubtab`/`setBranchReviewSubtab` from the slice + `INITIAL_BRANCH_REVIEW_STATE` + `BranchReviewPersistedState` (schemas.ts). `setBranchReviewActiveFile` drops the `activeSubtab='diff'` line. (Clear dev IndexedDB; no migration.)
- Verify: screenshot the review tab — confirm one full-width Monaco diff, no sub-tabs, files expandable, app font/size/tint. User confirm.

**A2. Delete react-diff-view.**
- Delete: `components/diff/review-diff-view.tsx`, `components/diff/__react-diff-view-smoke.tsx`, `components/review-about-tab.tsx`, `components/review-diff-tab.tsx` (fold its gating into A1), `components/review-thread-panel.tsx`, `lib/to-diff-view-hunks.ts`, `lib/render-tree-sitter-token.tsx`, `lib/diff-change-key.ts`. Keep `lib/diff-highlight-shared.ts` only if still imported (else delete). Remove `react-diff-view` from `web/package.json`; reinstall with **pnpm**. Delete the 5 associated tests.
- Verify: `vitest run src/__tests__/features/git` green; `tsc` no new errors; screenshot still renders.

---

### Phase B — Move diff tint into Monaco decorations (prereq for view zones)

The red/green tint is a CSS overlay (`DiffLineBackgroundLayer`) positioned by `index*lineHeight`; a view zone would desync every band below it. Move tint to Monaco line decorations so zones don't break it.
- In `monaco-diff-editor.tsx`/`EmbeddedDiffSectionEditor`: replace the `DiffLineBackgroundLayer` overlay with `editor.createDecorationsCollection` adding `isWholeLine` line decorations (`linesDecorationsClassName`/`className` → `.diff-line-added`/`.diff-line-removed` mapped to `--git-added/18`/`--git-deleted/18`). Drive from `lineKinds`.
- Verify: screenshot — tint identical to before (compare to your screenshot #9), now decoration-based. User confirm the diff still looks right.

---

### Phase C — Inline comment threads via Monaco view zones (the hard part)

**C1. Expose the per-file Monaco editor.** Thread a callback ref `onEditorReady(editor: IStandaloneCodeEditor)` up through `CodeEditor` → `DiffMonacoEditor` (currently private) so each file section can attach a comment layer. Store per-(fileKey) editor handles.

**C2. Add-comment affordance.** Reuse `onReadonlySurfaceClick({line,column})` + a gutter hover "+": clicking a line opens an inline `CommentComposer` (Write/Preview) anchored to `{filePath, side, line}` (map view-line→file-line via `actualLines`/`findNearestActualLine`). Submit → `openThread(wsId, {...anchor, author: identity.login, isAgent:false, body})`.

**C3. Render threads as view zones.** For each thread on a file (from `branchReview.threads`, filtered by filePath), compute its view-line (file-line→view-line via `actualLines`), then `editor.changeViewZones(acc => acc.addZone({afterLineNumber, domNode, heightInPx}))`; render `<ReviewThreadItem>` into the zone's `domNode` via `createPortal` (own the React-root lifecycle; dispose on zone/editor removal). **Recompute the section height on `editor.onDidChangeViewZones`** (or make commented sections `scrollable`) so the zone isn't clipped. Unified view only; in split, show a "switch to unified to comment" hint.
- Re-wire the existing `ReviewThreadItem` (markdown via MarkdownPreview, AGENT badge, GitHub avatar, two-way resolve/reopen, outdated) + `use-current-identity`.
- Verify (the big one): screenshot — hover a line shows "+"; clicking opens the composer; a posted thread appears as a real between-lines zone with the thread card; tint stays aligned below it; resolve collapses. **Post a test thread, screenshot, confirm with user, then delete the test thread from the daemon DB** (`~/.crowbar/state/view.db` read_review_threads + event stream) so no fake data lingers.

---

### Phase D — State-based sidebar merge (#10)

- In `git-panel.tsx`: the bottom region is conditional on `uncommittedCount > 0`. If uncommitted → render ONLY `GitCommitPanel` (commit). Else if merge-eligible (clean & ahead) → render ONLY `MergeSection`. Never both. (Protected/conflict states swap the bottom region per `resolveMergeState`.)
- `MergeSection` no longer shows the disabled "commit first" button (that state is now just "show the commit box instead").
- Verify: screenshot both states (with uncommitted work, then after committing) — confirm only one affordance shows. User confirm.

---

### Phase E — File-explorer git decorations (#11)

- In `file-explorer-tree-item.tsx`: read the workspace git status (the tree already has `workspaceGitStatus` via `useGitStore` + `createFileTreeGitStatusLookup` per the file-tree-git-status lib). Color the filename by status (`modified→--git-modified`, `added→--git-added`, `untracked→--git-untracked`, `deleted→--git-deleted`) + a trailing M/A/U/D letter; tint parent folders that contain changes.
- Verify: screenshot the Files tab — modified/added files colored like your mock #03. User confirm.

---

### Phase F — Retire the standalone "Uncommitted Changes" buffer

- `use-git-diff-handlers.ts` `handleViewFileDiff` is the only producer of the `diff://working-tree/all-files` / "Uncommitted Changes" buffer. Repoint it (and the sidebar `handleFileOpen`) so clicking a changed file opens ONLY the branch-review tab + `setBranchReviewActiveFile(fileKey)` (scroll-to-file). Do NOT delete the `'diff'` buffer machinery (commit/stash/tag diffs still use it) — only retire the working-tree production path.
- Verify: screenshot — no "Uncommitted Changes" tab opens on file click; the review tab scrolls to the file. Confirm the top tab bar no longer shows two diff tabs.

## Self-review
- Covers all 5 corrections + retirement. The one risk is Phase C (view zones / height / tint-resync) — explicitly de-risked by Phase B (decorations first) and verified by screenshot at C3.
- Verification is REAL screenshots every visual phase + user confirm — the failure that caused this rebuild cannot recur silently.
