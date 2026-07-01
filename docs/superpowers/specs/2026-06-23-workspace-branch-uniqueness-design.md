# One Workspace Per Branch — Design

**Date:** 2026-06-23
**Status:** Approved (pending spec review)

## Goal

Enforce that a repository has **at most one non-deleted workspace per branch**, and
give the user clear, immediate feedback (live validation + offer to open) when they
try to create a workspace on a branch that already has one — including the default
branch.

## Background

Crowbar imports the user's cloned repo folder as the repository's **default
workspace**, surfaced on the repo header (not a tree row). It adopts the repo's
main worktree; clicking the repo header opens it. Child workspaces are separate git
worktrees, each on its own branch.

Git allows a branch to be checked out in at most one worktree, so "one workspace per
branch" is git-natural. A field bug (`2026-06-23`) showed duplicate `develop` rows:
`CreateChild` re-adopted the main worktree on every `POST {branch: defaultBranch}`
with no parent, persisting phantom duplicate default-branch workspaces. A first fix
(`20e29c4`) guarded only the adopt path. This design generalizes the invariant to
**all** branches with clean errors and a good create UX.

## Non-goals

- **Branch rename.** There is no backend rename path today (`renameWorkspace` in the
  sidebar store is local-only and does not persist; the WS DTO's branch wins on
  reload). The invariant is therefore enforced **at create time only**. A persisted
  rename feature, if ever added, must re-check the invariant — out of scope here.
- Changing how the default workspace is displayed (header vs row).
- Replacing random workspace ids with `UUIDv5(repo, branch)` — considered and
  rejected because it couples identity to the branch name, which fights any future
  rename and the many entities keyed on the id (chats, review threads, branch
  reviews, files, LSP diagnostics, terminal sessions).

## Invariant

> For a given `repoId`, at most one workspace with `Status != deleted` may hold a
> given `branch`.

Identity stays a random v4 UUID (`uuid.NewString()`). Uniqueness is a separate
invariant enforced in the create path, NOT derived from the id.

## Backend

### Sentinel

Rename `worktree.ErrDefaultWorkspaceExists` → **`worktree.ErrBranchWorkspaceExists`**
(`"usecases: a workspace already exists for this branch"`). It now covers every
branch, not just the default. Keep the HTTP 409 mapping in `internal/api/libs/status.go`
(update the symbol name).

### Usecase guard (the invariant's home)

In `worktree.CreateChild` (`internal/app/usecases/worktree/worktree.go`), before any
git work and on **both** paths (adopt-main-worktree and create-child), reject when a
non-deleted workspace already holds `(in.RepoID, in.Branch)`:

```go
exists, err := u.branchWorkspaceExists(ctx, in.RepoID, in.Branch)
if err != nil {
    return domain.Workspace{}, err
}
if exists {
    return domain.Workspace{}, fmt.Errorf("%w (repo %s, branch %q)", ErrBranchWorkspaceExists, in.RepoID, in.Branch)
}
```

Generalize the existing `mainWorktreeAdopted` helper into `branchWorkspaceExists`,
matching on `RepoID` + `Branch` (not `WorktreePath`), skipping
`WorkspaceStatusDeleted`. This replaces the raw `git worktree add` "branch already
exists" failure on the child path with a clean, mapped 409, and keeps the adopt-path
guard.

Placement: at the top of `CreateChild`, after the `RepoPath == ""` virtual-repo
short-circuit, so it precedes both the adopt branch and the worktree-add branch.

### Synchronous handler pre-check (UX fallback)

Creation is async (`202` then `CreateChild`), so a usecase error is currently
swallowed (`broadcastLastError` with a blank wsID). To return a real **409** when
live validation is bypassed or two clients race, add the same `(repoId, branch)`
check **synchronously** in the create handler
(`internal/api/v0/endpoints/workspaces/handlers/crud.go`, in `buildCreateInput` or
`Create`) before `WriteAccepted`, mapping via `StatusAndMessage` → 409. The usecase
guard remains the authoritative backstop.

The handler needs to list the repo's workspaces; it already resolves the repo via
`h.repos`. If it lacks a workspace lister, thread the existing workspace usecase
`List` (filter by repoId) into the handler.

## Frontend

### Expose the default branch

Add `defaultBranch?: string` to the sidebar `Repo` type (`web/src/lib/store/sidebar.ts`)
and populate it in `toSidebarRepo` (`web/src/lib/store/build-repo-tree.ts`) from the
default workspace's branch (the workspace where `isDefault`), alongside the existing
`defaultWorkspaceId`. This is what lets live validation catch a collision with the
default branch (whose workspace is filtered out of `repo.workspaces`).

### Live validation in the create input

`WorkspaceInlineInput` (`web/src/components/layout/workspace-inline-input.tsx`) gains
the repo's existing branches and a resolver. As the user types, trim the value and
compare exact, case-sensitive, against:

- every `repo.workspaces[].branch` (excluding deleted), and
- `repo.defaultBranch`.

On a match:

- render a subtle inline hint: **`'<branch>' already has a workspace — open it`**,
- **disable confirm** (Enter and blur-confirm no-op while the name collides),
- the hint text is the **clickable action**: clicking it cancels the create and
  navigates to the matching workspace (`/ide/$projectId/$repoId/$wsId`).

The matching workspace's id comes from the repo's workspace list; for a default-branch
match it is `repo.defaultWorkspaceId`.

### Wiring

The create flow lives in `workspace-tree-context.tsx` (`creatingChildOf`,
`confirmCreate`). Pass the active repo's branch→workspaceId map (including the
default) into `WorkspaceInlineInput` where it renders (in `workspace-tree.tsx` for the
repo-level create and `workspace-tree-item.tsx` for the per-workspace create), plus an
`onOpenExisting(wsId)` callback that navigates and clears the create state.

## Data flow

1. User clicks "Add child workspace" → inline input opens.
2. On each keystroke, the input checks the trimmed value against the repo's branches.
   - **No match:** normal create; Enter → `confirmCreate(branch)` → `POST .../workspaces`.
   - **Match:** confirm disabled; hint shown; clicking the hint navigates to the
     existing workspace.
3. If a collision slips through (race / programmatic), the handler returns 409 and the
   usecase guard rejects, so no duplicate is ever persisted.

## Error handling

- Empty branch: already rejected with 400 (`"branch is required"`) — unchanged.
- Duplicate branch (any, incl. default): 409 `ErrBranchWorkspaceExists` from the
  handler (sync) and the usecase (backstop).
- Deleted prior workspace on the same branch: **allowed** (re-create), since the
  guard skips `WorkspaceStatusDeleted`.

## Testing

**Backend unit** (`worktree_test.go`):
- `CreateChild` rejects a duplicate on the **adopt** path (default branch already
  adopted) — no row persisted, no git work.
- `CreateChild` rejects a duplicate on the **child** path (non-default branch already
  has a workspace) — rejected before `git worktree add`.
- A `deleted` prior workspace does NOT block re-create.
- Existing adopt tests updated for the new `List` call.

**Backend unit** (`status_test.go`): `ErrBranchWorkspaceExists` → 409.

**Backend integration** (`tests/regressions_test.go`): extend
`TestRegression_DuplicateDefaultBranchWorkspace` (or add a sibling) to also POST a
**non-default** duplicate branch and assert no second workspace appears + a 409 on the
sync path.

**Frontend unit:** the branch-exists predicate — matches an existing child branch,
matches the default branch, is case-sensitive, ignores deleted, scopes to the repo.

**Frontend component** (`workspace-inline-input`): typing an existing branch shows the
hint and disables confirm; clicking the hint fires `onOpenExisting` with the right
wsId; typing a free branch keeps confirm enabled.

## Files touched

- `api/internal/app/usecases/worktree/worktree.go` — generalize guard + helper.
- `api/internal/app/usecases/worktree/worktree_errors.go` — rename sentinel.
- `api/internal/api/libs/status.go` — 409 mapping symbol.
- `api/internal/api/v0/endpoints/workspaces/handlers/crud.go` — sync pre-check.
- `api/internal/app/usecases/worktree/worktree_test.go`, `api/internal/api/libs/status_test.go`, `api/tests/regressions_test.go` — tests.
- `web/src/lib/store/sidebar.ts`, `web/src/lib/store/build-repo-tree.ts` — `defaultBranch`.
- `web/src/components/layout/workspace-inline-input.tsx` — live validation + hint.
- `web/src/components/layout/workspace-tree-context.tsx`, `workspace-tree.tsx`, `workspace-tree-item.tsx` — wiring + `onOpenExisting`.
- Frontend tests alongside the above (mirror structure under `web/src/__tests__/`).
