# Safe Pull + Conflict Surfacing — Design

**Date:** 2026-07-09
**Branch:** `fix/safe-pull-conflict-surfacing` (off `develop`)
**Status:** approved (design), pending implementation

## Problem

A user clicked **Pull** on a branch that had diverged from its origin upstream
(local was 2-ahead / 72-behind `origin/<branch>`). The daemon ran
`git pull --no-rebase` — a blind merge of two divergent histories — which
conflicted and left the worktree **wedged** (`MERGE_HEAD` + unmerged paths),
with **no UI indication** whatsoever.

### Root cause (verified)

- **Bug A — unsafe merge, no rollback.** `git pull` runs through the git
  usecase `mutate` wrapper (`api/internal/app/usecases/git/git_write.go:142`)
  with no safety gate (no `--ff-only`, no pre-check) and no rollback: on a
  conflict, `mutate` returns the error *before* the resync and never calls
  `OperationAbort`, so `MERGE_HEAD` is left in place. `engine.Pull`
  (`api/internal/engine/git/engine.go:424`) uses `--no-rebase`.
  `WouldMergeConflict` exists but only feeds a read-only UI overlay.
- **Bug B — conflict never surfaced.** The only thing that flips a workspace to
  `pr-conflicts` is `SyncWorkingTreeState`
  (`.../workspace/internal/commands/sync_working_tree_state.go:57`). It never
  fired because (1) `mutate` returned before calling it; (2) the FS watcher's
  passive recompute early-returns while `MERGE_HEAD` exists
  (`isRewriteInProgress`, `api/internal/engine/fs/internal/watch/watcher.go:361`);
  (3) the dedicated `OperationInProgress` detector is dead code. The frontend is
  fine — `web/src/components/layout/workspace-branch-icon.tsx:40` already renders
  an amber warning for `status==='pr-conflicts'`; nothing ever set it.

## Desired behavior

1. **Safe-only Pull.** Crowbar performs a pull only when it fast-forwards. If it
   cannot fast-forward, it does **not** merge — it refuses and informs the user.
   The tree is never wedged by a pull.
2. **Refusal is surfaced immediately** via an **inform-only modal** (strict
   cossui `Dialog`).
3. **Actual conflict states** (from explicit Merge/Rebase, or a `git merge` run
   in the integrated terminal) always light up the existing workspace warning
   icon (`pr-conflicts`).

## Design

### Backend (`api/`)

**B1 — Pull refuses synchronously; executes ff-only.**
`Handlers.Pull` (`api/internal/api/v0/endpoints/git/handlers/write.go:249`),
*before* `WriteAccepted`: determine whether the local branch is ahead of its
upstream `origin/<branch>` (a local, no-network `rev-list --count
origin/<branch>..HEAD`, reusing the existing ahead/behind machinery in
`engine/git/internal/status`). A pull can fast-forward iff `ahead == 0`.
- `ahead > 0` → **HTTP 409** with a stable machine-readable code
  `not_fast_forwardable` (via the existing error envelope). Start nothing.
- `ahead == 0` → `202` + async pull, and change `engine.Pull`
  (`engine.go:424`) from `--no-rebase` to **`--ff-only`** so even a
  check-then-fetch race can only fast-forward or fail cleanly (never merge).

New git-engine surface: a method to report ff-ability of the current branch vs
its upstream, e.g. `PullIsFastForward(ctx, repoPath, branch) (bool, error)`
(local-only; no network). Reuse `ErrNonFastForward` /
`ErrRejectedNonFastForward` sentinels where sensible.

**B2 — Conflicts always surface (Bug B).**
- `mutate` (`git_write.go:142`): on op error, still run `SyncWorkingTreeState`
  before returning the error, so an explicit Merge/Rebase that conflicts flips
  the workspace to `pr-conflicts`.
- FS watcher (`watcher.go:361`): stop suppressing the *conflict summary* while
  `isRewriteInProgress`. Keep the per-file identical-frame storm guard, but
  still compute + broadcast `HasConflicts` during a rewrite so conflicts created
  outside Crowbar (terminal `git merge`) become visible.

### Frontend (`web/`)

**F1 — Inform-only cossui modal.** New `pull-conflict-modal.tsx` mirroring
`web/src/components/layout/detach-holder-modal.tsx` exactly: `Dialog`,
`DialogPopup`, `DialogHeader`, `DialogTitle`, `DialogDescription`,
`DialogFooter` from `@/components/ui/dialog`, a single primary "Got it" button,
CSS-variable tokens only. Backed by a tiny zustand store
`use-pull-conflict-modal-store.ts` (shape mirrors `useDetachModalStore`:
`target: { wsId, branch } | null`, `open(target)`, `close()`). Mounted once at
the layout root next to `DetachHolderModal`.

**F2 — Wiring.** `gitRemoteOp`/`pullChanges`
(`web/src/features/git/api/git-remotes-api.ts`) must surface the `409` code so
callers can distinguish it (extend `GitRemoteActionResult` with an optional
`code`). The two Pull triggers — `branch-section.tsx` `runRemote('pull')` and
`git-actions-menu.tsx` `handlePull` — open the modal when the result code is
`not_fast_forwardable`, instead of a generic error toast.

**F3 — Icon.** No change. `workspace-branch-icon.tsx:40` already renders the
warning for `pr-conflicts`; the B2 fixes ensure the status is set.

## Contract (frozen — both sides depend on it)

- Refused pull: **HTTP 409**, error code string **`not_fast_forwardable`**,
  carried in the existing error envelope so the FE can read it.
- ff-able pull: unchanged (`202 Accepted`, async), now `--ff-only`.

## Testing

- **Backend** (black-box `TestRegression_*` in `api/tests`, plus focused unit
  tests). **No timing** — block on asynx signals (WaitPublish/WaitQuiescent),
  channels; never sleeps/Eventually. Cases: diverged→409
  `not_fast_forwardable`; ff-able→202; a Merge/Rebase conflict flips status to
  `pr-conflicts` via `mutate`'s error path; watcher emits `HasConflicts` while a
  merge is in progress.
- **Frontend** (`web/src/__tests__/` mirror, `@/` imports): `pullChanges`
  surfaces the `not_fast_forwardable` code; the Pull handler opens the modal on
  that code (and still toasts other errors); the modal renders header/body/
  footer.

## Non-goals (YAGNI)

- Explicit **Merge/Rebase** endpoints keep leaving conflicts for interactive
  resolution — we only make them *visible*, not refuse them.
- No new persistent "diverged-from-origin" workspace status — the modal covers
  the refusal, the icon covers real conflicts.
- No change to the agentic-bridge feature integration (separate concern).

## Verification (definition of done)

- `go build ./...`, `go test` (unit + `-race` where used) and lint green in
  touched packages; `bun run` typecheck + FE tests green.
- **Live** on the dev Tauri app (`make dev-desktop`, never prod): reproduce a
  diverged branch, click Pull → inform-only modal appears, tree stays clean; a
  real conflict state shows the amber warning on the workspace icon.
