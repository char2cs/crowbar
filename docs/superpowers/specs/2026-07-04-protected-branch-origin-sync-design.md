# Protected Branch Origin Sync — Design

**Date:** 2026-07-04
**Status:** Approved (brainstorm), pending spec review → plan

## Goal

A protected/root workspace (e.g. `develop`, `master`) never has its local knowledge of `origin/<branch>` refreshed once it's already provisioned, so its `ahead`/`behind` status silently rots — it can sit behind origin indefinitely while the UI keeps reporting "Up to date". Keep that status honest while a protected workspace's tab is open, using UI and API surface that already exists, so staleness is visible (and actionable via the existing Pull button) before someone forks a child off a stale parent.

## Background — what exists today

- `BranchSection` (`web/src/features/git/components/branch-section.tsx`) + `resolveBranchAction` (`web/src/features/git/lib/branch-action.ts`) already render a `sync-only` state — "Clean · N behind" status line plus a "Pull N" secondary button — for any workspace with no parent branch and `behind > 0`. **No new UI is needed**; this already exists and needs no changes.
- `gitStatus.ahead`/`behind` (consumed via `useGitStore`, rendered by `GitPanel`) come from local `git status --branch`, parsed in `api/internal/engine/git/internal/status/status.go:39`. This only reflects the local `refs/remotes/origin/<branch>` ref — accurate only as long as *something* has fetched recently.
- The only code that ever refreshes that ref (`FastForwardBranch`, `api/internal/engine/git/engine.go:358`) runs at: project import (`project_import.go:639`), worktree retry-provision (`worktree.go:793`), and fork-time when a workspace is used as the parent of a new child (`worktree.go:266`, `worktree.go:295`, both best-effort/fail-open). **None of these fire from simply reopening or continuing to view an already-provisioned protected workspace** — that's the gap this design closes.
- (Adjacent, already fixed separately: the `added`/`deleted` diff badge on protected branches, which read from a `ForkPointSha` frozen at provisioning time, is now hidden for locked branches in `workspace-tree-item.tsx` and `workspace-switcher.tsx`. Unrelated to this design.)
- The file watcher (`api/internal/engine/fs/internal/watch/watcher.go:163-167`) already watches `.git/HEAD`, `.git/refs/*`, and `packed-refs` "so branch switches and remote fetches are detected" — any ref change from a background fetch already flows through the existing recompute-and-push pipeline with no changes needed there.
- The realtime layer already has an established, near-identical precedent for "run background work only while a specific workspace's WebSocket is subscribed": `ProviderPollManager` (`api/internal/app/realtime/provider_poll_manager.go`) is refcounted by `wsId` — the first subscriber starts a ticking goroutine (currently every 1 minute, polling PR/CI status via `gh`), the last unsubscribe stops it — wired through `StreamDef.OnSubscribe`/`OnUnsubscribe` (`api/internal/api/v0/container.go:190-198`, `service.go:95-106`).
- The `git` WebSocket broadcaster already uses that same `OnSubscribe`/`OnUnsubscribe` mechanism for a different purpose today: `withWatcherLifecycle` (`container.go:70`, `161-169`) wires it to `AcquireWatcher`/`ReleaseWatcher`, which starts/stops the filesystem watcher.
- The frontend already opens exactly one `git/status` WebSocket connection at a time, scoped to whatever workspace the current route points at (`git-panel.tsx` derives `wsId` from `useRouterState`). No frontend change is needed to produce an "activation" signal — this connection's lifetime already *is* that signal.

## Architecture

Add a new **`OriginSyncManager`**, structurally mirroring `ProviderPollManager`: refcounted per `wsId`, ticking every 5 minutes (longer than the 1-minute PR poll — origin staleness isn't as time-sensitive). On each tick (including immediately on the first `Acquire`), it loads the workspace and, only if it has no parent (`domain.Workspace.ParentID == ""`, i.e. a protected/root branch), performs a best-effort single-branch fetch (`enginegit.Engine.FetchRef`) — updating only the local `origin/<branch>` tracking ref, never the working tree or local branch ref, so staleness stays visible rather than silently self-healing.

It's wired onto the **existing** `git` WebSocket broadcaster's subscribe lifecycle, composed alongside (not replacing) the current watcher-acquire behavior. This requires **no new REST endpoint and no frontend change** — the trigger is a connection that already exists today for every open workspace tab.

## Components

- **`api/internal/app/realtime/origin_sync_manager.go`** (new) — `OriginSyncManager`, mirroring `provider_poll_manager.go` almost line for line: a refcounted `handles map[string]*originSyncHandle`, `Acquire`/`Release`/`StopAll`, and a `run(ctx, wsID)` loop that ticks every `originSyncInterval = 5 * time.Minute` and, per tick, calls `workspace.Get(ctx, wsID)` then, if `ws.ParentID == ""`, `gitEngine.FetchRef(ctx, ws.WorktreePath, ws.Branch)` on a bounded-timeout, `context.WithoutCancel`-derived context (mirroring `pollTimeout`) — logging (not surfacing) any error. Depends directly on `workspacerepo.Workspace` and `enginegit.Engine`, exactly like `WatcherManager` already does — no new interface needed.
- **`api/internal/app/realtime/service.go`** — add `originSync *OriginSyncManager` field, construct it in `New(...)` alongside `providerPoll`, add `AcquireOriginSync`/`ReleaseOriginSync` methods mirroring `AcquireProviderPoll`/`ReleaseProviderPoll`, and call `s.originSync.StopAll()` from `Close()`.
- **`api/internal/api/v0/container.go`** — a new `withOriginSyncLifecycle[T any](def ws.StreamDef[T], appContainer *app.Container) ws.StreamDef[T]` decorator that **chains** onto whatever `OnSubscribe`/`OnUnsubscribe` are already set (calls the previous hook, then `AcquireOriginSync`/`ReleaseOriginSync`), applied only to the `git:` broadcaster construction (line ~70): `git: ws.NewBroadcaster(withOriginSyncLifecycle(withWatcherLifecycle(gitDef(appContainer), appContainer), appContainer))`.

## Data flow

```
Frontend opens git/status WS for wsId (already happens today, per open workspace tab)
  → Broadcaster.Handle: register client → onSubscribe(scope=wsId)
      → AcquireWatcher(wsId)          [existing, unchanged]
      → AcquireOriginSync(wsId)       [new]
          → OriginSyncManager.Acquire(wsId): 0→1 starts the ticking goroutine
              tick (immediately, then every 5m):
                ws, err := workspace.Get(ctx, wsId)
                if ws.ParentID == "":                      # protected/root only
                    gitEngine.FetchRef(ctx, ws.WorktreePath, ws.Branch)
                    # best-effort — errors logged, never surfaced
  (a successful fetch updates .git/refs/remotes/origin/<branch> or packed-refs)
  → existing file watcher notices the ref change (already watches refs/*, packed-refs)
      → recomputes status → new ahead/behind
      → pushes over the SAME already-open git/status WS
  → useGitStore updates gitStatus.ahead/behind
  → BranchSection re-renders "Clean · N behind" / "Pull N"     [existing UI, unchanged]

Tab closes / navigates away → onUnsubscribe(scope=wsId)
  → ReleaseWatcher(wsId)              [existing, unchanged]
  → ReleaseOriginSync(wsId)           [new] → 1→0 stops the ticking goroutine
```

## Error handling

Best-effort and silent throughout, matching `FastForwardBranch`'s existing philosophy elsewhere in this codebase. A failed `FetchRef` (offline, auth failure, etc.) is logged via `slog.WarnContext` and the manager simply tries again on the next tick — no toast, no error state, since this is background work the user never explicitly requested (unlike the manual Push/Pull buttons in `BranchSection`, which do surface `remoteError`). Each tick runs with a bounded timeout (mirroring `pollTimeout = 30 * time.Second`) so a hung network call can't leak a goroutine or block a subsequent `Release`/shutdown.

## Testing

- **`OriginSyncManager`** (mirroring `provider_poll_manager_test.go`'s shape): `Acquire` starts exactly one goroutine per `wsId` regardless of subscriber count; `Release` on the last unsubscribe cancels it; `Release` before the last is a no-op; `StopAll` is idempotent and stops every live handle; a fake workspace repo + fake `FetchRef` let a test assert the tick actually ran (with a short synthetic interval) and that a `ParentID != ""` workspace never triggers a fetch.
- **Tick behavior**: a protected workspace (`ParentID == ""`) calls `FetchRef` with the workspace's own `WorktreePath`/`Branch`; a non-protected workspace (`ParentID != ""`) never calls it; a `FetchRef` error is swallowed (logged, tick returns normally, next tick still scheduled).
- No frontend tests needed — zero frontend changes.
- Manual/live verification in the real Tauri app per the standing rule: open a protected branch's workspace tab, confirm (e.g. by deliberately staging a local ref behind origin, or watching daemon logs) that "Clean · N behind" appears within one tick interval, and that clicking the existing "Pull N" button resolves it.

## Out of scope (YAGNI)

- Auto-pulling or fast-forwarding the protected branch automatically. Deliberately fetch-only, so staleness stays visible and actionable rather than silently resolving itself.
- Any change to non-protected (has-parent) workspaces — unaffected. Their fork point is already kept accurate by the existing fork-time `FastForwardBranch`, and their own ahead/behind-vs-upstream is already served by `BranchSection`'s existing Push/Pull secondary.
- A configurable poll interval or user-facing setting — `5 * time.Minute` is a hardcoded constant, matching how the provider-poll interval and `pollTimeout` are already hardcoded, not exposed as settings.
- Any change to the diff badge (`added`/`deleted`) mechanism — already fixed separately (hidden for locked branches).

## Open decisions (resolved)

- **Fetch-only, not pull**: keeps staleness visible for the user to act on, rather than silently self-healing on tab-open.
- **Scope**: protected/no-parent workspaces only (`ParentID == ""`). Non-protected workspaces are untouched by this feature.
- **Poll interval**: 5 minutes (vs. the provider-poll's 1 minute) — origin staleness is less time-sensitive than PR/CI status.
- **Hook point**: the existing `git/status` WebSocket subscribe/unsubscribe lifecycle, not a new REST endpoint and not a side effect bolted onto the existing GET handler — keeps read endpoints pure and requires no frontend change.
- **Fetch primitive**: `FetchRef` (single branch), not the blanket `Fetch()` used by the (currently unwired) manual fetch menu action — scoped to exactly the branch this feature cares about.
