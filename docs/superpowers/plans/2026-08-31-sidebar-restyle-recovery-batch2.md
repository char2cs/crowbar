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
