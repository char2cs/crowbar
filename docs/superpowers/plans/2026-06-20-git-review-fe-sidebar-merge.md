# Git Review FE — Sidebar + Merge (Plan 2 of 4)

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]`.

**Goal:** Replace the flat git "Changes" sidebar with a blended changed-files **directory tree** (committed + uncommitted in one tree; only uncommitted files get an amber pill), a pinned commit bar, and a **merge section** (Merge/Squash/Rebase + state machine) that merges the branch into its parent locally.

**Architecture:** The sidebar's git tab (`git-panel.tsx`) recomposes its "Changes" body into: scrollable blended tree (top) + pinned `GitCommitPanel` + merge section (bottom). The blended file set comes from `GET .../review` (`getReview(wsId).diff.files`, each `FileDiff` now carrying `uncommitted`). Merge eligibility (`canMergeLocally`, `parentBranch`) is read from `useSidebarStore`; merge fires `POST .../merge-into-parent {strategy}`.

**Tech Stack:** React 19, zustand, kebab-case files, `@/` imports, vitest (tests mirror under `web/src/__tests__/`).

## Global Constraints
- Components: `@/components/ui/*` + CSS-variable tokens only; never hardcode colors. Amber pill uses an existing warning/amber token, not a hex.
- Files kebab-case; exported component PascalCase. Stores: narrow selectors; `getState()` only in handlers/effects.
- Tests mirror `web/src/` under `web/src/__tests__/`, `@/` imports. Run: `cd web && pnpm vitest run <path>`.
- Do NOT reuse the global `useFileTreeStore` for the git tree (it's keyed by real FS paths and shared with the Files tab) — use **local** collapse state keyed by folder path, as the salvaged `git-status-panel.tsx` does.
- The blended tree's data source is `getReview(wsId).diff.files` (committed+uncommitted). Working-tree staging/commit still operate on `useGitStore.gitStatus`. Keep both straight (see Task 5).
- `MergeStrategy = 'merge' | 'squash' | 'rebase'` (from `branch-review-slice.ts`).
- Verify the result live in the running Tauri app before claiming the sidebar "works" (per project convention) — covered by the Plan-2 smoke check at the end.

---

### Task 1: FE types + merge client

**Files:**
- Modify: `web/src/features/git/types/git-types.ts` (add `uncommitted?: boolean` to `GitDiff`)
- Modify: `web/src/features/git/api/review-api.ts` (add `mergeIntoParent`)
- Test: `web/src/__tests__/features/git/api/review-api.test.ts` (extend or create — assert `mergeIntoParent` POSTs the right path/body)

**Interfaces produced:**
- `GitDiff.uncommitted?: boolean`
- `mergeIntoParent(wsId: string, strategy: MergeStrategy): Promise<void>` — `POST ${workspaceBase(wsId)}/merge-into-parent` with `{ strategy }` (202 async, no body used).

- [ ] **Step 1:** Add `uncommitted?: boolean` to the `GitDiff` interface in `git-types.ts` (next to `additions?`/`deletions?`), with a doc comment: `// true when the file has working-tree changes not yet committed (blended review diff).`
- [ ] **Step 2:** Write a failing test for `mergeIntoParent` that mocks `apiFetch` and asserts it calls `POST ${workspaceBase(wsId)}/merge-into-parent` with `{ strategy: 'squash' }`. Mirror the existing `setMergeStrategy` test if one exists; otherwise model the mock on how other `review-api` methods are tested. Run it; confirm it fails (method undefined).
- [ ] **Step 3:** Implement `mergeIntoParent` in `review-api.ts` mirroring `setMergeStrategy`'s `apiFetch` + `workspaceBase` usage, POSTing `{ strategy }`. (Reuse the existing `MergeStrategy` import.)
- [ ] **Step 4:** Run the test; confirm pass. `cd web && pnpm tsc --noEmit` clean for these files.
- [ ] **Step 5:** Commit: `feat(git-fe): GitDiff.uncommitted type + mergeIntoParent client`.

---

### Task 2: Extract the folder-tree builder into a shared, tested lib

**Files:**
- Create: `web/src/features/git/utils/build-git-folder-tree.ts` (move `buildGitFolderTree` + helpers `createFolderNode`, `normalizePathSegments`, `sortFoldersByName`, `sortFilesByPath`, `collectNodeFiles`, and the `GitFolderNode` type out of `status/git-status-panel.tsx`)
- Test: `web/src/__tests__/features/git/utils/build-git-folder-tree.test.ts`

**Interfaces produced:**
- `buildGitFolderTree(files: GitFile[]): GitFolderNode` (and the generic shape — keep the exact current signature; if it's currently `GitFile[]` keep that, the tree consumes `path`/`status`).
- `GitFolderNode { name, path, folders: GitFolderNode[], files: GitFile[] }` (match the existing shape exactly).

- [ ] **Step 1:** Read `status/git-status-panel.tsx` lines ~98–152 to copy `buildGitFolderTree` + helpers + the `GitFolderNode` type verbatim into the new lib file. Keep the function pure (no React). Adjust imports (`@/` paths).
- [ ] **Step 2:** Write a unit test: given files `['src/a.ts','src/sub/b.ts','README.md']`, assert the tree nests `src` with a child `sub`, files sorted, root file `README.md` at top level. Run; it should pass immediately (pure move) — if it fails, the move was lossy; fix.
- [ ] **Step 3:** Re-point `status/git-status-panel.tsx` to import from the new lib (temporary — the file is deleted in Task 5; this keeps the build green in between). Run `pnpm tsc --noEmit` clean.
- [ ] **Step 4:** Commit: `refactor(git-fe): extract buildGitFolderTree to shared util`.

---

### Task 3: Blended changed-files tree component

**Files:**
- Create: `web/src/features/git/components/changed-files-tree.tsx` (the blended tree)
- Modify: `web/src/features/git/components/status/git-status-file-item.tsx` (add `uncommitted?: boolean` prop → amber pill)
- Test: `web/src/__tests__/features/git/components/changed-files-tree.test.tsx`

**Interfaces consumed:** `buildGitFolderTree` (Task 2), `GitFileItem`, `SidebarTreeRow`/`SidebarTreeDisclosure` (`@/components/ui/sidebar-tree`), `useGitDiffHandlers().handleViewFileDiff`.
**Interfaces produced:** `ChangedFilesTree({ files, repoPath, onFileOpen })` where `files` are review `GitDiff[]` (carrying `uncommitted`); renders a directory tree with local collapse state; clicking a leaf calls `onFileOpen(filePath)`.

- [ ] **Step 1:** Add an `uncommitted?: boolean` prop to `GitFileItem`. When true, render a small amber pill (`uncommitted`) using the warning/amber token + `@/components/ui` Badge (or the existing pill style in the file). Keep icon/diffstats/checkbox. Add a vitest assertion that the pill renders only when `uncommitted` is true.
- [ ] **Step 2:** Write a failing test for `ChangedFilesTree`: given a 3-file review diff where one file has `uncommitted: true`, assert (a) the directory structure renders, (b) exactly one amber `uncommitted` pill appears, (c) clicking a leaf calls `onFileOpen` with its path.
- [ ] **Step 3:** Implement `ChangedFilesTree`: build the tree from `files` via `buildGitFolderTree` (adapt: the builder takes `GitFile[]`-ish with `path`; map `GitDiff.file_path`→`path` and carry `uncommitted` + `additions`/`deletions` onto the node's file objects, or generalize the builder's input type to `{ path: string; uncommitted?: boolean; additions?: number; deletions?: number }`). Render folders with `SidebarTreeDisclosure` + local `useState<Set<string>>` collapse (keyed by folder path), and leaves with `GitFileItem` passing `uncommitted`. One uniform row per file; no staged/unstaged split.
- [ ] **Step 4:** Run the test; confirm pass. `pnpm tsc --noEmit` clean.
- [ ] **Step 5:** Commit: `feat(git-fe): blended changed-files directory tree with uncommitted pill`.

---

### Task 4: Merge section component + state machine

**Files:**
- Create: `web/src/features/git/components/merge-section.tsx`
- Create: `web/src/features/git/lib/merge-section-state.ts` (pure state resolver)
- Test: `web/src/__tests__/features/git/lib/merge-section-state.test.ts`

**Interfaces consumed:** `setMergeStrategy` + new `mergeIntoParent` (Task 1), `useSidebarStore` (for `canMergeLocally`/`parentBranch` of the active ws), workspace status (`pr-conflicts`), the blended diff's uncommitted count.
**Interfaces produced:**
- `resolveMergeState(input: { canMergeLocally: boolean; hasUncommitted: boolean; status: string }): { kind: 'eligible' | 'uncommitted' | 'protected' | 'conflict'; reason: string }` (pure).
- `MergeSection({ wsId, parentBranch, canMergeLocally, hasUncommitted, status })` — strategy selector + a primary button whose enabled/label/intent follow `resolveMergeState`.

- [ ] **Step 1:** Write a failing unit test for `resolveMergeState` covering all four kinds: `status==='pr-conflicts'`→`conflict`; `!canMergeLocally`→`protected`; `canMergeLocally && hasUncommitted`→`uncommitted`; `canMergeLocally && !hasUncommitted && status!=='pr-conflicts'`→`eligible`. (Conflict takes precedence over everything; protected over uncommitted.)
- [ ] **Step 2:** Implement `resolveMergeState` with that precedence. Run; confirm pass.
- [ ] **Step 3:** Implement `MergeSection`: a `Merge/Squash/Rebase` segmented selector reusing the optimistic-PATCH pattern from `review-about-tab.tsx`'s `MergeStrategySelector` (bind to `branchReview.mergeStrategy`, optimistic `setMergeStrategy` + rollback/toast). Below it, the primary action: for `eligible` → enabled "Merge into <parentBranch>" calling `mergeIntoParent(wsId, strategy)` then toast "Merging…"; for `uncommitted` → disabled + "Commit your N change(s) first"; for `protected` → disabled + "<parentBranch> is protected — open a pull request"; for `conflict` → a destructive "Resolve N conflicts" affordance (wire to the existing conflict flow entry if present, else a toast placeholder noting the conflict — note this in the report). Use `@/components/ui` Button variants + tokens. Show the eligibility line ("<parentBranch> is local & unprotected") for `eligible`.
- [ ] **Step 4:** Add a render test (`merge-section.test.tsx`) asserting the button is disabled with the right copy for `uncommitted` and `protected`, enabled for `eligible`. Run; confirm pass. `pnpm tsc --noEmit` clean.
- [ ] **Step 5:** Commit: `feat(git-fe): merge section with eligibility state machine`.

---

### Task 5: Recompose the Changes tab + retire the flat panel

**Files:**
- Modify: `web/src/features/git/components/git-panel.tsx` (Changes body = tree + commit bar + merge section)
- Create: `web/src/features/git/hooks/use-review-diff.ts` (fetch+cache the blended review diff for the active ws, refresh on `git-status-changed`)
- Delete: `web/src/features/git/components/git-changes-panel.tsx`, `web/src/features/git/components/status/git-status-panel.tsx`
- Modify: any imports referencing the deleted files
- Test: `web/src/__tests__/features/git/hooks/use-review-diff.test.ts` (smoke: fetches `getReview`, exposes `files` + `uncommittedCount`)

**Interfaces produced:** `useReviewDiff(wsId): { files: GitDiff[]; uncommittedCount: number; loading: boolean }` — calls `getReview(wsId)`, stores into `branchReview.diffCache` (reuse the slice), recomputes on the `git-status-changed` window event (same event `use-git-diff-handlers` already listens to).

- [ ] **Step 1:** Implement `use-review-diff.ts`: on mount + on `git-status-changed`, call `getReview(activeWsId)`, set `branchReview.diffCache`, derive `files = diffCache.files` and `uncommittedCount = files.filter(f=>f.uncommitted).length`. Guard for no active ws. Write the smoke test (mock `getReview`, assert files surface). Run; confirm pass.
- [ ] **Step 2:** Rewrite the `git-panel.tsx` "Changes" `TabsPanel` body as a flex column: `<ChangedFilesTree files={files} repoPath={repoPath} onFileOpen={handleViewFileDiff}/>` (flex-1, scroll) + `<GitCommitPanel .../>` (shrink-0, `stagedFilesCount` from `useGitStore.gitStatus`) + `<MergeSection .../>` (shrink-0) reading the active ws's `canMergeLocally`/`parentBranch`/`status` from `useSidebarStore` and `hasUncommitted = uncommittedCount>0`. Keep the History tab untouched. Keep or remove the GitPullRequest→branch-review button (the merge section supersedes it for merging; the button can still open the full Diff review tab — keep it, relabel to "Open review" if helpful).
- [ ] **Step 3:** Delete `git-changes-panel.tsx` and `status/git-status-panel.tsx`. Fix any imports. Run `pnpm tsc --noEmit` and `pnpm vitest run web/src/__tests__/features/git` — confirm green, no dangling references.
- [ ] **Step 4:** Commit: `feat(git-fe): blended sidebar — tree + commit + merge; retire flat changes panel`.

---

### Task 6: Live Tauri smoke check (Plan-2 gate)

**Not a code task — a verification gate.** The controller (not a subagent) runs this via the Tauri MCP after Tasks 1–5 land.

- [ ] **Step 1:** Build the web bundle + sidecar and launch the Tauri app (per the project's run procedure). Open a workspace that has a parent (so `canMergeLocally` can be true) with both a committed branch change and an uncommitted edit.
- [ ] **Step 2:** Via the Tauri MCP webview, open the Git sidebar tab and confirm: the changed files render as a **directory tree**; the uncommitted file shows the **amber pill** and the committed one does not; the **commit bar** and **merge section** are present; the merge button reflects the correct state (eligible vs uncommitted-present). Screenshot.
- [ ] **Step 3:** Click a file → confirm it opens a diff. (Full review-tab behavior is Plan 3; here just confirm the click path works.)
- [ ] **Step 4:** Record the screenshot + observations in the ledger. If anything is visually broken, file findings and fix before declaring Plan 2 done.

## Self-Review
- Spec coverage: blended tree (T3) ✓; amber-pill-only-on-uncommitted (T3) ✓; commit bar pinned (T5) ✓; merge section + 4 states (T4) ✓; merge execution via `mergeIntoParent` (T1/T4) ✓; dedicated local collapse (T2/T3) ✓; retire flat panel (T5) ✓; live Tauri verify (T6) ✓.
- Placeholder scan: the only soft spot is the `conflict` action wiring (T4 Step 3) — flagged to use the existing conflict flow if present, else a toast + report note. Acceptable; not a silent TODO.
- Type consistency: `mergeIntoParent(wsId, strategy)`, `GitDiff.uncommitted`, `resolveMergeState({canMergeLocally,hasUncommitted,status})`, `useReviewDiff(wsId)` are consistent across tasks.
- Open dependency: the blended tree relies on `getReview` returning `uncommitted` per file (Plan 1 Task 2 — landed).
