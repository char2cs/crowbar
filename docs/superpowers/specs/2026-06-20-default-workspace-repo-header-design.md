# Default Workspace & Repo Header Design

**Date:** 2026-06-20 (revised after codebase review)
**Branch:** refactor/entity-scoped-api-ws

## Problem

When a repo is imported, `adoptOneWorktree` creates a workspace for every git worktree — including the main one (where `worktreePath == repo.Path`). This "original repo folder" workspace shows up in the workspace tree alongside user-created workspaces, which is wrong: it is not a workspace the user created, it is the folder they imported.

Locked branches (protected branches from the remote provider) must continue to appear in the tree as normal. Only the main-worktree workspace is affected.

## Core decision: frontend resolves the default to its real id

The on-disk import folder is the git **main worktree** — the one workspace where `worktreePath == repo.Path`. It is flagged `IsDefault == true` at import time (an unambiguous flag; a `worktreePath == repo.Path` comparison is NOT usable because protected-branch stubs are created with the same path).

A literal `"default"` wsId in the URL was rejected: the IDE builds **every** workspace-scoped URL (files, git, lsp, terminal, review, search, threads, provider — ~9 route groups) from the raw wsId via `workspaceBase(wsId)`, and the per-workspace WS stream matches on `projectId/repoId/<realId>` via prefix match. Resolving `"default"` would require touching all of those plus the broadcaster, and every future scoped route would have to remember to do it.

Instead: **the frontend resolves the default to its real workspace id before navigating.** The repo header reads the `IsDefault` workspace from the entity cache and navigates to `/ide/:projectId/:repoId/<realId>`. Every existing backend route and WS stream then works unchanged because they receive a real id.

### Why not exclude the default on the backend

The sidebar tree is built from one IndexedDB store (`crowbar_workspaces`) fed by the REST list seed **and** the WS stream **and** the per-wsId provider stream that re-seeds when the default is opened. Excluding the default from any one source is self-defeating (another source re-populates the cache) and excluding it from `filterWorkspaces` would also corrupt the merge-eligibility sibling set in `Detail`/`List`. So the backend **includes** the default everywhere carrying `isDefault`, and the **frontend** filters it out of the tree render.

---

## Backend

The default workspace is already created by import. The backend changes are minimal.

1. **Revert `MainBranch`** from `domain.Repository` (added in commit 8a08def, read/written nowhere). The header subtitle comes from the default workspace's own `Branch`, not a denormalized repo field.

2. **Flag the default at import.** In `adoptOneWorktree`, set `IsDefault: wt.Path == repo.Path` on the `workspace.CreateInput`. Protected-branch stubs (`importProtectedBranchStubs`) are never flagged. No worktree pre-fetch / `worktrees[0]` refactor (order-fragile; breaks `TestImport_WorktreeListError_IsTolerated`).

3. **Persist `IsDefault` through the command path.** `workspace.CreateInput.IsDefault` already exists (8a08def) but is dropped. Add `IsDefault` to `commands.CreateWorkspace`, set it on the returned aggregate in `EmitEvent`, and pass `IsDefault: in.IsDefault` in `workspace.Create`'s `SendWait`. The read-model store JSON-marshals the whole `domain.Workspace`, so persistence is then automatic — no storage/schema change. Also copy `IsDefault` in the `mocks.WorkspaceRepo.Create` test double.

4. **Expose `IsDefault` on the wire.** Add `IsDefault bool \`json:"isDefault,omitempty"\`` to `dto.WorkspaceDTO` and map `w.IsDefault` in `WorkspaceDTOFrom`. (The spec's earlier claim that no `WorkspaceDTO` change was needed was wrong.)

**No** changes to `filterWorkspaces`, `workspacesSnapshot`, the detail/create/sync/merge/reparent handlers, or any `"default"` alias. There is no alias.

---

## Frontend

1. **`types.ts`:** add `isDefault?: boolean` to `WorkspaceDTO` (optional — Go omits it when false).

2. **`sidebar.ts` `Repo`:** add `defaultWorkspaceId?: string` and `defaultWorkspaceBranch?: string`.

3. **`build-repo-tree.ts` `toSidebarRepo`:** of the repo's workspaces, find the one with `isDefault` → set `defaultWorkspaceId` and `defaultWorkspaceBranch` (its `branch`); **exclude** it from `Repo.workspaces`. This is the single chokepoint (`buildRepoTree` → `toSidebarRepo`, used by both `workspace-list.ts` and `sidebar-sync.ts`), so the render code needs no filter.

4. **`workspace-tree.tsx` repo header:**
   - **Click** the avatar+name → navigate to `/ide/:projectId/:repoId/:defaultWorkspaceId` (no-op if not yet known).
   - **Hover subtitle** → `defaultWorkspaceBranch` beneath the name.
   - **`+` button** → `startCreating(repo.id, defaultWorkspaceId)` (the existing inline child-create flow). Because the default has no tree row, render the inline branch-name input at **repo level** (under the header, above the roots) when `creatingChildOf.repoId === repo.id && creatingChildOf.parentId === defaultWorkspaceId`. The new child forks from the default workspace's branch and, since its parent is excluded from the tree, renders as a top-level row.
   - **Chevron** → `toggleRepo` (collapse/expand), now a separate control.
   - **Active state** → highlight the header when the route's active wsId === `defaultWorkspaceId`.
   - Preserve the existing avatar rendering, `data-repo-drop` drag target, and Settings button.

5. **`workspace-route-guard.ts`:** the default's real id is excluded from `Repo.workspaces`, so navigating to it would be treated as unknown. `shouldRedirectUnknownWorkspace` must also accept any repo's `defaultWorkspaceId`. (No `"default"` literal handling — we never navigate to that string.)

---

## Data flow (repo header click)

```
Import: adoptOneWorktree creates the main-worktree workspace with IsDefault=true
  → persisted via command path → exposed on WorkspaceDTO.isDefault
Frontend: list seed + WS stream populate crowbar_workspaces (incl. the default)
  → toSidebarRepo pulls the isDefault ws out: Repo.defaultWorkspaceId + .defaultWorkspaceBranch
  → tree renders WITHOUT the default row
User clicks "crowbar" header
  → navigate to /ide/p/r/<defaultWorkspaceId>   (a REAL uuid)
  → route guard accepts it (matches repo.defaultWorkspaceId)
  → IDE + all files/git/lsp/terminal/WS use the real id → everything works
```

---

## Edge cases

- **Main worktree on a protected branch:** `adoptOneWorktree` marks it adopted (so `importProtectedBranchStubs` skips a duplicate) AND flags it `IsDefault`. One workspace row exists; it is pulled out as the default and not shown as a protected row. Correct — the default workspace *is* that branch.
- **Fork from default:** the child's `parentId` is the default's real id; the default is excluded from the tree, so `buildWorkspaceTree` makes the child a root (top-level row under the repo header). Correct.
- **Merge eligibility:** `filterWorkspaces` is unchanged, so the default remains in the sibling set used by `Detail`/`List` for eligibility. No corruption.
