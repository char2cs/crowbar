# Branch Review shell: design

Visual reference (private mock, read the HTML for exact structure/copy):
`/private/tmp/claude-501/-Users-char2cs--crowbar-projects-cb81ea03-54b7-4093-8bdd-fdd6b183e91d-github-com-char2cs-crowbar-enhancement-ui-leakage-worktree/ddcae6fb-6b23-4ea7-bf2d-f4a777d6c438/scratchpad/review/project/{Main,Conversation}.dc.html`
(Main = sidebar navigator + Files changed; Conversation = description, threads, merge box, composer). Re-skin with the app's own tokens/components, not the mock's hexes.

## Intent
Make Branch Review a GitHub-PR-like surface for reviewing agent work, cheap on huge diffs (1M-line fixture). The diff engine (windowed patch fetch, outline geometry, `@pierre/diffs`) already exists and stays. Comments are the only channel to agents: they read them through the `wait_for_review_comments` MCP tool (separate lane, in flight). There is NO "send to agent" action, no turns, no commits/history UI.

## Anti-goals
- Nothing on the client may grow with total diff LINES. Nothing always-mounted may grow with changed FILES (the git panel is always mounted).
- No second UI library: `@/components/ui/*` (base-ui, Coss style via `components.json` `@coss` registry), phosphor icons, `@tanstack/react-virtual`, `idb`. No timers/polls/resync effects. No stubs wired to UI.

## Decisions (user-made)
1. **Viewed state lives in frontend IndexedDB**, per workspace, in the existing `branch-review` store (keyPath `wsId`, currently dead fields; reshape it to `{ wsId, viewed: Record<path, signature> }`, bump DB version only if required). The signature is the strongest cheap fingerprint of a file available from the files summary WITHOUT fetching patches; a file whose signature changed is NOT viewed any more.
2. **Merge lives at the end of the Conversation**, GitHub's merge-box position (above the comment composer). Slim card: strategy select + Merge button; when blocked show the reason (open threads, cannot merge locally, uncommitted/conflicts). Reuse `resolveBranchAction`, `mergeIntoParent`, `patchMergeStrategy`, `branchReview.mergeStrategy`. No checklist, no "send to agent".
3. Sidebar git panel = navigator only: branch card, "Reviewing" progress (n of N viewed + open-thread line linking to Conversation), folder tree. No History, no Commits/Turns anywhere.

## Main pane (inside `BranchReviewPane`, kind stays `'branchReview'`)
- Header: title (branch), state chip, branch → base. Tabs: **Conversation** · **Files changed (N)**. Persist the active view as optional `view?: 'conversation' | 'files'` on `BranchReviewContent` (layout row already round-trips).
- Files changed = existing `ReviewDiffTab` surface minus its own toolbar duplicates; per-file header gets a **Viewed** toggle; viewed files collapse to a header-only placeholder and are never fetched/materialised (evict if held).
- Conversation = description (read-only), thread timeline (all threads, anchored threads link to their line), merge box, composer. Must render even when the file list is empty (`ReviewDiffTab` early-returns today).

## General (un-anchored) comments
The model has no un-anchored comment. The Conversation composer posts a thread with `filePath: ''`, `lineNumber/startLine/endLine: 0`, side `'new'`. Backend must accept it end-to-end (domain validation, HTTP open route, DTO, list/render for agents so `list_review_threads` and `wait_for_review_comments` surface it). Web: diff annotations and per-file counts ignore threads with empty `filePath`; Conversation shows all.

## Lanes (disjoint files)
- **A. Viewed state + windowing (web):** IDB store reshape, workspace slice (`viewedFiles`, lazy load, `setFilesViewed`), `features/git/lib/review-viewed.ts` (signature + `isViewed`), skip viewed paths in `review-code-view.tsx` placeholders and `use-review-patch-window.ts` fetch/evict; guard `revealFile/Line/Thread/expandTruncated/retryPatch` (reveal of a viewed file un-collapses it only via an explicit user action). Does NOT touch sidebar/pane shell files.
- **B. Sidebar navigator (web), after A:** `changed-files-tree.tsx` (compact single-child folders, viewed checkbox per file + tri-state per folder, per-file open-thread badge, stays virtualized), `git-panel.tsx` (Reviewing card, drop History and the redundant "Review this branch" row), `build-git-folder-tree.ts` helpers.
- **C. Main-pane shell (web):** `branch-review-pane.tsx`, `review-diff-tab.tsx` header, new view components under `features/git/components/` (conversation view, merge box, general composer, tabs wiring), extract thread write handlers from `use-review-annotations.tsx` into a reusable hook, `pane-content.ts` optional `view`.
- **D. Backend un-anchored threads (Go):** per the section above, black-box `TestRegression_*`.
- **MCP (running):** `wait_for_review_comments`.

## Verification
Targeted vitest/go tests per lane (never a full vitest run). Final: lint/knip/tsc/prettier, then live in the dev desktop app against the 1M-line fixture baseline (`perf-baselines.md`): viewing/collapsing a file must not fetch its patch; sidebar tree mount must stay O(files) from the summary only.
