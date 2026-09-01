# Sidebar Restyle Recovery — Batch 2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the nine items of follow-up feedback the user sent after the first recovery pass closed, per `docs/superpowers/specs/2026-08-31-sidebar-restyle-recovery-batch2.md`.

**Architecture:** Six small-to-medium, independently-shippable UI fixes (Tasks 1-6), then a two-part backend-then-frontend fix (Tasks 7-8) for the workspace/chat row-unification gap, which is the one item comparable in size to the entire first recovery pass. One item (project-selector placement) is explicitly NOT a task here — flagged in the spec §3.1 for the user's confirmation, left unchanged.

**Tech Stack:** React + TypeScript (web/), Zustand stores, Tailwind, Go backend (api/), Tauri desktop shell.

**Spec:** `docs/superpowers/specs/2026-08-31-sidebar-restyle-recovery-batch2.md`.

## Global Constraints

- Same conventions as the first recovery plan: kebab-case component files, tests mirror `web/src/__tests__/` with `@/` imports, narrow store selectors only, `.getState()` confined to handlers/effects, stores never import from `components/`.
- "Big bang, no legacy" applies ONLY to the surfaces these 9 tasks touch. Do not re-open, re-review, or modify anything the first recovery pass (Tasks 1-8) already fixed and reviewed clean, and do not touch the project-selector placement (spec §3.1 — explicitly flagged, not actioned).
- Task 7/8: the three protected/locked-branch rows (`develop`, `main`, project home) MUST continue to render as chat-less `branch`-kind rows exactly as today — they are not part of this fix, and any change that alters their behavior is a regression, not a feature.
- Every task needs a regression test that fails on pre-fix code.

---

## Task 1: Fix the pane's chat head — pill to flat underline

**Files:**
- Modify: `web/src/features/tabs/components/chat-head.tsx`
- Test: `web/src/__tests__/features/tabs/components/chat-head.test.tsx` (create if it doesn't exist at that path; check first)

**Interfaces:** none new — pure styling change to an existing component's className.

- [ ] **Step 1: Read the design's ground truth**

Read `Main.dc.html` directly (`/private/tmp/claude-501/-Users-char2cs--crowbar-projects-cb81ea03-54b7-4093-8bdd-fdd6b183e91d-github-com-char2cs-crowbar-enhancement-unify-sidebar-worktree/95884450-e2c6-4cc1-97c2-02800f6284d1/scratchpad/Main.dc.html` — an earlier research pass found the `.hitem`/`.hitem.is-on` rule there: no background at rest, no border-radius, an `::after` 2px bottom bar in `--primary` on selection. Confirm this reading yourself before implementing. Also read `web/src/features/tabs/components/tab-bar-item.tsx` (the file-tab equivalent) to see how it implements its own selected-state underline, so the chat head matches it exactly rather than inventing a second implementation of the same visual rule.

- [ ] **Step 2: Write the failing test**

Assert the chat-head button's className does NOT contain `rounded-full`, `bg-background`, or `shadow-sm`, and DOES carry whatever underline/active-state class `tab-bar-item.tsx` uses for its own `is-on` treatment, when the chat head is the active surface.

- [ ] **Step 3: Implement, matching `tab-bar-item.tsx`'s pattern**

- [ ] **Step 4: Run the test, confirm PASS; run the full `web/src/__tests__/features/tabs/` directory to confirm no regression.**

- [ ] **Step 5: Commit**

```bash
git commit -m "fix(pane): chat head is a flat underline tab, not a rounded pill"
```

---

## Task 2: Fix the file-explorer card head's height (48px measured, 28px specified)

**Files:**
- Modify: whichever file defines the Files/Git underline-tabs head — locate it (likely `web/src/components/layout/sidebar-carousel.tsx` or a component it renders; a prior recovery task already confirmed this file implements the card's carousel head) and its Tailwind classes.
- Test: the corresponding file in `web/src/__tests__/components/layout/`.

**Interfaces:** none new.

- [ ] **Step 1: Locate the exact class producing the conflict**

A prior live measurement found the tab buttons carry both `h-7` (28px, correct) and `sm:h-8` (32px, winning the cascade) — grep for `sm:h-8` near `h-7` in the file-explorer card's head component. Also measure the head container's own padding (a prior pass measured 48px total against 40px inner content, so ~4px top+bottom padding is adding on top of the tab height itself) and confirm against the spec's flat 28px target for the WHOLE head, not just the buttons.

- [ ] **Step 2: Write a failing test**

Assert the head element's rendered height (or its Tailwind height class, whichever this codebase's test conventions prefer for pixel-value assertions — check a sibling test file for the pattern) is 28px, not 48px/32px.

- [ ] **Step 3: Remove the conflicting `sm:h-8` (or whatever the actual conflicting utility is) and correct any excess head padding so the whole head lands at 28px.**

- [ ] **Step 4: Run the test, confirm PASS; run the full file-explorer-card test directory.**

- [ ] **Step 5: Commit**

```bash
git commit -m "fix(sidebar): file-explorer card head is 28px, not 48px — sm:h-8/h-7 cascade fix"
```

---

## Task 3: Remove "New Chat" from the New Tab empty stage

**Files:**
- Modify: `web/src/features/panes/components/new-tab-view.tsx`
- Test: `web/src/__tests__/features/panes/components/new-tab-view.test.tsx` (check exact path first)

**Interfaces:** consumes nothing new. Removes a call site of `AGENT_NEW_CHAT`/`createNewChat` — check whether either becomes dead code elsewhere as a result (unlikely; chats are still created from the sidebar's `+`) before deleting more than this one call site.

- [ ] **Step 1: Read `new-tab-view.tsx` in full**, including whatever renders "below the divider" (a capped recent-chat-history list, per this repo's own memory `project_new_tab_empty_stage.md`). Confirm: does clicking an entry in that list currently open the chat INTO this pane's editor view (wrong, per spec §7.2 — a chat is never something the editor view opens), or does it navigate/focus that chat's own pane (correct)? Fix this too if it's wrong — it's the same underlying defect as the "New Chat" action row.

- [ ] **Step 2: Write a failing test** asserting no "New Chat" action row renders in the New Tab stage, and (if Step 1 found the recent-history click behavior wrong) that clicking a recent-chat entry does not load content into the current pane's chat surface.

- [ ] **Step 3: Delete the "New Chat" `ActionRow` and its `⌘N`/`AGENT_NEW_CHAT` binding from this view.** Fix the recent-history click handler if Step 1 found it wrong.

- [ ] **Step 4: Run the test, confirm PASS; run the full `web/src/__tests__/features/panes/` directory.**

- [ ] **Step 5: Commit**

```bash
git commit -m "fix(panes): New Tab stage offers files/terminal/branch-review only, never chat"
```

---

## Task 4: Restore double-click-to-rename

**Files:**
- Modify: `web/src/components/sidebar/sidebar-row.tsx`, `web/src/components/layout/sidebar-project-header.tsx` (or wherever the space header row lives post-Task-5-of-the-first-recovery-pass — confirm current location)
- Test: `web/src/__tests__/components/sidebar/sidebar-row.test.tsx`, corresponding space-header test file.

**Interfaces:** consumes the EXISTING `RenameDialog`/`performRenameRow` path the right-click menu's "Rename" item already reaches (`web/src/components/sidebar/lib/row-actions.ts:181`) and the state that opens it (`renamingRowId` in `sidebar-tree-chrome.tsx`, or wherever Task 5 of the first recovery pass relocated adjacent state — check current wiring first, don't assume the exact prop path from an earlier point-in-time read).

- [ ] **Step 1: Trace the exact current wiring** from `row-context-menu.tsx`'s `onRename` callback through to `RenameDialog` opening, so the double-click handler reuses the identical path rather than duplicating dialog logic.

- [ ] **Step 2: Write a failing test**: double-clicking a row's label opens the same rename dialog / triggers the same `onRename`-equivalent callback the context menu already uses.

- [ ] **Step 3: Add an `onDoubleClick` handler to `sidebar-row.tsx`'s label span and to the space header's equivalent**, calling the same trigger the context menu's Rename item calls. Make sure it doesn't fire the row's own `onClick`/open behavior at the same time (a double-click contains two single clicks — check whether `onClick` firing alongside `onDoubleClick` causes an unwanted navigation, and guard against it if so).

- [ ] **Step 4: Run tests, confirm PASS; run the full sidebar test directory.**

- [ ] **Step 5: Commit**

```bash
git commit -m "fix(sidebar): restore double-click-to-rename, deleted during the tree retirement"
```

---

## Task 5: Rebuild icon personalization

**Files:**
- Modify or create: a new thin wrapper around the still-present `web/src/components/layout/icon-popover.tsx` primitive, wired to project rows and repo-home rows.
- Modify: `web/src/components/layout/repo-icon-mark.tsx` (its own comment already documents it used to route through this component and no longer does — this is the reconnection point).
- Test: new test file(s) under `web/src/__tests__/components/layout/`.

**Interfaces:** consumes `icon-popover.tsx`'s current API (read it first — it may not match the deleted `project-icon-popover.tsx`/`repo-icon-popover.tsx`'s old call shape; build a new, thin adapter rather than trying to resurrect the deleted files verbatim, per the spec's "no legacy" directive).

- [ ] **Step 1: Read `icon-popover.tsx` in full** to understand its current, real API surface (props, what it renders, how a caller supplies the set of choosable icons and receives a selection). Read `git show cf422bc5:web/src/components/layout/project-icon-popover.tsx` (or the equivalent old path) ONLY as reference for what the feature used to do conceptually — do not copy its code verbatim; it was written against APIs that may no longer exist.

- [ ] **Step 2: Write a failing test**: clicking a project row's (or repo-home row's) leading glyph opens an icon picker; selecting an icon persists it (check what persistence mechanism existed before — likely a field on the Project/Repo domain object reachable via an existing API call; if no such field/endpoint currently exists server-side, STOP and report this as a scope finding rather than inventing a new backend field — this task assumes the persistence layer already exists from the pre-restyle feature and just needs its frontend trigger rebuilt, but verify that assumption first).

- [ ] **Step 3: Build the new, thin wrapper and wire it to project/repo-home rows' glyph click**, reusing `icon-popover.tsx` as-is.

- [ ] **Step 4: Run tests, confirm PASS.**

- [ ] **Step 5: Commit**

```bash
git commit -m "feat(sidebar): rebuild icon personalization on the still-present icon-popover primitive"
```

---

## Task 6: Add per-branch lock choice to the import flow

**Files:**
- Modify: `web/src/components/projects/repo-import-dialog.tsx`
- Test: corresponding test file under `web/src/__tests__/`.

**Interfaces:** consumes `performSetWorkspaceLock`/`setWorkspaceLock` (already exist, currently only called post-hoc from the row context menu) — wire it into the import flow's own submit path instead of (or in addition to) the post-import menu toggle. Consumes `importBranches` (`web/src/lib/api.ts`, already exists) — check its current signature for whether it already accepts a per-branch lock flag or needs extending.

- [ ] **Step 1: Read `repo-import-dialog.tsx`'s current form in full** — what fields does each branch row in the import list already carry? Determine the smallest addition: a per-row lock checkbox/toggle.

- [ ] **Step 2: Write a failing test**: importing branches with a lock choice set results in the created workspace(s) carrying the correct locked state, without a separate post-import action.

- [ ] **Step 3: Implement** — add the per-branch lock control to the dialog; wire its result either through an extended `importBranches` call (if the backend route accepts a lock field) or as a follow-up `performSetWorkspaceLock` call fired immediately after each import succeeds (if not — note in your report which approach you took and why, since extending the backend route may be out of this task's frontend-only scope; if the backend genuinely needs a new field to do this atomically and cleanly, treat that as its own small backend task and report it rather than guessing at Go changes under this task's brief).

- [ ] **Step 4: Run tests, confirm PASS.**

- [ ] **Step 5: Commit**

```bash
git commit -m "feat(sidebar): choose branch lock state at import time"
```

---

## Task 7: Backend — finish Stage 5 (atomic workspace+chat creation)

**Files:**
- Modify: `api/internal/api/v0/endpoints/chat/handlers/chats.go` (the `POST /repos/:rid/chats` handler — extend its request body with `ownWorktree: boolean`)
- Modify: whatever usecase this handler calls into (`api/internal/app/usecases/chat/chat.go` or similar — read the handler first to find it)
- Read (pattern to reuse, do not copy blindly): `api/internal/app/usecases/chat/promote.go` — it already resolves a fork parent and cuts a worktree for the *existing-chat* case; this task needs the same worktree-minting logic composed with *creating a new chat row* instead of updating one.
- Test: `api/internal/app/usecases/chat/chat_test.go` or a new file alongside it, mirroring `promote_test.go`'s coverage shape; a black-box regression test in `api/tests/` per this repo's own convention (`TestRegression_*`, see `feedback_blackbox_regression_tests` memory).

**Interfaces:**
- Produces: `POST /repos/:rid/chats {parentId, providerId, ownWorktree: true}` (no `workspaceId`) → creates a new workspace (worktree provisioned, branch cut from the resolved fork parent) AND a new chat row with `WorkspaceID` set to it, in one request, one response.
- Must NOT change behavior when `ownWorktree` is false/absent, or when `workspaceId` is supplied instead (the existing attach-to-existing-workspace path stays exactly as it is).

- [ ] **Step 1: Read `promote.go` in full.** Identify exactly which internal functions/usecase calls it uses to resolve the fork parent and provision the worktree.

- [ ] **Step 2: Read the current `POST /repos/:rid/chats` handler and its usecase in full.** Identify where a new chat row is currently constructed, and what would need to change to also provision a workspace first when `ownWorktree` is requested.

- [ ] **Step 3: Write the failing test(s) first**: a request with `ownWorktree: true` and no `workspaceId` should create both a workspace (with a real worktree on disk, cut from the correct fork parent per the model spec §3.2's cwd-walk rule) and a chat row whose `WorkspaceID` points at it, in one call, with no intermediate chat-less-workspace state ever observable. Confirm these fail against current code first.

- [ ] **Step 4: Implement**, composing `promote.go`'s worktree-provisioning pattern with new-chat-row creation instead of existing-chat-row attachment.

- [ ] **Step 5: Run the full Go test suite for the touched packages**; confirm no regression in the existing attach-to-existing-workspace path or in `promote.go`'s own tests (shared internal functions must not have been broken by extraction/reuse).

- [ ] **Step 6: Commit**

```bash
git commit -m "feat(chat): CreateChild via POST /chats — finish Stage 5's chat+workspace atomicity"
```

---

## Task 8: Frontend — consume atomic workspace+chat creation, verify one-row rendering

**Files:**
- Modify: `web/src/components/layout/space-content-actions.ts` (`handleCreate`'s `kind === 'workspace'` branch)
- Modify (only if Step 3 below finds it necessary): `web/src/components/layout/workspace-tree-utils.ts` (`buildSidebarTree`)
- Test: `web/src/__tests__/components/layout/space-content-actions.test.ts`, and a `sidebar-tree`/`rows-from-repo` test covering the new one-row rendering.

**Interfaces:** consumes Task 7's extended `POST /repos/:rid/chats {ownWorktree: true}` — this task cannot start until Task 7 is merged and its endpoint is live.

- [ ] **Step 1: Add the frontend API wrapper** for the extended endpoint (in `web/src/features/agent/api/agent-api.ts` or `web/src/lib/api.ts`, matching whichever already hosts chat-creation calls), accepting `ownWorktree`.

- [ ] **Step 2: Write the failing test**: creating a workspace via `handleCreate('workspace', ...)` calls the new endpoint with `ownWorktree: true`, not `postWorkspace()`.

- [ ] **Step 3: Swap `handleCreate`'s `kind === 'workspace'` branch to call the new endpoint.** Then verify — do not assume — that a freshly created workspace renders as ONE row (a `chat`-kind row that also owns a workspace), not a bare `branch`-kind row plus a separate child chat. If `buildSidebarTree`/`rows-from-repo.ts` still misroutes a workspace-owning chat node into a branch-only shape, fix the classification there; if it already renders correctly once the backend stops creating a chat-less workspace (likely, since the chat-node branch already produces a full row), just add the regression test proving it.

- [ ] **Step 4: Confirm the three protected/locked-branch rows (`develop`, `main`, project home) are unaffected** — they don't go through `handleCreate`'s workspace-creation path at all, but add an explicit test asserting they still render as chat-less `branch` rows, since this is the one case this migration must never touch.

- [ ] **Step 5: Confirm no Recents/reorder-animation regression** (spec §4's carried-through constraint) — a workspace-owning row now being `chat`-kind instead of `branch`-kind changes what data shape flows into Recents; spot-check that Recents entries for a newly-created workspace still animate/reorder correctly.

- [ ] **Step 6: Run the full web gate**: `bun tsc --noEmit`, `bun vitest run`, lint.

- [ ] **Step 7: Commit**

```bash
git commit -m "fix(sidebar): create-workspace renders as one row, not a branch+chat split"
```

---

## Task 9: The pane's identity row must share one background with its content, not a distinct header band

**Inserted mid-execution** after the user sent follow-up feedback (Image #10) on Task 1's own result. Task 1 fixed what's INSIDE the row (tab styling, pill → underline). This fixes the row's CONTAINER.

**Files:**
- Modify: `web/src/features/panes/components/pane-container.tsx` — move `paneContentStyle` (rounding, border, shadow, the 4px gutter from `web/src/features/panes/utils/pane-border.ts`'s `buildPaneContentStyle`) so it wraps the WHOLE pane (the identity row + the content together), not just `data-pane-content` alone. Add a shared `bg-pane-background` (or equivalent) at that same outer level so the row no longer shows the translucent `--chrome-bg` body tint through it.
- Modify: `web/src/features/tabs/components/tab-bar.tsx` — remove whatever background the row inherits from being unstyled against the window backdrop (confirm after Step 1 below exactly what needs to change here, if anything, once the outer wrapper is fixed).
- Test: `web/src/__tests__/features/panes/components/pane-container.test.tsx`.

**Interfaces:** none new — this changes WHERE an existing style object (`buildPaneContentStyle`'s return value) is applied, not its logic. Do not change `pane-border.ts`/`buildPaneContentStyle` itself unless Step 1's investigation finds a genuine reason to.

**Root cause, already confirmed by research:** `data-pane-container` (the outer div `pane-container.tsx:613`) paints no background at all, so the identity row shows the page body's translucent `--chrome-bg` vibrancy tint through it. `data-pane-content` (`pane-container.tsx:661`) paints an explicit opaque `bg-pane-background` fill AND gets the only rounded-top corners, via `style={paneContentStyle}`. That mismatch — different fill, only the lower box rounded — is exactly the "distinct gray header block over a white rounded content pane" the user is describing. The design canvas's ground truth (`Main.dc.html:675-688`, `.app .pane { background: var(--pane); border: 2px solid transparent; box-shadow: ...; }` / `.app .pnhead { ... no background, no border-bottom ... }`) is unambiguous: ONE shared background across the row and the content, with rounding/border/shadow on the outer pane box, not the inner content div alone. The only hairline anywhere in the row is a 1px *vertical* divider between the chat button and the tab strip (`.app .hdiv`) — never a horizontal seam under the whole row.

- [ ] **Step 1: Read `pane-container.tsx` and `pane-border.ts`/`buildPaneContentStyle` in full.** Confirm precisely which element `paneContentStyle` currently attaches to, and trace how the 4px gutter, the corner-flattening-at-window-edges logic, and the active-pane focus ring currently behave — these were all deliberately built (first recovery plan, Task 6, and the original design work) to avoid a specific measured WKWebView performance regression (rounded+shadowed corners composited against the window's own vibrant edge: 8ms → 106ms frames). Moving where this style attaches must preserve that exact conditional behavior (flatten at a real window edge, keep it elsewhere) — it must not become "always round" or "never round."

- [ ] **Step 2: Write the failing test(s) first.** Assert: (a) the identity row and the content area resolve to the SAME background color/token (not two different fills); (b) the pane's rounded corners, border, and shadow now visually enclose the row too, not just the content div, while still correctly flattening at a real window edge exactly as before (reuse/extend whatever test already covers Task 6 of the first recovery plan's gutter fix, don't write a parallel, disconnected assertion).

- [ ] **Step 3: Implement** — move the `paneContentStyle` application (and the background fill) to wrap the whole pane. Adjust `tab-bar.tsx`'s own row styling only as needed once the outer wrapper is fixed (it may need nothing beyond what already exists, since it currently carries no background class of its own — confirm before adding anything).

- [ ] **Step 4: Run the tests, confirm PASS.** Run the full `web/src/__tests__/features/panes/` directory to confirm no regression to the first recovery plan's gutter/border-flattening work.

- [ ] **Step 5: Live-verify via the Tauri MCP** (load `mcp__tauri__*` via `ToolSearch` if deferred; confirm you're looking at this worktree's instance via `ipc_get_backend_state` before trusting anything you see) — screenshot a pane and confirm the row and content now read as one continuous surface with rounded corners enclosing both, not a two-tone header-over-content split. Also confirm a pane that sits against a real window edge still correctly flattens its corners there (don't just check the common case).

- [ ] **Step 6: Commit**

```bash
git commit -m "fix(pane): identity row shares one background/rounding with its content, not a header band"
```

---

## Task 10: Relocate the project selector to the sidebar's own footer

**Inserted mid-execution.** User's explicit direction, overriding this recovery's own §3.1 flagged-not-acted-on placement AND the closed design spec's §4.1 window-chrome placement: *"It should be the last element on the sidebar, right down the file explorer, but not inside it, just outside, at the end of the sidebar."* This is a real, deliberate design override — not a bug fix. Implement it as stated; do not re-litigate window-chrome placement's original justification, that discussion already happened.

**Files:**
- Modify: `web/src/components/layout/sidebar-project-header.tsx` — remove the `marks`/`add-project-mark` cluster from the window-chrome row (keep the toggle and the back/forward/settings cluster; rebalance the row's layout once the marks are gone — likely reverts closer to its pre-Task-5-of-the-first-recovery-pass shape, a bare `flex-1` spacer).
- Modify: `web/src/components/layout/ide-shell.tsx` (or wherever the first recovery pass's Task 5 lifted `importProjectOpen`/`ImportProjectModal` state to — confirm current location) — the lifted state stays, but what renders it moves.
- Create or modify: a new sidebar-footer element, rendered AFTER (below) the floating file-explorer card, as the last element in the sidebar's own DOM/visual order — not inside the card, a sibling after it. Find the component that currently renders the sidebar's overall vertical stack (rail → spaces scroller → floating card) to know where to add this.
- Test: corresponding files under `web/src/__tests__/components/layout/`.

**Interfaces:** reuses `ImportProjectModal`/`importProjectAndSync` (unchanged, already correctly wired by the first recovery pass) and the space-mark rendering logic already built (`data-testid=space-mark`/`add-project-mark`, icon-only, current-vs-muted opacity) — this task moves WHERE that cluster renders, not what it looks like or does.

- [ ] **Step 1: Find the component that lays out the sidebar's full vertical stack** (rail top-to-bottom) to identify exactly where a new "sidebar footer" element belongs, sitting after the floating file-explorer card in DOM order.

- [ ] **Step 2: Write the failing test**: the project-marks cluster no longer renders inside the window-chrome's first-44px row; it renders as the last element in the sidebar, positioned after the file-explorer card, not inside it.

- [ ] **Step 3: Implement** — extract the marks-cluster JSX from `sidebar-project-header.tsx` into wherever it now belongs (a new small component if that's cleanest, or inline at the new call site — match this codebase's existing granularity for similar small clusters), remove it from the window-chrome row, rebalance that row's layout.

- [ ] **Step 4: Run tests, confirm PASS.** Run the full `web/src/__tests__/components/layout/` directory.

- [ ] **Step 5: Live-verify via Tauri MCP** — screenshot the full sidebar top-to-bottom, confirm the marks cluster now sits below/after the file-explorer card as the sidebar's last element, and confirm the window-chrome row still looks balanced without it (no leftover dead space that reads as a mistake).

- [ ] **Step 6: Commit**

```bash
git commit -m "fix(sidebar): relocate the project selector to the sidebar's own footer, below the file-explorer card"
```

---

## Task 11: Rebuild real inline-edit rename (Task 4's modal was the wrong interaction)

**Inserted mid-execution, supersedes part of Task 4.** User's direct correction: *"Double clicking to rename isn't working as in develop. It should work exactly like before. This new modal is a shortcut that isn't sticking in."*

**Confirmed against `develop` directly (not assumed):** every pre-restyle renameable row (`workspace-tree-item.tsx`, `project-home-row.tsx`, `repo-section.tsx`, all on `develop`) used `WorkspaceInlineInput` — a real inline `<input>` that REPLACES the row's label in place on double-click, not a modal dialog. `onConfirm`/`onCancel` wire directly to the row's rename call (`renameProject`/`renameRepo`/branch rename); Enter confirms, Escape cancels, blur confirms unless already handled by Enter/Escape, and (for branch rows specifically) a `resolveExisting`/`onOpenExisting` pair suppresses a colliding rename and offers a hint to the row that already has that name. `workspace-inline-input.tsx` was deleted in the SAME tree-retirement commit (`f119a402`) that deleted the icon popovers — it does not exist anywhere in the current tree; this needs a fresh rebuild (per "no legacy"), using `develop`'s version as a behavioral reference only, not code to port verbatim.

**Task 4 (commit `d02a47f6`, already merged) built double-click to open the EXISTING right-click-menu's modal `RenameDialog` instead.** That was the planning brief's own mistake — it assumed reusing the current right-click path was correct without checking what the original double-click interaction actually looked like. This task corrects it. **Leave the right-click menu's "Rename" item alone** (it can keep opening the modal — the user's complaint is specifically about double-click, not about removing the modal entirely); this task changes ONLY what double-click does.

**Files:**
- Create: `web/src/components/sidebar/inline-rename-input.tsx` (kebab-case, fresh build referencing `develop`'s `workspace-inline-input.tsx` for behavior — focus+select on mount, Enter/Escape/blur handling, optional collision hint for branch names).
- Modify: `web/src/components/sidebar/sidebar-row.tsx`, `web/src/components/sidebar/space-header.tsx` — swap the label span for `InlineRenameInput` in place when in rename mode, driven by local/lifted state (owned by the parent, matching `develop`'s "owned by the parent" pattern), rather than opening `RenameDialog`.
- Modify: `web/src/components/layout/sidebar-tree-chrome.tsx` — the double-click handler currently opens `RenameDialog` (Task 4's `renamingRowId`/`setRenamingRowId`); change it to enter inline-rename mode on the target row instead.
- Test: update the tests Task 4 added (`sidebar-tree-chrome.test.tsx`, `sidebar-row.test.tsx`, `space-header.test.tsx`) to assert inline-edit behavior instead of modal-opening behavior.

**Interfaces:** consumes `performRenameRow`/`performRenameProject` (already correct, from Task 4 — the ACTION being fired doesn't change, only how the user triggers text entry). Does NOT touch `RenameDialog`, `row-context-menu.tsx`'s "Rename" item, or `performRenameRow`/`performRenameProject` themselves.

- [ ] **Step 1: Read `develop`'s `workspace-inline-input.tsx`, `workspace-tree-item.tsx`, `project-home-row.tsx`, `repo-section.tsx` in full** (`git show develop:<path>`) to fully understand the exact behavior being restored, including the collision-hint mechanic for branch rows.

- [ ] **Step 2: Write the failing test(s) first**: double-clicking a row's label replaces it with a real, focused, pre-selected `<input>` in place (not a dialog); typing and pressing Enter calls the same rename action Task 4 already wired; Escape cancels with no call; blur behaves like Enter unless Escape already fired.

- [ ] **Step 3: Build `InlineRenameInput`** fresh, matching `develop`'s behavior. Wire it into `sidebar-row.tsx`/`space-header.tsx` and `sidebar-tree-chrome.tsx`'s double-click handler, replacing the modal trigger.

- [ ] **Step 4: Run tests, confirm PASS.** Run the full sidebar test directory.

- [ ] **Step 5: Live-verify via Tauri MCP** — double-click a row, confirm it becomes a real editable input in place (not a popup), typing works, Enter commits, Escape cancels.

- [ ] **Step 6: Commit**

```bash
git commit -m "fix(sidebar): double-click-rename edits inline, matching develop, not a modal"
```

---

## Task 12: Fix Recents' blank untitled-chat label, then live-verify every batch-2 task that shipped without a Tauri check

**Inserted mid-execution.** User spotted a live bug (a blank pill in Recents, icon only, no text — Image #11) and asked directly whether every task in this plan was actually verified via the Tauri MCP. Honest answer: not all of them were. This task closes both gaps in one pass.

**Part A — the blank-label bug, already root-caused:**

`web/src/components/sidebar/recents-band.tsx:291` — `RecentsMemberRow` builds its row as `label: chat.title`, with no fallback. `web/src/components/sidebar/lib/rows-from-repo.ts` (the tree's equivalent) correctly does `node.chat.title || UNTITLED_CHAT_LABEL` (imported from `web/src/features/agent/lib/chat-label.ts`) AND sets `labelProvisional: !node.chat.title` for the italic treatment. Recents currently does neither — an untitled chat that's live/working/dormant in Recents renders as a bare pill with no visible label at all, not even a placeholder.

**Files:**
- Modify: `web/src/components/sidebar/recents-band.tsx` — import `UNTITLED_CHAT_LABEL` from `web/src/features/agent/lib/chat-label.ts`, apply the same `chat.title || UNTITLED_CHAT_LABEL` fallback and `labelProvisional: !chat.title` that the tree row already uses, so a Recents entry for an untitled chat matches the tree's own treatment exactly.
- Test: `web/src/__tests__/components/sidebar/recents-band.test.tsx`.

**Part B — live-verify what wasn't:**

Per the ledger, these tasks landed on unit-test evidence only, with no confirmed Tauri MCP live check: Task 1 (chat-head pill→underline), Task 3 (New Tab "New Chat" removal), the original Task 4 rename attempt (superseded by Task 11, but the SURFACE it touches — the right-click-menu path — was never itself live-checked), and Task 6 (lock-at-import, explicitly disclosed as mocked-API-only). Drive the live app (Tauri MCP) through each of these specific behaviors and confirm they actually work as shipped, not just as unit-tested. This is not a re-implementation of any of them — only a live confirmation pass, filing any real bug found as its own small, TDD'd fix rather than guessing.

- [ ] **Step 1: Write the failing test for Part A** — a Recents entry for a chat with an empty/falsy `title` renders `UNTITLED_CHAT_LABEL`, italic, exactly matching the tree row's fixture/assertion shape in `sidebar-row.test.tsx` or `rows-from-repo.test.ts` if one exists to mirror.

- [ ] **Step 2: Implement the fallback in `recents-band.tsx`.** Run the test, confirm PASS, run the full `web/src/__tests__/components/sidebar/` directory.

- [ ] **Step 3: Live-verify Part A** via Tauri MCP — get an untitled chat into a Recents-visible state (open it into a pane, or leave it working/dormant per whichever is easiest to reach), screenshot, confirm the label now reads "Untitled chat" in italic, not blank.

- [ ] **Step 4: Live-verify chat-head (Task 1)** — open a pane, confirm the flat-underline treatment renders correctly live (not just via the unit test's class assertions), matching the file tabs beside it.

- [ ] **Step 5: Live-verify New Tab "New Chat" removal (Task 3)** — open a pane's New Tab stage live, confirm only New Terminal/New File show, and confirm the recent-chat-history list's click behavior is what the code review found (opens the chat's own pane, doesn't load into the current editor view).

- [ ] **Step 6: Live-verify rename's right-click path (Task 4's surviving surface)** — right-click a row, click "Rename," confirm the modal opens and a rename round-trips against the real backend. (Double-click's own inline path was already live-verified by Task 11 — this step is specifically the right-click/modal path, which never got its own live check.)

- [ ] **Step 7: Live-verify lock-at-import (Task 6)** — import a branch with the lock choice set, confirm live that the resulting workspace is actually locked (check its row's lock indicator / attempt an action that a locked workspace should refuse), against the real backend, not mocks.

- [ ] **Step 8: For anything found broken in Steps 4-7**, file it as its own small, TDD'd fix (failing test first, real fix, commit) rather than batching unrelated fixes into one commit — same pattern as every other task in this plan.

- [ ] **Step 9: Run the full web gate** (`bun tsc --noEmit`, `bun vitest run`, lint) and report a final live-verification status for every item in Part B, ticked or flagged.

- [ ] **Step 10: Commit** (Part A's fix as its own commit; each Part B fix, if any, as its own commit).

---

## Task 13: chat-head's title fallback drops empty strings, and Recents may be entirely unreachable for chats under an unvisited workspace

**Inserted mid-execution**, two more live findings from the user and the controller's own follow-up investigation.

### Part A — `chat-head.tsx`'s title fallback, already root-caused

`web/src/features/tabs/components/chat-head.tsx:27`:
```ts
const title = useWorkspaceStoreContext(
  (s) => s.agentChats.chats.find((c) => c.id === chatId)?.title ?? 'Chat',
)
```
`??` (nullish coalescing) only replaces `null`/`undefined` — NOT an empty string, which is exactly what an untitled chat's `title` field actually is. Live-confirmed via Tauri MCP DOM snapshot: the pane's own `[data-testid=chat-head]` label span renders completely empty for an untitled open chat — this is a DIFFERENT surface than Task 12's Recents fix (`recents-band.tsx`), a separate instance of the same bug class, introduced independently by Task 1 of this plan. Swept the whole `web/src` tree for the same `?? '...'`-on-a-title pattern — no other UI-display instance found; this is isolated to `chat-head.tsx`.

**Fix:** import `UNTITLED_CHAT_LABEL` from `@/features/agent/lib/chat-label` (the same source `rows-from-repo.ts` and the now-fixed `recents-band.tsx` use) and change `?? 'Chat'` to `|| UNTITLED_CHAT_LABEL`, matching both surfaces exactly — same fallback text everywhere a chat's name can be blank, not a third, different placeholder string.

**Files:** `web/src/features/tabs/components/chat-head.tsx`. Test: `web/src/__tests__/features/tabs/components/chat-head.test.tsx`.

### Part B — Recents may not be reachable at all for a chat under a never-separately-navigated workspace

**Not yet root-caused as bug-or-by-design — this needs real investigation first.** Live-observed: a chat visibly `hasView:true` in the tree (greyed label, confirmed genuinely open in a pane) produces ZERO Recents entries — the whole band doesn't render (`recents-band.tsx`'s own `if (entries.length === 0) return null` is firing). `recents-for-project.ts`'s own doc comment states the likely cause outright: *"Only workspaces that ALREADY have a live store — `getAllActiveWorkspaceIds`, populated by `WorkspaceHost`'s active + keep-alive-retained set — are read. A workspace nobody has opened this session has no working/dormant chats to show."* If the chat in question is a bubble borrowing an ancestor workspace's store, and that ancestor's workspace route was never separately navigated-into this session (only the chat's OWN row was clicked, or it was reached via drag/pane manipulation without a sidebar navigation to its specific workspace), its data may be structurally unreachable to `deriveRecentsEntries` — which would undermine the entire point of Recents (per spec §5, "is this up right now" — answered independently of navigation history, not gated on it).

**Files to read first, in full, before deciding anything:** `web/src/components/sidebar/lib/recents-for-project.ts`, `web/src/features/workspace/stores/workspace-store-registry.ts` (`getAllActiveWorkspaceIds`, what makes a workspace's store "active" — is it keyed on navigation, on any chat of that workspace being referenced by a live pane, or something else?), `web/src/features/workspace/components/workspace-host.tsx` (the "active + keep-alive-retained set" the comment names).

- [ ] **Step 1: Reproduce and confirm precisely.** Live, via Tauri MCP: find a chat that is genuinely `hasView:true` (open in a pane) whose owning/ancestor workspace has NOT been separately navigated to this session (reachable via drag-drop from the tree into an existing pane, or via the New Tab stage's recent-chat list, without ever clicking that workspace's own row) — confirm Recents shows nothing for it. Then navigate directly into that workspace (click its row) and confirm whether Recents THEN picks it up (proving the "unvisited workspace" gate is the actual cause, not something else).

- [ ] **Step 2: Decide bug vs. by-design, backed by evidence, not a guess.** If `getAllActiveWorkspaceIds`'s "active" set can be extended to include any workspace that owns a chat currently referenced by a LIVE pane (`windowPaneStore`, which is genuinely global/window-level already, per `recents-for-project.ts`'s own comment) — regardless of navigation history — that's very likely the correct fix, since it makes Recents answer "what's up right now" independently of how you got there, matching the design's own stated purpose. If there's a real reason this gate exists (e.g. a workspace store needs real initialization/connection state that can't be cheaply created just to read `agentChats` for Recents), STOP and report the constraint clearly rather than forcing a fix that could mount expensive per-workspace resources just to populate a sidebar list.

- [ ] **Step 3: If it's a real, fixable gap** — write the failing test first (a chat live in a pane, under a workspace never separately activated, should still produce a Recents entry), implement, confirm GREEN, run the full sidebar/workspace test directories, live-verify via Tauri MCP that the previously-missing Recents band now appears for this exact scenario.

- [ ] **Step 4: If it's genuinely by-design or too costly to fix safely**, report precisely why, and do not force a change — this becomes a documented, deliberate limitation for the controller to decide whether to accept or escalate.

- [ ] **Step 5: Commit** each part separately.

---

## Task 14: AffordanceRow's icon is invisible until hover — make it always visible

**Inserted mid-execution.** Controller root-caused this live via Tauri MCP geometry inspection (not a guess): what looked in a user screenshot like large, dead, empty gaps between tree rows are actually the empty-container affordance rows (spec §3.5) rendering with entirely correct layout and spacing — the row itself is 36px tall, correctly positioned. The problem is that its icon has `display: none` at rest and only becomes visible on `:hover`, so an empty container (a workspace/folder with nothing in it yet) looks like a completely blank, unclickable void until the user's mouse happens to land exactly on that row.

**Root cause, exact:** `web/src/components/sidebar/affordance-row.tsx`'s trigger button(s) use `ROW_SUB_ACTION_HOVER` (`web/src/components/layout/workspace-row-base.ts:128`), whose `hidden ... group-hover:inline-flex group-focus-within:inline-flex group-data-[active]:inline-flex` is a deliberate, documented recipe for a row's SECONDARY/TRAILING controls (trash/create/chevron on an otherwise-populated row) — spec §3.1: "Trailing controls are revealed on row hover." `AffordanceRow` reuses this exact token for its PRIMARY AND ONLY content, which spec §3.5 describes as meant to always be visible: "No subtitles, no descriptions, no visible dropdown chrome — just the icon." A row whose entire content is a secondary-style hover-revealed control is, in practice, invisible and undiscoverable.

**Files:**
- Modify: `web/src/components/sidebar/affordance-row.tsx` — its trigger button(s) need a variant that keeps `ROW_SUB_ACTION_HOVER`'s padding/radius/color/hover-background treatment but drops the `hidden`/`group-*:inline-flex` visibility gating, so the icon renders at rest, every time, not just on hover.
- Possibly modify: `web/src/components/layout/workspace-row-base.ts` — if the cleanest fix is a new exported token (e.g. `ROW_SUB_ACTION` without the `_HOVER` visibility gate — check whether one already exists under a different name before adding a new one; if `ROW_SUB_ACTION_HOVER` was built by stacking gating onto a base recipe, the base without the gate may already be exported and reusable here).
- Test: `web/src/__tests__/components/sidebar/affordance-row.test.tsx`.

**Do not touch `ROW_SUB_ACTION_HOVER` itself or any of its other call sites** (trash/create/chevron on populated rows) — those correctly want hover-only visibility per spec §3.1; this task is scoped to `AffordanceRow` alone.

- [ ] **Step 1: Read `workspace-row-base.ts` in full** to find or confirm there's no existing always-visible variant of this recipe already exported under a different name. Read `affordance-row.tsx` in full to see both of its trigger-button call sites (the plain single-icon case and the dropdown case).

- [ ] **Step 2: Write the failing test** — the affordance row's icon/trigger button, rendered at rest (no hover, no focus), has a computed `display` other than `none` (or equivalently, does not carry the `hidden` class at all) — i.e. it's visible without any interaction, for both the single-icon and dropdown variants.

- [ ] **Step 3: Implement** — add or reuse an always-visible variant of the sub-action recipe (same padding/radius/color/hover-background as `ROW_SUB_ACTION_HOVER`, without the `hidden`/`group-*:inline-flex` gate) and apply it to `AffordanceRow`'s trigger button(s).

- [ ] **Step 4: Run the test, confirm PASS.** Run the full `web/src/__tests__/components/sidebar/` directory to confirm no regression to trailing-control hover behavior on populated rows (which must still hide-until-hover, unaffected by this change).

- [ ] **Step 5: Live-verify via Tauri MCP** — find an empty container in the live tree (or fold/create one), screenshot it at rest with the mouse elsewhere, confirm the affordance icon is now visible without hovering. Also confirm hovering it still shows the correct hover background treatment, and clicking it still opens the create-thread/create-workspace action correctly.

- [ ] **Step 6: Commit**

```bash
git commit -m "fix(sidebar): AffordanceRow's icon is always visible, not hidden until hover"
```
