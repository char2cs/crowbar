# Workspace Data Layer & Git Lock Granularity — Design

**Date:** 2026-07-04
**Status:** Approved (brainstorm), pending spec review → plan

## Goal

Opening or interacting with the Workspaces sidebar tree intermittently hangs or tanks app performance. Give the workspace data layer a real scoped query path (instead of scanning every workspace in the whole install on every request) and split the git engine's per-repo lock so a background network fetch can't block a concurrent local read — without changing the per-entity event-sourced storage model that makes workspace deletion a clean `rm -rf`.

## Background — what exists today

- Each workspace is its own event-sourced aggregate: a separate `event_stream.db` + `view.db` per workspace under `<home>/projects/<P>/<R>/workspaces/<W>/storages/`, opened lazily and cached in a ref-counted LRU (`adapter.Container.WorkspaceES`/`WorkspaceView`, `api/internal/adapter/container.go:185-240`), capped at `maxOpenEntityDBs = 64` (`api/internal/adapter/registry.go:9`). This is deliberate — deleting a workspace just removes its own directory (`workspace.go:687-707`), never touches shared rows. Only chat, reviewthread, projects, repos, terminal profiles, and settings live in one shared **global** `view.db`.
- `workspace.List()` (`api/internal/app/repositories/workspace/workspace.go:711-733`) is the *only* listing primitive, and it works by enumerating every location and opening (or LRU-hitting) each workspace's own per-entity `view.db` one at a time (`readRow`, `workspace.go:765-778`). Past 64 total workspaces in the install, this thrashes real SQLite file opens on every single call — a plausible source of the *intermittent* feel (fast when the LRU's warm, slow when it's evicted).
- Three separate call sites all discard the scope they're given and pay this full-install cost on every request:
  - `workspacesSnapshot` (`api/internal/api/v0/snapshots.go:23-42`) calls the unscoped `ListWorkspaces`, then filters to a repo via `scopeWorkspacesToRepo` (`snapshots.go:67`) — filtering happens *after* the expensive part.
  - `gitSnapshot` (`snapshots.go:172-184`) has the signature `func(_ string)` — it explicitly discards its scope, calls the unscoped `Workspace.List`, then runs a real `git status` subprocess (`appendGitStatus` → `Usecases.Git.Status`, `snapshots.go:189-201`) for **every workspace in the entire install**, on every single `git/status` WebSocket subscribe (i.e. every time any workspace tab opens).
  - `lspSnapshot` (`snapshots.go:211-230`) has the identical pattern for diagnostics.
- `repositories.Container.eligibilityFor` (`api/internal/app/repositories/container.go:153-171`) calls the full unscoped `Workspace.List` (via `ListWorkspaces`, `container.go:173-187`) just to find one sibling row, on every broadcast of a workspace that has a parent. `ResolveMergeEligibility` (`api/internal/app/usecases/workspace/merge_eligibility.go:37-71`) itself is cheap (one linear scan + one `WouldMergeConflict` dry-run) — the full-install list is the actual cost.
- The global `view.db` is opened via `storesqlite.OpenDB` (`api/internal/adapter/store/sqlite/sqlite.go:33-56`), which already sets `PRAGMA journal_mode=WAL` and `busy_timeout=5000` — but also hardcodes `SetMaxOpenConns(1)`, forcing every reader and writer through one connection regardless of WAL's concurrent-reader support. The comment says this was to avoid `SQLITE_BUSY` on DDL during a specific crash-recovery test scenario, not a deliberate production concurrency choice.
- The git engine's per-repo lock (`engine.mu sync.Map` of `*sync.Mutex`, keyed by canonicalized git-common-dir so all worktrees of one clone share it — `api/internal/engine/git/engine.go:28-44,74-78`) is only taken by *mutating* methods: `StageFile/Hunk`, `Unstage*`, `Discard`, `Commit`, `Push`, `Fetch`, `FetchRef`, `FastForwardBranch`, `Pull`, `Reset`, `Merge`, `Rebase`, `OperationContinue/Abort` (`engine.go:267-573`, each with `defer e.lockRepo(ctx, repoPath)()`). Read-only methods — `Status`, `Diff`, `CommitDiff`, `Log`, `Blame`, `Branches`, `Stashes` (`engine.go:213-260`), `ConflictedFiles`, `ConflictHunks`, `WorkingTreeSummary`, `ComputeStatus`, `ComputeWorkingTreeSummary` (`engine.go:535-611`), and `WouldMergeConflict` (`would_merge_conflict.go:21`) — take **no lock at all** today. Network ops (`Fetch`/`Pull`/`Push`/`FetchRef`) already run under a bounded timeout (3 min transfer / 30 s query, `engine.go:90-99`) so they can't wedge forever, but they still hold the mutex for that whole bounded duration, serializing every other mutating op on that clone — including the newly-added `OriginSyncManager`'s periodic background `FetchRef` (`api/internal/app/realtime/origin_sync_manager.go`, wired via `withOriginSyncLifecycle` onto the `git:` broadcaster, `api/internal/api/v0/container.go:70,190-210`).
- This is not the menu's primary cause (reads take no lock today, so they're never blocked by a concurrent write) but it is a real, adjacent gap: reads can currently observe a git state mid-mutation with no synchronization at all, and any *write* (stage/commit/push) on one worktree can now get stuck for up to 30s behind an origin-sync fetch running for a different worktree of the same clone.

## Architecture

Add one new query-only projection table, `workspace_directory`, to the *existing* global view.db — not a new database, not a new storage subsystem. It holds exactly the fields the list/tree/sibling-matching call sites need (`id, project_id, repo_id, parent_id, branch, status, kind, worktree_path`). The per-entity `event_stream.db`/`view.db` pair remains the sole source of truth for a workspace's full state; this table is derived, rebuildable, and only ever read through a repo-scoped (or project-scoped) indexed query.

It's kept in sync the same way every workspace change already reaches the outside world today: `repositories.Container.broadcastWorkspace` (`container.go:80-87`) is the one function every create/update/delete already funnels through before pushing to the hub. It gains one more step there: upsert the row (or delete it, on the `WorkspaceStatusDeleted` tombstone).

Separately and independently, the git engine's per-repo `sync.Mutex` becomes a `sync.RWMutex`: read-only methods take `RLock`, mutating methods keep taking the exclusive `Lock` they already take today.

## Components

- **`api/internal/app/repositories/workspace` — `workspace_directory` projection.** New GORM model + migration on the global view.db. New method `ListInRepo(ctx, projectID, repoID string) ([]domain.Workspace, error)` — one indexed `WHERE project_id = ? AND repo_id = ?` query. The existing `List(ctx)` stays for the rare truly-global caller (none identified today beyond `GetHomeForProject`, which itself could move to a project-scoped query later — out of scope here). Write side: `broadcastWorkspace` upserts/deletes the row; failure is logged and swallowed (matches the existing best-effort broadcast philosophy) — it never blocks or fails the underlying mutation, since the per-entity store already committed by the time this runs.
- **`RebuildDirectory(ctx) error`** (new, same package) — runs today's existing full per-entity scan once and repopulates the table from scratch. Used for the one-time backfill migration when this ships, and safe to invoke anytime afterward as a recovery action since the table is fully derived from the per-entity stores.
- **`api/internal/api/v0/snapshots.go`** — `workspacesSnapshot`, `gitSnapshot`, `lspSnapshot` all switch from "unscoped List, then filter" to calling `ListInRepo` with the `projectID`/`repoID` already parsed from the subscription scope (`parseRepoScope`, `snapshots.go:48-65`, unchanged). `gitSnapshot`'s signature changes from `func(_ string)` to actually using its scope parameter.
- **`api/internal/app/repositories/container.go`** — `eligibilityFor` (`container.go:153-171`) calls `ListInRepo(ctx, ws.ProjectID, ws.RepoID)` instead of the full `ListWorkspaces`.
- **`api/internal/adapter/store/sqlite/sqlite.go`** — `OpenDB` gains a parameter (or a second constructor) so the global view.db can request a larger pool (e.g. 8) while per-entity workspace DBs keep `SetMaxOpenConns(1)` (they're effectively single-tenant — one workspace, one or two open tabs at most).
- **`api/internal/engine/git/engine.go`** — `mu sync.Map` now stores `*sync.RWMutex`; `repoMutex`/`lockRepo` return an `RLock`-capable variant. Add `lockRepoRead(ctx, repoPath) func()` (RLock) alongside the existing `lockRepo` (Lock, unchanged name/behavior for writers). Add `defer e.lockRepoRead(ctx, repoPath)()` to `Status`, `Diff`, `CommitDiff`, `Log`, `Blame`, `Branches`, `Stashes`, `ConflictedFiles`, `ConflictHunks`, `WorkingTreeSummary`, `ComputeStatus`, `ComputeWorkingTreeSummary`, and to `WouldMergeConflict` (`would_merge_conflict.go:21`).

## Data flow

```
Tree/tab opens with a project+repo scope (already true today — parseRepoScope
already derives it, it was just discarded downstream)
  → workspacesSnapshot / gitSnapshot / lspSnapshot call ListInRepo(project, repo)
      → one indexed SQL query against workspace_directory (global view.db,
        now multi-reader) — no per-entity SQLite opens, no LRU thrashing

Any workspace event (create/update/status change/delete)
  → broadcastWorkspace (unchanged entry point)
      → per-entity store write (authoritative, unchanged)
      → hub broadcast (unchanged)
      → workspace_directory upsert/delete   [new step, same function]

git op on repo R
  → read (Status/Diff/Log/...)   → RLock  → runs concurrently with other reads
  → write (Commit/Push/Fetch/...) → Lock   → exclusive, same timeouts as today
  (origin-sync's periodic FetchRef no longer has any effect on concurrent reads)
```

## Error handling

- `workspace_directory` write failure: logged via `slog.WarnContext` and swallowed — the per-entity store is already the durable source of truth, so a projection write failure never fails the caller's actual mutation. A drifted/missing row just means that one workspace is briefly invisible to list views until the next event or a `RebuildDirectory` run; it can never produce stale-but-wrong data for reads that fall through to a workspace's own per-entity store directly (e.g. `Get(id)`), since those are unaffected by this change.
- `ListInRepo` returning zero rows for a repo that does have workspaces is a signal to run `RebuildDirectory`, not a case the read path needs to special-case — it's a recovery/ops action, not user-facing.
- RWMutex: no new failure modes. Same context-cancellation and timeout semantics as today; only the blocking relationship between concurrent callers changes (reads no longer wait on writes at all when uncontended, and now observe a consistent pre-/post-write state instead of an unsynchronized one).

## Testing

- `workspace_directory`: unit tests for upsert-on-broadcast, delete-on-tombstone, `ListInRepo` repo-isolation (a row from repo B never appears in repo A's result), and `RebuildDirectory` producing rows identical to a fresh full per-entity scan.
- Regression tests asserting `gitSnapshot`/`lspSnapshot` only touch workspaces in the requested scope — call-count assertion against a fake git engine / LSP host, proving a subscribe to workspace W's `git/status` no longer triggers a `Status` call for every other workspace in the install.
- Existing WS scope-isolation integration tests (`TestV0_PushGit_QueryScope_IsolatesWsId`, `TestV0_GitDualServe_PathScope_IsolatesWsId`) must keep passing unchanged.
- Global view.db pool: the crash-recovery test that originally motivated `SetMaxOpenConns(1)` must be re-run at the new pool size to confirm WAL + `busy_timeout` alone are sufficient (flagged for the implementation plan to locate and verify, not re-derived here).
- Git RWMutex: a race-detector test with two goroutines confirming concurrent `Status` calls never block each other, and a `Status` call started while a `Fetch` is in flight observes either the fully-pre-fetch or fully-post-fetch ref state, never a torn read — same fakes-and-channels style as `origin_sync_manager_test.go` (no sleeps).

## Out of scope (YAGNI)

- Replacing per-entity event sourcing for workspaces with a single shared store. The per-entity model's `rm -rf`-on-delete property is intentional and unrelated to this fix; `workspace_directory` is purely an additive index.
- A per-worktree (finer than per-clone) git lock. RWMutex fixes the actual observed contention (reads-vs-writes, write-vs-write with origin-sync); splitting further would need auditing every git subcommand for which shared-clone state it touches, for a benefit not established by this investigation.
- Any change to the `OriginSyncManager` feature itself (interval, scope, fetch primitive) — it's out of scope; this design only changes what it contends with.
- Scoping `GetHomeForProject` or other rare global callers to a repo/project query — not on the hot path this investigation covers.

## Open decisions (resolved)

- **Projection storage**: persisted in the existing global view.db (not in-memory-only), so a daemon restart never pays a cold full-install scan and the pattern matches how projects/repos/settings already live there.
- **Git lock shape**: `RWMutex` (reads vs. writes), not a finer per-worktree/per-clone split — simplest primitive that fixes the actual contention found, lowest risk of misclassifying a git subcommand's shared-state footprint.
- **Scope**: this spec covers the workspace read path (List/Detail/snapshot builders + merge-eligibility) and the git engine lock. It does not re-open the frontend "destroy-everything" workspace-switch model or the terminal/editor rehydration cost — those are tracked separately (see `workspace-switch-latency` investigation notes) and are independent of this fix.
