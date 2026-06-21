# Git Review FE — Unified Review Tab via react-diff-view (Plan 3 of 4)

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]`.

**Goal:** Replace the branch-review "Diff" subtab's bespoke renderer with a single full-width unified review tab rendered by **react-diff-view**: all changed files stacked, virtualized at the file-stack level, scroll-to-clicked-file, unified/split, per-file headers (path, +/−, uncommitted pill, Viewed). Syntax colors come from the existing tree-sitter pipeline (custom `tokenize`/`renderToken`), so they match the editor exactly.

**Architecture:** Keep the existing file-stack virtualization shell from `git-diff-multi-file.tsx`; inside each virtual file row render a react-diff-view `<Diff>` fed by a pure `gitDiffToHunks(file)` mapper and a side+line-keyed tree-sitter token map. Clicking a file in the sidebar sets `branchReview.activeFileKey` (+nonce); a `useEffect` scrolls the virtualizer to that file (dedup-safe: the tab dedups on wsId, so re-clicks scroll the live component rather than reopening).

**Tech Stack:** React 19, zustand, @tanstack/react-virtual, **react-diff-view (NEW dep)**, tree-sitter (`@/features/editor/lib/wasm-parser`), vitest.

## Global Constraints
- `@/components/ui/*` + CSS-variable tokens only; no hardcoded colors. Diff add/del colors come from react-diff-view's classes themed against tokens OR the existing `--syntax-*`/git tokens — never hex.
- Files kebab-case; `@/` imports; narrow zustand selectors; `getState()` only in handlers/effects.
- Tests mirror under `web/src/__tests__/`. Run via `cd web && ./node_modules/.bin/vitest run <path>` (pnpm may be off PATH).
- **Syntax colors MUST come from the existing tree-sitter pipeline** (reuse `tokenizeByLine` + the `.token-*` classes bound to `--syntax-*`), NOT react-diff-view's refractor path. Do not add `refractor`.
- **Tokenize lazily** — only tokenize files that are expanded/near the viewport (tree-sitter wasm is async, per-file); never tokenize all files eagerly on first paint.
- `renderToken` must **degrade gracefully** to plain text when tokens are empty (tree-sitter assets can be unprovisioned — `tokenizeByLine` returns an empty Map then).
- Scroll-to-file MUST be driven by a store value that changes on every click (`activeFileKey` + `activeFileNonce`), watched via `useEffect` — NOT a mount-time prop (the tab is dedup-on-wsId and won't remount on re-click).
- Verify live in the running Tauri app before claiming done (Task 6).

---

### Task 1: Install react-diff-view + de-risk render (React 19/Vite/Tauri)

**De-risk first:** confirm the dep installs and a trivial `<Diff>` mounts under React 19 before building the real integration.

**Files:**
- Modify: `web/package.json` (add `react-diff-view`)
- Create: `web/src/features/git/components/diff/__react-diff-view-smoke.tsx` (a tiny dev-only component rendering a hardcoded 1-hunk `<Diff>`) — OR fold the smoke directly into the Task-4 component later; for now a standalone smoke keeps Task 1 self-contained.
- Test: `web/src/__tests__/features/git/diff/react-diff-view-smoke.test.tsx`

- [ ] **Step 1:** Add `react-diff-view` to `web/package.json` dependencies and install (the project uses pnpm; run the install the project uses, e.g. `cd web && pnpm install` — if pnpm is unavailable in your shell, use `corepack pnpm install` or `npx pnpm install`). Import its stylesheet `react-diff-view/style/index.css` once (in the smoke component for now). Confirm install completes with no peer-dep error against React 19 (note any warning in the report).
- [ ] **Step 2:** Write a render test that mounts a component rendering `<Diff viewType="unified" diffType="modify" hunks={HARDCODED_HUNKS}>{hunks => hunks.map(h => <Hunk key={h.content} hunk={h} />)}</Diff>` with a minimal hardcoded `hunks` array (one hunk, two changes) and asserts the rendered output contains the change text. Run: `cd web && ./node_modules/.bin/vitest run src/__tests__/features/git/diff/react-diff-view-smoke.test.tsx`. This proves the API shape + React 19 compat.
- [ ] **Step 3:** If the import/types resolve oddly (react-diff-view ships its own types), confirm `cd web && ./node_modules/.bin/tsc --noEmit` introduces no NEW errors for the smoke files (ignore the pre-existing `otp-field.tsx` error).
- [ ] **Step 4:** Commit: `chore(git-fe): add react-diff-view + render smoke test`. Report the installed version + any peer warnings + the exact `HunkData`/`ChangeData` field names react-diff-view expects (read from its types) — Task 2 needs them.

---

### Task 2: `gitDiffToHunks` — map GitDiff → react-diff-view hunks (pure, tested)

**Files:**
- Create: `web/src/features/git/lib/to-diff-view-hunks.ts`
- Test: `web/src/__tests__/features/git/lib/to-diff-view-hunks.test.ts`

**Interfaces produced:** `gitDiffToHunks(diff: GitDiff): HunkData[]` — walks `diff.lines` (`GitDiffLine { line_type: 'added'|'removed'|'context'|'header', content, old_line_number?, new_line_number? }`). On each `'header'` start a new hunk; for added/removed/context push a `ChangeData` of type `insert`/`delete`/`normal` using the existing 1-based `old_line_number`/`new_line_number`. Compute each hunk's `oldStart/newStart/oldLines/newLines` + a synthetic `content` `@@` header. Files with no header lines (pure add/delete) → one hunk starting at line 1.

- [ ] **Step 1:** From Task 1's report, use the EXACT `HunkData`/`ChangeData` field names react-diff-view expects. Write a failing unit test: feed a small `GitDiff` (1 header + a removed + an added + a context line) and assert one `HunkData` with three `ChangeData`s of the right types and correct old/new line numbers, plus correct `oldStart/newStart`. Add a second case: a pure-new file (all `added`, no header) → one hunk from line 1.
- [ ] **Step 2:** Implement `gitDiffToHunks`. Run; confirm pass. tsc clean for the file.
- [ ] **Step 3:** Commit: `feat(git-fe): gitDiffToHunks mapper (GitDiff → react-diff-view hunks)`.

---

### Task 3: Tree-sitter token bridge for react-diff-view

**Files:**
- Create: `web/src/features/git/lib/diff-highlight-shared.ts` (extract `reconstructContent` + parser-config resolution + line/token mapping out of `use-git-diff-highlight.ts` so both the old viewer and the new one share it; the old hook re-imports from here)
- Modify: `web/src/features/git/hooks/use-git-diff-highlight.ts` (import the extracted helpers; no behavior change)
- Create: `web/src/features/git/lib/render-tree-sitter-token.tsx` (`renderToken` impl + the per-line token resolution)
- Test: `web/src/__tests__/features/git/lib/render-tree-sitter-token.test.tsx`

**Interfaces produced:**
- `buildDiffTokens(diff: GitDiff): Promise<DiffTokenMap>` where `DiffTokenMap` keys tokens by `(side: 'old'|'new', lineNumber)` (reconstruct old/new content, run `tokenizeByLine` per side, map to lines). Reuse the existing config-resolution (`indexedDBParserCache` → `getDefaultParserWasmUrl` → `fetchHighlightQuery`) and `HighlightToken` (its `type` is already a `.token-*` class).
- `renderTreeSitterToken(tokensForLine: HighlightToken[]): (token, defaultRender, i) => ReactNode` (or the shape react-diff-view's `renderToken` expects, from Task 1) — ports `git-diff-line.tsx`'s `renderHighlightedContent` column-slice loop → `<span className={token.type}>…</span>`; returns `defaultRender`/plain text when `tokensForLine` is empty.

- [ ] **Step 1:** Extract the reusable internals from `use-git-diff-highlight.ts` into `diff-highlight-shared.ts` (`reconstructContent`, the parser-config resolution, the line↔token mapping). Re-point `use-git-diff-highlight.ts` to import them; run its existing tests (`./node_modules/.bin/vitest run src/__tests__/features/git` — the highlight-related ones) to confirm no behavior change.
- [ ] **Step 2:** Implement `buildDiffTokens(diff)` returning the `(side,lineNumber)`-keyed map. Write a unit test on a small TS diff asserting that a keyword line yields a token whose `type` is a `.token-*` class at the right (side,line) key. (Tree-sitter wasm may be unavailable in vitest — if so, assert it resolves to an empty map without throwing, and cover the column-slice logic of `renderTreeSitterToken` directly with a hand-built `HighlightToken[]` instead.)
- [ ] **Step 3:** Implement `renderTreeSitterToken` (port the column-slice → `.token-*` span loop; empty tokens → plain text). Unit-test it directly with a hand-built `HighlightToken[]` (e.g. one keyword token spanning cols 0–5 of `const x = 1`) → asserts a `<span class="token-keyword">const</span>` + plain remainder. This test does NOT need real tree-sitter.
- [ ] **Step 4:** Commit: `feat(git-fe): tree-sitter token bridge for react-diff-view (buildDiffTokens + renderToken)`.

---

### Task 4: `ReviewDiffView` — the unified review surface

**Files:**
- Create: `web/src/features/git/components/diff/review-diff-view.tsx`
- Modify: `web/src/features/git/types/git-diff-types.ts` (add `uncommitted: boolean` to `FileDiffSummary`)
- Test: `web/src/__tests__/features/git/components/diff/review-diff-view.test.tsx`

**Interfaces consumed:** `gitDiffToHunks` (T2), `buildDiffTokens`/`renderTreeSitterToken` (T3), the virtualization shell pattern from `git-diff-multi-file.tsx`, `react-diff-view` `<Diff>`/`<Hunk>`, `MultiFileDiff`.
**Interfaces produced:** `ReviewDiffView({ multiDiff })` — virtualized stack of per-file react-diff-view diffs; per-file header (path, +N/−M from the existing `fileSummaries` computation, an `uncommitted` pill when `summary.uncommitted`, a local-state "Viewed" toggle); a unified/split toggle (`viewMode`) in a header bar; image files → existing `ImageDiffViewer`.

- [ ] **Step 1:** Add `uncommitted: boolean` to `FileDiffSummary` and populate it from `file.uncommitted` wherever `fileSummaries` is computed (reuse that computation from `git-diff-multi-file.tsx`).
- [ ] **Step 2:** Write a render test: given a 2-file `MultiFileDiff` (one with `uncommitted:true`), assert both file headers render with their paths + the uncommitted pill on the right one, and that each file's diff body renders its changed lines (react-diff-view output). Mock `buildDiffTokens` to resolve empty (so the test doesn't need tree-sitter) and assert it still renders plain (graceful-degrade).
- [ ] **Step 3:** Implement `ReviewDiffView`: copy the virtualization shell from `git-diff-multi-file.tsx` (`useVirtualizer` over `multiDiff.files`, `scrollRef`, `estimateSize` from `file.lines.length`, absolute rows, `measureElement`, `contentVisibility:auto`, the `LARGE_DIFF_THRESHOLD` auto-collapse). Inside each row: the per-file header + (when expanded/viewed) a `<Diff viewType={viewMode==='split'?'split':'unified'} diffType={status} hunks={gitDiffToHunks(file)} renderToken={...}>{hunks=>hunks.map(h=><Hunk key=.. hunk={h}/>)}</Diff>`. Tokenize lazily: `buildDiffTokens(file)` memoized per fileKey, kicked off when the file expands/enters view; pass the resolved tokens into `renderToken` (resolve per change line by (side,lineNumber)). Image branch → `ImageDiffViewer`. Use `@/components/ui` + tokens; import `react-diff-view/style/index.css`.
- [ ] **Step 4:** Run the test; confirm pass. tsc clean for touched files (ignore pre-existing `otp-field.tsx`).
- [ ] **Step 5:** Commit: `feat(git-fe): ReviewDiffView — react-diff-view unified review surface`.

---

### Task 5: Scroll-to-file + rewire the review tab & sidebar; retire old renderer

**Files:**
- Modify: `web/src/features/workspace/stores/slices/branch-review-slice.ts` (add `activeFileKey`/`activeFileNonce` + `setBranchReviewActiveFile`)
- Modify: `web/src/features/git/components/diff/review-diff-view.tsx` (scroll-to-file effect)
- Modify: `web/src/features/git/components/review-diff-tab.tsx` (render `ReviewDiffView` instead of `MultiFileDiffViewer`)
- Modify: the sidebar file-click path (`git-panel.tsx` `onFileOpen`) → open the branch-review tab + `setBranchReviewActiveFile(fileKey)` + activeSubtab='diff'
- Delete: `web/src/features/git/components/diff/git-diff-multi-file.tsx` (after confirming only `review-diff-tab` imported it)
- Test: `web/src/__tests__/features/workspace/stores/branch-review-slice.test.ts` (extend: activeFileKey set + nonce increments on every call)

**Interfaces produced:** `setBranchReviewActiveFile(key: string|null)` increments `activeFileNonce` each call so re-clicking the same file re-scrolls.

- [ ] **Step 1:** Add `activeFileKey: string|null` (INITIAL null) + `activeFileNonce: number` (INITIAL 0) to `BranchReviewState`; action `setBranchReviewActiveFile(key)` sets the key and `activeFileNonce += 1`. Unit-test: two calls with the same key bump the nonce twice. Run; pass.
- [ ] **Step 2:** In `ReviewDiffView`, subscribe `activeFileKey`+`activeFileNonce` (via the workspace store; this component renders inside the branch-review pane's provider, so `useWorkspaceStoreContext` is correct here). In `useEffect([activeFileNonce])`, find the file index where `fileSummaries[i].key === activeFileKey`, force-expand it, and `virtualizer.scrollToIndex(index, {align:'start'})`.
- [ ] **Step 3:** Repoint `review-diff-tab.tsx`'s lazy import to `./diff/review-diff-view` and render `<ReviewDiffView multiDiff={diff}/>` (drop the `onClose` no-op). Keep the existing loading/error/empty branches.
- [ ] **Step 4:** Rewire the sidebar file click: in `git-panel.tsx`, change `onFileOpen` so clicking a changed file (a) opens the branch-review tab for the active ws via `openBranchReviewForActiveWorkspace()` (dedups on wsId) AND (b) `getOrCreateWorkspaceStore(wsId).getState().setBranchReviewActiveFile(fileKey)` + sets activeSubtab='diff'. This REPLACES the old `handleViewFileDiff` working-tree-diff routing (resolving Plan 2's I2: committed files now open in the unified review tab which has their diff). The `fileKey` must match the `FileDiffSummary.key`/`fileKeys` scheme the review diff uses (derive it the same way `git-diff-multi-file`/`ReviewDiffView` does).
- [ ] **Step 5:** Delete `git-diff-multi-file.tsx`; fix any imports. Run `cd web && ./node_modules/.bin/vitest run src/__tests__/features/git src/__tests__/features/workspace/stores/branch-review-slice.test.ts` — green. tsc no new errors.
- [ ] **Step 6:** Commit: `feat(git-fe): scroll-to-file review tab + sidebar opens unified review; retire old multi-file viewer`.

---

### Task 6: Live Tauri verification (Plan-3 gate)

Controller-run via Tauri MCP after Tasks 1–5.

- [ ] **Step 1:** With the app running (the dev stack is live; HMR picks up the changes — reload the webview if an error boundary latches), open the branch-review surface. Confirm the **Diff** tab renders the unified react-diff-view diff with **syntax-highlighted** lines (colors matching the editor), per-file headers, and unified/split toggle. Verify no console errors.
- [ ] **Step 2:** Click a changed file in the Git sidebar → confirm it opens/activates the review tab and **scrolls to that file**. Click a second file → confirm it scrolls (tab does not reopen).
- [ ] **Step 3:** Screenshot/DOM-verify; record findings in the ledger. Fix any runtime issues before declaring Plan 3 done. (Full blended/uncommitted styling depends on the rebuilt backend — deferred to the final pass; structure + highlighting + scroll are verifiable now against the current `getReview`.)

## Self-Review
- Spec coverage: react-diff-view render (T1) ✓; hunk mapping (T2) ✓; tree-sitter colors via tokenize/renderToken (T3) ✓; unified surface + per-file headers + uncommitted pill + viewed + split toggle (T4) ✓; scroll-to-file + dedup-safe nonce (T5) ✓; sidebar opens unified review (resolves Plan-2 I2) (T5) ✓; retire old renderer (T5) ✓; live verify (T6) ✓.
- Placeholder scan: none — the lazy-tokenize + graceful-degrade behaviors are specified, not deferred.
- Type consistency: `gitDiffToHunks(diff)`, `buildDiffTokens(diff)`, `renderTreeSitterToken`, `setBranchReviewActiveFile`, `FileDiffSummary.uncommitted` consistent across tasks.
- Risk: react-diff-view's exact `HunkData`/`ChangeData`/`renderToken` shapes are confirmed in T1 before T2/T3 depend on them.
