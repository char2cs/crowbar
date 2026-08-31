# Sidebar restyle recovery — batch 2

**Date:** 2026-08-31

**Status:** open for execution.

**Relationship to the first recovery pass.** `2026-08-31-sidebar-restyle-recovery.md` (Tasks 1-8) is complete and closed — its four confirmed defects and three production-readiness items are fixed, reviewed, and live-verified. This document does not reopen or re-litigate that work. It exists because the user sent nine additional, direct pieces of feedback (with screenshots) after that pass closed. Each item below was re-grounded against primary sources (the design canvas's actual CSS/annotations, pre-restyle git history, and live re-verification) before being written down — none of it is taken on the screenshots' word alone.

**Directive governing this document, in the user's words:** "big bang approach... scrap and throw everything that is old into the trash... no fallback logic to what was before whatsoever... NEVER keep anything legacy." This applies to the surfaces named below. It does not license touching anything the first recovery pass already confirmed clean.

---

## 1. Confirmed, ready to fix

### 1.1 — The pane's chat head renders as a rounded pill, not the design's flat underline tab

**User's words:** "This tab concept is the old pre restyling one... refactor into a multi layer tab and pane system."

**What's actually wrong (confirmed by direct CSS comparison, not by the screenshot alone):** the pane's *visibility* logic is already correct — `Pane.dc.html`'s own decision table says the identity row shows even in chat-only mode ("The row is the chat and a plus. The pane is the conversation"), and current `tab-bar.tsx` already mounts it unconditionally per pane. The bug is styling only: `web/src/features/tabs/components/chat-head.tsx:33-37` renders `'h-8 shrink-0 gap-1.5 rounded-full px-2.5 text-[13px]'`, adding `'bg-background text-foreground shadow-sm'` when active — a filled, rounded, shadowed pill. The design's own artboard CSS (`Main.dc.html`, `.hitem`/`.hitem.is-on`) specifies: no background at rest, no border-radius anywhere, selection shown only by a flat 2px underline bar — identical treatment to the file tabs sitting right beside it in the same row.

**Fix:** rewrite `chat-head.tsx`'s className to match `.hitem`/`.hitem.is-on` — no radius, no fill, a `--primary` 2px bottom bar on active, matching whatever active-state pattern `tab-bar-item.tsx` (the file-tab equivalent) already uses so the two stay visually consistent.

### 1.2 — The file-explorer card's head renders at ~48px, not the spec's 28px

**User's words:** "The scaling of this is completely out of scale... parity with the mock should be 1:1."

**Confirmed live** (Tauri MCP, computed styles): head container measures 48px, individual Files/Git tab buttons measure 32px. Root cause: the tab buttons carry both `h-7` (28px, the intended value) and `sm:h-8` (32px), and the media-query-scoped utility wins the cascade — plus the head's own container padding adds further height on top. Spec §6.1/§11: the card head must be 28px, *deliberately less* than the segmented-bar alternative it replaced (36px) — the live 48px is taller than even the rejected alternative.

**Fix:** remove the `sm:h-8` (or whatever is producing it) so `h-7` actually wins, and audit the head container's padding against the 28px target end to end.

### 1.3 — The New Tab empty stage offers "New Chat," which the spec explicitly forbids there

**User's words:** "This is entirely gone, on this subpane system, you're only allowed to see files, or terminals. Chats are another concept of themselves."

**Confirmed live and in source:** `web/src/features/panes/components/new-tab-view.tsx:331-343` renders "New Chat" (bound to `⌘N`) as the first, default-focused action, ahead of "New Terminal" and "New File." Design spec §7.2: the editor view's empty stage offers exactly three ways in — a file from the sidebar card's Files, a terminal, or the branch review from Git. Chat is deliberately excluded: it's the pane's own identity (fixed at the head, per §1.1 above), never something opened *into* the editor view.

**Fix:** delete the "New Chat" action row and its `⌘N` binding from `new-tab-view.tsx`. Check what else the view currently renders below the divider (a capped recent-chat history list, per prior project memory `project_new_tab_empty_stage.md`) — that list existing is fine (it's navigation, not "opening a chat into this pane's editor view") as long as clicking an entry in it does the same thing clicking a tree/Recents row does (focuses/opens that chat's *own* pane) rather than loading the chat into the current editor-view pane. Verify this distinction holds; fix if it doesn't.

### 1.4 — Rename and icon-personalization: real pre-restyle features, deleted, never fully rebuilt

**User's words:** "renaming by double clicking. Icon personalization through clicking on the image... Now they're inside a right click menu which is incorrect."

**Confirmed via git history, not assumed:** commit `cf422bc5` ("finish the workspace sidebar — project icons, rename, delete, and per-branch locking") built real double-click-rename and a click-to-open icon picker (`icon-popover.tsx`, `project-icon-popover.tsx`, `repo-icon-popover.tsx`). The restyle's tree-retirement commit (`f119a402`) deleted `project-icon-popover.tsx`/`repo-icon-popover.tsx` outright and **its own commit message admits the gap**: rename/lock had "no home... until Parts C/D/G/H build [them] back onto SidebarTree." Those later parts rebuilt rename and lock as `row-context-menu.tsx` items — but only ever wired the right-click trigger. Double-click was never restored. Icon personalization was never rebuilt in any form. The generic `icon-popover.tsx` primitive (316 lines) still exists in the tree, unused — `repo-icon-mark.tsx`'s own comment confirms it used to route through this component and no longer does.

**Fix, two parts:**
- Add a double-click handler to `sidebar-row.tsx` and `space-header.tsx` that opens the same inline rename path the context menu's "Rename" item already reaches (`RenameDialog`/`performRenameRow` — reuse, don't duplicate). Keep the right-click "Rename" item too; both paths should work.
- Rebuild icon personalization on the still-present `icon-popover.tsx` primitive, wired to project and repo rows (the two row kinds that had it before). Check what `icon-popover.tsx`'s actual API surface is before assuming its old callers' shape still fits — it may need a thin new wrapper rather than the literal deleted `project-icon-popover.tsx`/`repo-icon-popover.tsx` files restored verbatim (this is the "big bang, no legacy" directive in practice: rebuild the capability cleanly against today's `SidebarRow`/`space-header.tsx`, don't resurrect deleted files as-is).

### 1.5 — Branch locking should be chosen at import time (new capability, not a regression)

**User's words:** "lock branches would be chosen upon importing them."

**Confirmed:** no evidence this ever existed pre-restyle; this is new direction, not a dropped feature. Today, locking is only reachable post-hoc via the right-click menu's Lock/Unlock toggle (`row-context-menu.tsx:89`, `performSetWorkspaceLock`). The right-click menu itself is otherwise correct and needs no fix — `New folder` and `Lock` are already present, correctly gated by row kind (branch/folder only); the screenshot that read as "missing" options was a chat row, which correctly shows only Rename.

**Fix:** add a per-branch lock choice to `repo-import-dialog.tsx`'s branch-import flow (read its current form fields first — this is new UI, not a restoration), wired so a branch's lock state is set atomically as part of import rather than requiring a separate post-import menu action.

---

## 2. Confirmed, large — the workspace/chat unification gap

**User's words:** "Workspaces are now obligated to have a chat always... visually, a workspace can only have a chat, then chat/workspaces child of them, but its always 1 row 1 chat. Thats a hard rule."

**This is real, and it is comparable in size to Tasks 2-6 of the first recovery pass combined.** It is not a frontend styling bug; it's an unfinished piece of the model spec's own backend sequencing (`docs/superpowers/specs/2026-08-23-unified-sidebar-design.md` §7, "Stage 5: One `CreateChild`; promotion via handoff respawn"), and the codebase says so itself:

- `api/internal/api/v0/endpoints/workspaces/handlers/crud.go:121`: *"Until Stage 5 (CreateChild/Promote unification) lands, CreateChild mints no [chat]."*
- Restated at `patch_test.go:150-152`, `hierarchy.go:109`, `crud_test.go:418`.

**What actually happens today, traced end to end:** creating a workspace from the sidebar's `+` calls `postWorkspace()` → the old `POST /v0/projects/:p/repos/:r/workspaces` route → `CreateChild`, which mints **no chat**. `rows-from-repo.ts` then renders every workspace as a bare, non-opening `branch`-kind row; a user has to separately create a chat as a *child* row underneath it. Per the model spec's own four-row taxonomy, this is wrong for anything other than the three protected/locked branches: a regular workspace should be **one row** carrying both facts at once (`Type: chat`, `WorkspaceID` set — "a worktree chat," which opens) — not a container row plus a separate child.

`POST /repos/:rid/chats` (the model spec's intended unified creation route) does already exist, but its request body only accepts `{provider, parentId, workspaceId}` — it can attach a chat to an *existing* workspace, it cannot mint a *new* one. No `GET /repos/:rid/tree` unified read endpoint exists either, but that's not the blocker here — the create path is.

**The good news that narrows this task considerably:** `api/internal/app/usecases/chat/promote.go` already implements almost the exact machinery this needs — it resolves a fork parent, cuts a worktree, and attaches the result to a chat, for the *promote an existing bubble* case. The missing piece is the same worktree-minting logic composed with *creating a new chat* instead of updating an existing one.

**Fix, backend then frontend, in that order (frontend depends on the backend actually minting the chat):**

1. **Backend:** extend `POST /repos/:rid/chats`'s request body with an `ownWorktree: boolean` (mirroring the model spec's `CreateChild{ParentID, OwnWorktree, ProviderID}` shape exactly). When true and no `workspaceId` is given: resolve the fork parent (reuse `promote.go`'s resolution logic), cut the worktree, create the chat row with `WorkspaceID` set to the new workspace — one atomic operation, not two round-trips. Read `promote.go` in full first; adapt its pattern rather than writing new worktree-provisioning logic from scratch. Add regression tests mirroring `promote_test.go`'s existing coverage shape.
2. **Frontend:** swap `handleCreate`'s `kind === 'workspace'` branch (`space-content-actions.ts`) from `postWorkspace()` to the extended `POST /repos/:rid/chats {ownWorktree: true}`. Confirm `rows-from-repo.ts`/`buildSidebarTree` render the result as one row (a `chat`-kind row that also owns a workspace) rather than a bare branch — this may already fall out correctly once the backend stops creating a chat-less workspace, since the tree-builder's chat-node branch already produces a full row; verify rather than assume, and fix `buildSidebarTree`'s classification if a workspace-owning chat still gets misrouted into the branch-only path.
3. **Do not touch the three protected/locked-branch rows** (`develop`, `main`, project home) — these are the one legitimate case of a workspace-owning row with no chat, per the model spec's own taxonomy, and must keep working exactly as they do today.

---

## 3. Flagged, not acted on — needs the user's confirmation

### 3.1 — Project-selector placement

**User's words:** "The project selector should be right down the file/git explorer" (Image #6, a tightly cropped "library-icon + plus" snippet).

**Live-confirmed current placement:** the project marks + add-project `+` sit in the window chrome's very first 44px row (`data-testid=space-mark`/`add-project-mark`, `y:0-44`), above the SPACES scroller entirely — matching Task 5's implementation and the closed design spec §4.1 exactly, which justifies this placement at length (it's the discoverable counterpart to the wheel-swipe gesture, deliberately at the rail level because "at rest one panel fills the rail, so with no neighbour visible there is nothing to click").

**Why this is flagged instead of changed:** relocating it to sit near the floating file-explorer card (at the sidebar's *bottom*) would reverse a specific, heavily-justified, twice-confirmed design decision (written spec + canvas annotations) based on a single, ambiguously-cropped 2-icon screenshot with no supporting context for *why* it should move. The cost of guessing wrong here is high (undoing real, tested work); the cost of asking is one clarifying question next time the user is back. **Recommendation:** leave as-is until the user confirms whether they mean literal relocation, or something else (e.g. a different visual treatment, or a reference to the OLD app's layout rather than the new mock).

---

## 4. Constraint carried through every task above

**Preserve Recents' reorder animations and styling.** The user's own words: "the animations and styling for reordering the view, should be preserve as chats can now be rearrange as the user wants it to be." None of the tasks above are expected to touch Recents' drag/reorder code directly, but §2's row-unification work changes what counts as a "workspace row" vs. a "chat row" feeding into Recents — verify no animation/reorder regression as part of that task's own testing, not as an afterthought.
