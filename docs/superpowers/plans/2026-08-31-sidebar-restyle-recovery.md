# Sidebar Restyle Recovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring `enhancement/unify-sidebar` to verified 1:1 parity with the closed design (`docs/superpowers/specs/2026-08-28-sidebar-and-pane-surface-design.md`, ground truth the `Main.dc.html` "The sidebar, live" artboard) by closing the 4 confirmed defects, resolving the 1 open design gap, then verifying nothing else is actually broken.

**Architecture:** This is recovery, not a rewrite. Four independent audits already found the sidebar/pane component code substantially spec-conformant (see the recovery spec §2). Task 1 confirms or kills the leading "stale build" hypothesis for why a live screenshot looked broken; Tasks 2-5 close the four confirmed, narrow defects (all frontend-only wiring — backend routes already exist and are tested); Tasks 6-7 are checklist-driven verification passes whose exact scope may be amended after Task 1's findings land (this repo's own convention — see `d7f81f77`, `1723009c` for precedent); Task 8 signs off.

**Tech Stack:** React + TypeScript (web/), Zustand stores, Tailwind, Go backend (api/), Tauri desktop shell.

**Spec:** `docs/superpowers/specs/2026-08-31-sidebar-restyle-recovery.md` (this plan's argument — read it in full before Task 1; conflicts in this plan resolve against it and, beneath it, against `docs/superpowers/specs/2026-08-28-sidebar-and-pane-surface-design.md`).

## Global Constraints

- Component files: kebab-case (`my-component.tsx`), PascalCase export.
- Tests live in `web/src/__tests__/` mirroring `web/src/`, using `@/` imports — never a co-located `tests/` directory. Several of this plan's target files already have test files at the mirrored path; add cases there rather than creating new files.
- Store access: narrow `useXxxStore((state) => state.field)` selectors only; `.getState()` confined to event handlers/effects; stores never import from `components/`.
- **Do not touch anything the recovery spec's §2 ("audited and confirmed conformant") already lists as conformant** unless a task below explicitly says to — that budget was already spent once; re-litigating conformant code wastes it twice.
- No migration, no legacy/back-compat shims (repo convention, restated in the surface spec's own header) — Crowbar is pre-production.
- Every task that changes behavior needs a regression test that fails on pre-fix code (`web/src/__tests__/...`).
- Ruling (made now, not deferred): §3 of the recovery spec's open design question — where "add a project" lives once the New Project row is gone — is resolved as **a trailing `+` mark in the window chrome's dead middle, after the last project's space mark**, per the recovery spec's own recommendation (smallest change, same interaction language as the existing space marks). Task 5 implements this.

---

## Task 1: Live verification against a fresh build

**Files:**
- No source files touched unless the rebuild itself surfaces a real bug (then: whatever file that bug is in, plus its test).
- Report: `.superpowers/sdd/2026-08-31-sidebar-restyle-recovery/task-1-findings.md`

**Interfaces:**
- Produces: a written verdict on the recovery spec §0's "stale build" hypothesis and §2.3's three unverified items (pane gutter values, vibrancy/theme, trust-banner overlap), which Tasks 6-7 below will be scoped or re-scoped against.

**Context this task needs that the plan can't give in advance:** you have Tauri MCP tools available (`mcp__tauri__*`) — load them via `ToolSearch` if deferred. This is a desktop app; there is no headless/browser equivalent that exercises the same transport, per this repo's own convention (`feedback_verify_via_dev_desktop_not_headless`).

- [ ] **Step 1: Clean rebuild**

Run `make dev-desktop` from a clean state (confirm it fully rebuilds the frontend bundle and the sidecar — do not reuse a stale running instance; if one is already running, ask how the running instance was started before assuming it reflects current `HEAD`, since a long-lived dev process can be serving a stale bundle). Record the exact commands run and their output in the findings report.

- [ ] **Step 2: Screenshot the sidebar at rest**

Using the Tauri MCP (`mcp__tauri__webview_screenshot`, `mcp__tauri__webview_dom_snapshot`, etc.), capture the sidebar rail in the running app:
- Dark mode (the design's default) and light mode (`data-theme` toggle — check `useSettingsStore`'s `themeMode` setting for how to force it).
- With at least one project open showing a mix of row kinds (branch, chat, folder) if the dev environment's seed data has them; if not, use whatever the fixture provides and note what was and wasn't exercisable.
- A chat opened into a pane (to check whether its tree row greys — this will still be broken pre-Task-2, note it as expected).
- A fresh/untrusted worktree if one is reachable, specifically to check the trust-prompt banner's position relative to the composer (recovery spec §2.3).

- [ ] **Step 3: Compare against the design artboard**

The design canvas's `Main.dc.html` ("The sidebar, live" artboard) is the ground truth: <https://claude.ai/code/artifact/5a1008de-282b-494a-bd8d-1b9c123efdee>. Read it with the `Artifact` tool (`action: "read"`) if accessible, or use the numeric values already extracted into the surface spec's §11 "Numbers, lifted not invented" table as the comparison baseline where a direct artboard read isn't practical. Compare: row height (~36px, not default-browser sizing), rail width (~294-295px), background (frosted vibrancy, not a raw/photographic desktop image showing through).

- [ ] **Step 4: Verdict**

Write `task-1-findings.md` with:
1. Does a clean rebuild show compact, spec-matched spacing, or does the "everything oversized" symptom persist? This directly confirms or kills the stale-build hypothesis.
2. Does the background show vibrancy/blur, or a raw unblurred desktop? If broken, cite the specific `mcp__tauri__` evidence (computed styles, console errors) — do not just re-assert the hypothesis from the spec.
3. Does the trust-prompt banner overlap the composer live, and under what exact condition (fresh worktree only, per the spec's expectation)?
4. What are the actual pane gutter values (spec §7.4: 4px left/top, 8px between neighbours) — measured live, since static reading couldn't locate them.
5. A prioritized list of anything else observed live that doesn't match the design spec, each with a screenshot reference and the specific spec section it violates — this is new evidence, not a re-statement of the recovery spec's existing findings.

**This task has no fixed pass/fail spec compliance in the usual sense** — its job is to produce evidence. Treat "the app is fine, the screenshot was stale" as a completely valid, successful outcome, not a failure to find something.

---

## Task 2: Wire live pane-membership into the tree's `hasView`

**Files:**
- Modify: `web/src/components/sidebar/sidebar-tree.tsx`
- Modify: `web/src/components/sidebar/recents-band.tsx` (only if the live-pane-membership selector needs extracting into a shared helper both files call — check first; do not duplicate logic that already exists for Recents)
- Test: `web/src/__tests__/components/sidebar/sidebar-tree.test.tsx`

**Interfaces:**
- Consumes: `SidebarRowType` (`web/src/components/sidebar/types/sidebar-row.ts`) — `hasView: boolean` field already exists, currently always `false` from `rows-from-repo.ts`. Do NOT change `rows-from-repo.ts` — the spec's own comment there (line ~74-88, attached to `working: false`) explains why seeding live state into that pure bridge function is the wrong layer: "Anything else that comes to need real turn state here must subscribe per row the way Recents does (`recents-band.tsx`'s `RecentsMemberRow`) — never seed it into the row object, which is the latch described above." The same reasoning applies to `hasView`.
- Produces: `SidebarTree` renders each row with a LIVE `hasView`, computed per-row via a narrow store subscription, overriding the `false` the row object arrives with.

**Why this is a per-row-component problem, not a one-line fix:** `sidebar-tree.tsx`'s `renderRow` is a plain recursive function called inside `.map()`/recursion, not a React component — calling a hook inside it would violate the rules of hooks (varying call count/order as the tree's shape changes across renders). `recents-band.tsx` already solves the identical problem correctly: it renders each entry through a real component, `RecentsMemberRow` (lines 245-317), which calls `useWorkspaceStoreById(workspaceId, selector)` — a narrow, per-row selector — and constructs the `SidebarRowType` object inline with the live value, rather than trusting a value baked in ahead of render.

- [ ] **Step 1: Find or add the "is this chat's view open" narrow selector**

Panes live in `web/src/features/panes/stores/slices/pane-slice.ts` (`window-pane-store.ts`), keyed by pane id, each pane holding `.chatId`. First check whether an equivalent "is chat X already open in some pane" boolean already exists — the design spec requires drop-dedup ("dropping a chat that's already up goes to it, never opens twice"), so grep `web/src/components/sidebar/lib/sidebar-drop-policy.ts` and `web/src/components/sidebar/hooks/use-sidebar-drag.ts` for it first and reuse verbatim if found. If nothing reusable exists, add a narrow selector (module-level function or exported from `pane-slice.ts`/`window-pane-store.ts`) shaped like:

```ts
export function selectChatHasView(state: WindowPaneState, chatId: string): boolean {
  return Object.values(state.panes).some((pane) => pane.chatId === chatId)
}
```

(Adjust to the actual `panes` shape in `pane-slice.ts` — read it first; the type may be a Map, a Record, or a tree that needs a small recursive walk since panes form a binary split tree per spec §7.3. If it's a tree, walk it the same way `pane-node-renderer.tsx` does rather than re-deriving a new traversal.)

- [ ] **Step 2: Wrap the tree's row render in a component that subscribes per-row**

In `sidebar-tree.tsx`, extract a new function component (e.g. `SidebarTreeRow`) that takes `row: SidebarRowType` plus the rest of the props `renderRow` currently threads through to `<SidebarRow>`, and inside it:

```tsx
function SidebarTreeRow({ row, ...rest }: { row: SidebarRowType } & /* rest of SidebarRow's props minus `row` */) {
  const hasView = useWindowPaneStore((s) => selectChatHasView(s, row.id))
  return <SidebarRow row={{ ...row, hasView }} {...rest} />
}
```

Use whatever the real pane store hook is named (check `window-pane-store.ts` for its exported `useWindowPaneStore` or equivalent — do not invent a new store access pattern). Only a `kind: 'chat'` or `kind: 'branch'` row (one that can actually be opened into a pane) needs the live subscription — a `folder` row can never have a view, so either gate the subscription on `row.kind !== 'folder'` or confirm the selector naturally returns `false` for a folder id (no pane will ever hold a folder's id as `chatId`, so this is likely already safe without a special case — verify with a test).

- [ ] **Step 3: Replace the direct `<SidebarRow row={row} .../>` call in `renderRow` with `<SidebarTreeRow row={row} .../>`**

Keep every other prop (`onOpen`, `onTrash`, `onCreate`, `onToggleFold`, `folded`, `dragProps`, `isDragging`, `isNestTarget`, `onPointerDownDrag`) passed through unchanged.

- [ ] **Step 4: Write the failing test first**

In `web/src/__tests__/components/sidebar/sidebar-tree.test.tsx`, add a case that opens a chat into a pane (via whatever the test file's existing setup uses to seed pane state — check its existing imports/mocks first) and asserts the corresponding tree row renders with the greyed-label class (`text-muted-foreground`) that `sidebar-row.tsx:129` already applies when `hasView` is true. Run it before Step 1-3's implementation and confirm it fails (the row currently never greys).

- [ ] **Step 5: Implement Steps 1-3, then run the test**

Run: `bun vitest run web/src/__tests__/components/sidebar/sidebar-tree.test.tsx`
Expected: PASS. Also run the full sidebar test directory to confirm no regression: `bun vitest run web/src/__tests__/components/sidebar/`

- [ ] **Step 6: Commit**

```bash
git add web/src/components/sidebar/sidebar-tree.tsx web/src/__tests__/components/sidebar/sidebar-tree.test.tsx
git commit -m "fix(sidebar): wire live pane membership into the tree's hasView"
```

---

## Task 3: Wire a chat row's trash to the existing delete-chat backend route

**Files:**
- Modify: `web/src/components/layout/space-content-actions.ts` (`handleTrash`)
- Modify: `web/src/components/sidebar/sidebar-row.tsx` (`deletable` computation)
- Modify: `web/src/components/layout/sidebar-tree-surface.tsx` (`DeleteConfirmDialog` wiring, if the chat case needs a different `repoId`/`chatId` resolution path than the current `resolveRow`-based one)
- Test: `web/src/__tests__/components/layout/space-content-actions.test.ts`, `web/src/__tests__/components/sidebar/sidebar-row.test.tsx`

**Interfaces:**
- Consumes: `deleteChat(wsId: string, id: string, init?: RequestInit): Promise<void>` — already exists at `web/src/features/agent/api/agent-api.ts:990`, already resolves through `chatBase(wsId)` to the correct, current, repo-scoped route (`DELETE /v0/projects/:p/repos/:r/chats/:id`, backed by `api/internal/api/v0/endpoints/chat/routes.go:113`, `h.Delete`). **Do not modify `deleteChat` or `chatBase` — they are already correct.** Consumes `resolveChatRow(repos, id)` (`space-content-actions.ts:36`, already exists) to find a chat's owning repo.
- Produces: `handleTrash(id)` returns `true` and actually deletes when `id` names a chat, exactly as it already does for workspaces/folders.

**Context `space-content-actions.ts`'s existing comments give you:** `handleTrash`'s current chat branch (line ~180) explicitly checks `resolveChatRow` first and returns `false`, with an in-code comment explaining this was a deliberate stopgap because nothing downstream could act on a chat subject — that reasoning stands for the removal-TRAY path (`planRemoval`/`RemovalDraft`, which only knows `workspace | folder | repo | project` — do not widen that union, `resolveChatRow`'s own doc comment explains why not). A chat's delete does not go through the removal tray at all — it's a direct API call with its own confirm dialog, which `DeleteConfirmDialog` + `delete-preview-client.ts` (`fetchDeletePreview`) already provide generically (they don't care whether the row is a workspace or a chat, they just need `projectId`/`repoId`/`chatId`).

- [ ] **Step 1: Resolve the workspace id a chat's delete request needs**

A chat may have no owning workspace (a bubble). The DELETE route is repo-scoped, so ANY workspace of the SAME repo resolves the URL correctly — this is exactly the pattern `row-actions.ts`'s `scopedWorkspaceIdOf(repo)` already implements (`repo.defaultWorkspaceId ?? repo.workspaces[0]?.id`). Reuse that function (export it from `row-actions.ts` if it isn't already exported) rather than re-deriving it.

- [ ] **Step 2: Write the failing test**

In `web/src/__tests__/components/layout/space-content-actions.test.ts`, add a case: a repo with a chat row (no workspace, i.e. a bubble) and at least one real workspace; call `handleTrash(chatId)`; assert it returns `true` and that `deleteChat` (mock it) was called with `(scopedWorkspaceId, chatId)`. Run it now and confirm it fails (`handleTrash` currently returns `false` unconditionally for a chat).

- [ ] **Step 3: Implement the chat branch of `handleTrash`**

```ts
export function handleTrash(id: string): boolean {
  const currentRepos = useSidebarStore.getState().repos
  const chatRow = resolveChatRow(currentRepos, id)
  if (chatRow) {
    const wsId = scopedWorkspaceIdOf(chatRow.repo)
    if (!wsId) return false
    deleteChat(wsId, id).catch((err: unknown) => {
      toast.error(err instanceof Error ? err.message : 'Failed to delete chat')
    })
    return true
  }
  const found = resolveRow(currentRepos, id)
  // ...unchanged from here
}
```

Adjust to match the actual current shape of the function (read it fresh before editing — this plan was written against the code as of this branch's `HEAD` at planning time, and Task 2 may have touched adjacent lines). Import `deleteChat` from `@/features/agent/api/agent-api` and `scopedWorkspaceIdOf` from `./row-actions` (or wherever Step 1 exported it from).

- [ ] **Step 4: Make chat rows deletable**

In `sidebar-row.tsx`, change `const deletable = row.kind !== 'chat'` to `const deletable = true` (every row kind is now deletable — the project-home guard already above it, `!isProjectHome`, still applies independently). Update the comment above it: the old comment explains why the control was ABSENT; replace it with a short note that it's now wired (per this repo's "no verbose comments" convention — state what's non-obvious, not what changed).

- [ ] **Step 5: Confirm `sidebar-tree-surface.tsx`'s `DeleteConfirmDialog` wiring already works for a chat id**

Read the current wiring (`deletingRow`/`deletingRepo`/`chatId={deletingRowId ?? ''}` etc.) — `resolveRow` in that file's `deletingRepo` lookup does NOT resolve a chat (per `resolveChatRow`'s own doc: "callers must consult THIS FIRST"). If `deletingRepo` comes back `undefined` for a chat id, the dialog's `projectId` prop will be `undefined` and `fetchDeletePreview` will be skipped (falls into the `previewFailed` branch per `delete-confirm-dialog.tsx`'s effect) — which degrades gracefully ("This takes {label} and everything under it.") but loses the real file/chat counts. Fix `deletingRepo`'s resolution in `sidebar-tree-surface.tsx` to try `resolveChatRow` first, matching the pattern `handleOpen` already uses, so a chat's delete-preview gets the same real counts a workspace's does.

- [ ] **Step 6: Run the tests**

Run: `bun vitest run web/src/__tests__/components/layout/space-content-actions.test.ts web/src/__tests__/components/sidebar/sidebar-row.test.tsx web/src/__tests__/components/layout/sidebar-tree-surface.test.tsx`
Expected: PASS, including the new case from Step 2.

- [ ] **Step 7: Commit**

```bash
git add web/src/components/layout/space-content-actions.ts web/src/components/sidebar/sidebar-row.tsx web/src/components/layout/sidebar-tree-surface.tsx web/src/__tests__/components/layout/space-content-actions.test.ts web/src/__tests__/components/sidebar/sidebar-row.test.tsx
git commit -m "fix(sidebar): wire a chat row's trash to the existing delete-chat route"
```

---

## Task 4: Build the chat-to-worktree promotion dropdown on the glyph

**Files:**
- Modify: `web/src/components/sidebar/sidebar-row.tsx` (`RowGlyph`, made clickable for a promotable chat)
- Modify: `web/src/components/layout/space-content-actions.ts` or a new small `performPromoteChat` in `web/src/components/sidebar/lib/row-actions.ts` (follow that file's existing `perform*` naming convention)
- Test: `web/src/__tests__/components/sidebar/sidebar-row.test.tsx`, `web/src/__tests__/components/sidebar/lib/row-actions.test.ts`

**Interfaces:**
- Consumes: `POST /repos/:rid/chats/:id/promote` — already live, `api/internal/api/v0/endpoints/chat/routes.go:112`, backed by `api/internal/app/usecases/chat/promote.go` with its own tests (`promote_test.go`, `regression_promote_test.go`). Frontend has no wrapper for this yet — check `web/src/features/agent/api/agent-api.ts` and `web/src/lib/api.ts` first for an existing `promoteChat`/`promote` function before writing one; if none exists, add one following `deleteChat`'s exact shape (same file, same `chatBase(wsId)` pattern):
  ```ts
  export async function promoteChat(wsId: string, id: string): Promise<void> {
    await apiFetch<unknown>(`${chatBase(wsId)}/${encodeURIComponent(id)}/promote`, { method: 'POST' })
  }
  ```
- Produces: clicking a bubble chat row's glyph opens a two-item dropdown ("Open as chat" is implicit — clicking elsewhere on the row already does that — the dropdown's one real item is "Make workspace" / promote); confirmed on click, the row's chat becomes worktree-owning.

**Who can promote:** per the model spec §3.1/§4.2, ANY chat with `ownsWorktree: false` (a bubble) can always be promoted — a bubble's cwd walk always terminates at a real worktree ancestor by construction, so there's no separate "is a valid parent available" check to compute. Gate purely on `row.kind === 'chat' && !row.ownsWorktree`.

**Amended after Task 1's live findings:** Task 1 found a live, reproducible bug in the exact pattern this task is told to reuse — `affordance-row.tsx`'s `DropdownMenu`/`MenuTrigger asChild` combination renders two nested `<button>` elements (console: `<button> cannot contain a nested <button>`, `React does not recognize the 'asChild' prop on a DOM element`), because Base UI's `MenuTrigger`'s `asChild` isn't correctly merging onto its child button. **Step 0, before building the new dropdown:** fix `affordance-row.tsx`'s own trigger so it no longer renders a nested button (check how other working `DropdownMenuTrigger asChild` call sites in this codebase structure their trigger child — e.g. search for other `asChild` usages of the same `@/components/ui/dropdown-menu` primitives that don't throw; the fix is almost certainly either passing a non-`<button>` element as the `asChild` child, or removing `asChild` and letting `MenuTrigger` render its own trigger element directly). Add a regression test asserting no nested-button warning/error for `AffordanceRow`'s dropdown variant. Only then build Task 4's own dropdown on the corrected pattern — do not propagate the bug to a second call site.

- [ ] **Step 1: Write the failing test for the dropdown's presence**

In `web/src/__tests__/components/sidebar/sidebar-row.test.tsx`, add a case: a chat row with `ownsWorktree: false` renders a clickable/dropdown-triggering glyph (assert on `data-testid="promote-dropdown"` or similar, following `affordance-row.tsx`'s existing `data-testid="affordance-dropdown"` naming convention); a chat row with `ownsWorktree: true`, or a non-chat row, does not. Confirm it fails first.

- [ ] **Step 2: Add the dropdown to `RowGlyph`'s wrapping span**

Reuse `@/components/ui/dropdown-menu`'s `DropdownMenu`/`DropdownMenuTrigger`/`DropdownMenuContent`/`DropdownMenuItem` — the exact same primitives `affordance-row.tsx` already uses. Only wrap the glyph span in a dropdown trigger when `row.kind === 'chat' && !row.ownsWorktree && !row.working` (a working chat can't be reparented/promoted per the busy-drag-refusal law's spirit — confirm this against the model spec's §4.3 "a working row does not move" before deciding whether promotion should also be refused while working; if the backend's `promote.go` already refuses a working chat server-side, the frontend should refuse it up front too rather than let the click round-trip into an error toast). The dropdown's one item: `"Make workspace"`, calling the new `performPromoteChat`/`promoteChat` action on click, with `e.stopPropagation()` so it doesn't also fire the row's `onOpen`.

- [ ] **Step 3: Wire the action**

```ts
export async function performPromoteChat(chatId: string): Promise<void> {
  const repo = useSidebarStore.getState().repos.find((r) => r.chats?.some((c) => c.id === chatId))
  if (!repo) return
  const wsId = scopedWorkspaceIdOf(repo) // same helper Task 3 exported
  if (!wsId) return
  try {
    await promoteChat(wsId, chatId)
  } catch (err) {
    toast.error(err instanceof Error ? err.message : 'Failed to promote chat')
  }
}
```

Do not write optimistic UI here — every other `perform*` action in this codebase (renames, creates) relies on the daemon's broadcast/reseed to update the row, per `row-actions.ts`'s own repeated doc comments on why optimistic writes were rejected. Match that pattern.

- [ ] **Step 4: Run the tests**

Run: `bun vitest run web/src/__tests__/components/sidebar/sidebar-row.test.tsx web/src/__tests__/components/sidebar/lib/row-actions.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/sidebar/sidebar-row.tsx web/src/components/sidebar/lib/row-actions.ts web/src/features/agent/api/agent-api.ts web/src/__tests__/components/sidebar/sidebar-row.test.tsx web/src/__tests__/components/sidebar/lib/row-actions.test.ts
git commit -m "feat(sidebar): build the chat-to-worktree promotion dropdown (spec §3.5)"
```

---

## Task 5: Relocate "New Project" to the window chrome, delete the leftover row

**Files:**
- Modify: `web/src/components/layout/sidebar-project-header.tsx` (add the trailing `+` mark)
- Modify: `web/src/components/layout/sidebar-tree-chrome.tsx` (remove the "New Project" button block; keep `RemovalTray`, dialogs, context menu)
- Modify: whichever file mounts BOTH `SidebarProjectHeader` and `SidebarTreeSurface`/`SidebarTreeChrome` (find it — likely `web/src/components/layout/ide-shell.tsx` — before writing this task's implementation, since `ImportProjectModal`'s open state needs to live somewhere both can reach; if no natural shared ancestor exists close by, lift the state as high as the nearest one that already holds `activeProjectId`/`projects`, matching how `SidebarProjectHeader`'s `activeProjectId`/`onSelectProject` are already threaded)
- Test: `web/src/__tests__/components/layout/sidebar-project-header.test.tsx`, `web/src/__tests__/components/layout/sidebar-tree-chrome.test.tsx`

**Interfaces:**
- Consumes: `ImportProjectModal` (`web/src/components/projects/import-project-modal.tsx`) and `importProjectAndSync` (`web/src/lib/store/projects.ts`) — both already exist and already work (`sidebar-tree-chrome.tsx`'s current `handleImportProject` is the reference implementation to move, not rewrite).
- Produces: a trailing icon-only `+` mark after the last project's space mark in `SidebarProjectHeader`'s `marks` cluster, opening the same `ImportProjectModal`; the "New Project" row and its `useState`/`handleImportProject` are deleted from `sidebar-tree-chrome.tsx`.

**Ruling this task implements (recorded in Global Constraints above):** trailing `+` mark, not a reopened tree-foot row — this closes the recovery spec's §3 open question.

- [ ] **Step 1: Find the real mount point**

Read whatever file renders both `<SidebarProjectHeader>` and `<SidebarTreeSurface>` (the recovery spec's audits identified `ide-shell.tsx` as mounting the rail in header → tree → carousel order — confirm this is still accurate before proceeding). Note exactly what props each already receives there (`projects`, `activeProjectId`, `onActiveProjectChange`) — the new `+` mark's modal state should live at this same level, alongside them, not deeper.

- [ ] **Step 2: Write the failing test**

In `web/src/__tests__/components/layout/sidebar-project-header.test.tsx`, add a case asserting a trailing button (distinct `data-testid`, e.g. `"add-project-mark"`) renders after the last project mark and calls an `onAddProject` callback prop on click. In `web/src/__tests__/components/layout/sidebar-tree-chrome.test.tsx`, add/update a case asserting the tree chrome NO LONGER renders a "New Project" button (query for its old text/testid and assert absence). Confirm both fail against the current code first (the second one fails because the button currently exists).

- [ ] **Step 3: Add the mark to `SidebarProjectHeader`**

Add an `onAddProject?: () => void` prop; in the `marks` JSX, after `{projects.map(...)}`, render one more `<Button>` (same `variant="ghost" size="icon-sm"` as the project marks, `data-testid="add-project-mark"`, `aria-label="Add project"`) with a `Plus` icon (Lucide, matching this cluster's existing Lucide-not-Phosphor convention per the file's own top-of-file comment), calling `onAddProject?.()`. Only render it when `onAddProject` is supplied (mirrors how every other optional control on this row already degrades).

- [ ] **Step 4: Remove the row from `sidebar-tree-chrome.tsx`, lift its state up**

Delete the `importProjectOpen` state, `handleImportProject` callback, and the "New Project" `<div className="px-1.5">...</div>` block (lines ~43-77 as read during planning — re-locate exactly, this plan's line numbers may have drifted after Tasks 2-4). `SidebarTreeChrome` keeps mounting `<ImportProjectModal>` only if its open-state is passed in as a prop from the lifted-up parent (Step 1's mount point) — or, simpler, move the `<ImportProjectModal>` mount itself up alongside the new state, out of `SidebarTreeChrome` entirely, since nothing else in that component needs it once the trigger button is gone.

- [ ] **Step 5: Wire it through the mount point found in Step 1**

Add the lifted `importProjectOpen` state + `handleImportProject` (moved verbatim) at that level; pass `onAddProject={() => setImportProjectOpen(true)}` into `<SidebarProjectHeader>`; mount `<ImportProjectModal open={importProjectOpen} onOpenChange={setImportProjectOpen} onImport={handleImportProject} />` there instead of inside `SidebarTreeChrome`.

- [ ] **Step 6: Run the tests**

Run: `bun vitest run web/src/__tests__/components/layout/sidebar-project-header.test.tsx web/src/__tests__/components/layout/sidebar-tree-chrome.test.tsx web/src/__tests__/components/layout/sidebar-tree-surface.test.tsx`
Expected: PASS. Also grep the whole `web/src` tree for any other test asserting the old "New Project" row's presence (there may be one in an `ide-shell`-level test) and update it.

- [ ] **Step 7: Commit**

```bash
git add web/src/components/layout/sidebar-project-header.tsx web/src/components/layout/sidebar-tree-chrome.tsx web/src/__tests__/components/layout/sidebar-project-header.test.tsx web/src/__tests__/components/layout/sidebar-tree-chrome.test.tsx
git commit -m "fix(sidebar): relocate New Project to a window-chrome mark, delete the leftover row"
```
(Add whatever mount-point file Step 5 touched to this commit too.)

---

## Task 6: Production readiness — scope finalized from Task 1's live findings

**Files:**
- Delete: `web/src/components/layout/drop-rules.ts` and its test `web/src/__tests__/components/layout/drop-rules.test.ts` — confirmed dead code (recovery spec §2: only live importers pull the `DragSubject` type, not its policy logic). Before deleting, grep once more for `from '@/components/layout/drop-rules'` / `from './drop-rules'` across `web/src` and move the `DragSubject` type export to wherever `sidebar-drop-policy.ts` or `space-content-actions.ts` can own it instead, so the two current type-only importers (`removal-plan.ts`, `space-content-actions.ts`) keep compiling.
- Modify: `web/src/components/layout/pane-border.ts` (`buildPaneContentStyle`) — **CONFIRMED live, not conditional**: Task 1 measured the pane sitting completely flush (0px) against both the sidebar and the window top, and grepped the whole pane layer (`pane-container.tsx`, `pane-node-renderer.tsx`, `split-view-root.tsx`, `pane-sash.tsx`) confirming no gutter-producing margin/padding/inset exists anywhere. Spec §7.4/§11: 4px inset on left/top always (so the gutter above the first pane equals the one beside it and two neighbours sit 8px apart), right/bottom give it up at the window (this part is already correct — do not touch it). Add the 4px inset to whichever layer actually controls a pane's outer box — likely `buildPaneContentStyle` gains a `margin`/`inset` alongside its existing `border`/`borderRadius`, gated the same way the border-radius logic already is (`atLeft`/`atTop`/`atRight`/`atBottom`, threaded down from `pane-node-renderer.tsx`'s `binaryPosition()` per the recovery spec's own §2 note that this propagation is already real and correct) — read `pane-border.ts` and `pane-node-renderer.tsx` in full before implementing; do not guess at the exact insertion point.
- Modify: `web/src/features/agent/components/agent-terminal-wait-banner.tsx` and/or `web/src/features/agent/components/agent-chat-pane.tsx` — **CONFIRMED live, window-width-dependent**: Task 1 measured a 7px real overlap at 1280px window width (banner text wraps to 3 lines, its `absolute top-2` box grows taller than the fixed gap to the composer below) and no overlap at 1056px. The fix needs to reserve **dynamic** space for the banner's actual rendered height, not apply a fixed offset — e.g. render the banner in normal document flow above the composer (so the composer is pushed down by however tall the banner actually is) rather than `absolute`-positioned over it, if that doesn't conflict with the "first-turn event, pane-level overlay" framing already in its doc comment. Read that doc comment in full before changing the positioning strategy — the original absolute-overlay choice may have its own reason this plan hasn't seen.
- **Do NOT touch `desktop/src-tauri/src/lib.rs` (vibrancy).** Task 1 found no evidence vibrancy is actually broken — translucent alpha backgrounds, correct `transparent`/`macOSPrivateApi` config, no visual artifacts in either theme's screenshot. The one gap (a decisive native-layer check) was blocked by a dev-tooling permissions issue (`mcp-bridge:allow-script-result`), not by anything indicating the app itself is broken. Fixing unconfirmed-working code here would be exactly the mistake this whole recovery is trying to avoid repeating.
- Test: `web/src/__tests__/components/layout/pane-border.test.ts` (or wherever `buildPaneContentStyle` is already tested — check first), `web/src/__tests__/features/agent/components/agent-terminal-wait-banner.test.tsx`.

**Interfaces:** full findings at `.superpowers/sdd/2026-08-31-sidebar-restyle-recovery/task-1-findings.md` — read it in full before starting; the summary above is the load-bearing subset, not a replacement for it.

- [ ] **Step 1: Read `.superpowers/sdd/2026-08-31-sidebar-restyle-recovery/task-1-findings.md` in full.**
- [ ] **Step 2: Delete `drop-rules.ts` and its test per the description above; run the full web test suite to confirm nothing else depended on it.**
- [ ] **Step 3: Write a failing test for the pane gutter** (assert the computed inset/margin on a rendered pane's outer box is 4px on the left/top edges when not adjacent to a window edge), then implement the fix in `pane-border.ts`/`pane-node-renderer.tsx` per the description above, then confirm it passes.
- [ ] **Step 4: Write a failing test for the trust-banner not overlapping the composer** (render the banner at a height/width that would have triggered the 7px overlap Task 1 measured, assert the composer's top is at or below the banner's actual bottom), then implement the dynamic-space fix, then confirm it passes.
- [ ] **Step 5: Run the full web gate:** `bun tsc --noEmit`, `bun vitest run`, lint. Fix anything Steps 2-4 broke.
- [ ] **Step 6: Commit** (one commit per fixed item, following this branch's existing `fix(sidebar):`/`fix(agent):` commit-message convention).

---

## Task 7: Systematic 1:1 parity sweep

**Files:** determined by what the checklist below turns up — expect this task to mostly confirm conformance (per the recovery spec §2's audits) and produce few or no diffs. Do not manufacture changes to "look thorough."

**Interfaces:** none — this is a verification task against the running app, using the Tauri MCP, structured as a checklist.

- [ ] **Step 1: Drive the live app (post Tasks 2-6) through every item in the recovery spec's §4 "Phase 2" checklist**, screenshotting each and comparing against the named design artboard where one exists (`Files.dc.html`, `Git.dc.html`, `Rows.dc.html`, `Naming.dc.html`, `Pane.dc.html`, `Panes.dc.html`, `Twoup.dc.html`, plus `Main.dc.html` itself). Specifically re-verify, live, everything §2 of the recovery spec only confirmed by static code reading (mark each as re-confirmed or newly-broken):
  - No invented 6px trailing-action radius; no oversized fold caret.
  - No second sidebar/accordion/segmented-pill switcher for Files/Git; no head/body divider in the card.
  - No context pill anywhere.
  - Tree and Recents drag are visually and behaviorally identical (drag a row from each, confirm same ghost/threshold/targets).
  - Recents reorders by drag only, never by click.
  - No "Editor" tab reappears in the running app.
  - The busy-drag refusal inerts every other row (opacity ~0.26) and reddens the ghost, live.
  - Git verb copy never restates the source branch.
  - The row-greying from Task 2, the chat trash from Task 3, and the promotion dropdown from Task 4 all work as designed, live, not just in unit tests.
- [ ] **Step 2:** for anything found broken, file it as its own small fix (same pattern as Tasks 2-5: failing test first where practical, real fix, commit) rather than batching unrelated fixes into one commit.
- [ ] **Step 3: Report a final checklist status** (every item ticked or explicitly noted as a filed follow-up) to the controller.

---

## Task 8: Sign-off

**Files:** none (verification only), except updating the recovery spec's status line from "open for review" to closed, with a one-paragraph summary of what shipped vs. what was found to already be fine.

- [ ] **Step 1:** Fresh Tauri screenshots of: rail at rest (dark + light), a chat open in the tree (grey label now works), Recents with all four states populated, the file-explorer card open on Files and Git, a two-up pane split, the busy-drag refusal, and a chat being deleted — each placed next to its corresponding design artboard, matching the recovery spec's §4 Phase 4 exactly.
- [ ] **Step 2:** Confirm every regression test added across Tasks 2-6 still passes, and the full gate (`bun tsc`, `bun vitest run`, lint, react-doctor if this repo runs it) is green.
- [ ] **Step 3:** Update `docs/superpowers/specs/2026-08-31-sidebar-restyle-recovery.md`'s status line and append a closing note under a new `## 5. Outcome` section: what Task 1 found (stale build confirmed/refuted), what Tasks 2-5 fixed, what Task 6 additionally fixed, what Task 7's sweep confirmed was already fine. This is the record the user asked to see when everything is "fixed and working perfectly."
- [ ] **Step 4:** Hand off to `superpowers:finishing-a-development-branch` per the subagent-driven-development skill's own final step.
