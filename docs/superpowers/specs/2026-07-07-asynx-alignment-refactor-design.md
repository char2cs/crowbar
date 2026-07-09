# Asynx-Alignment Backend Refactor — Design Spec

> **Status:** Design spec — approved in brainstorming, pending implementation plan.
> **Branch:** `fix/unforseen-backend-crash` (base: `develop`).
> **Goal:** A production-ready, **quiver.core-faithful** backend that uses `asynx` the way it is designed to be used. Strong, production-grade code.
> **Approach:** **Big-bang.** No migration code, no deprecated shims, no dead code paths anywhere in the repo. The old model is deleted, not layered over.
> **Data:** The on-disk `.crowbar` layout changes incompatibly. That is accepted — production `~/.crowbar` will be wiped when this ships. **No migration is written.**

---

## 1. Why

### 1.1 The observed failure
Production daemon "crashes for no reason." Forensics (`~/.crowbar/logs/daemon.log`) proved it is **not** a Go panic: the Tauri app's **watchdog SIGQUITs a starved daemon** after 3 missed 10s health probes, then auto-respawns it (~29s later). At SIGQUIT the process was **fully idle** — nothing stuck in git/SQL/handlers — i.e. ~30s of *not being scheduled* = CPU/memory starvation under heavy local dev load (developing Crowbar in Crowbar).

### 1.2 The root architectural cause of the pressure
The daemon runs **one `asynx` instance per workspace *instance***, not per aggregate *type*:
- Each open workspace resolves its own `event_stream.db` + `view.db` (per-entity SQLite), via an LRU `Registry` (`api/internal/adapter/container.go`), and builds an `asynx.Asynx[Workspace]` over that per-entity store.
- Each `asynx` instance spins **8 shards × 8 workers + 8 dispatchers = 72 goroutines**. Only **one** of the three LRU registries — the workspace-*entity* registry (`workspace.go` `entities`) — holds asynx instances, so it alone sets the goroutine ceiling: a full 64-entry LRU × 72 = ~4600 goroutines, already **thousands**; ~700 were live in the crash dump. The other two registries (the adapter's `workspaceES` event-store handles and `workspaceView` read-model handles, `container.go:143-144`) run no asynx — they amplify **SQLite/GORM DB handles**, not goroutines.
- All that per-instance parallelism is **wasted**: the workspace repo already serializes every command per-aggregate with a hand-rolled `writeMu` (single-writer-per-aggregate) — reimplementing exactly what asynx's shard routing already guarantees.

**asynx is not at fault; our usage is.** `asynx` is built to be **one instance managing many aggregate ids**, sharded by `aggregateID` (FNV consistent hash). quiver.core uses it correctly (one instance per aggregate *type*, ~4 total, fixed). Crowbar fragmented it to one-per-instance and then rebuilt the shard behavior by hand.

### 1.3 What this refactor fixes vs. does not
- **Fixes:** collapses the per-instance goroutine/DB-handle explosion to a **fixed handful per aggregate type**, removing the starvation pressure that trips the watchdog. Aligns Crowbar with quiver's battle-tested asynx patterns.
- **Non-goals (explicitly out of scope — separate Rust-side follow-ups):**
  1. The **watchdog aggressiveness** (probe timeout / strike count) — `desktop/src-tauri/src/sidecar/*`.
  2. The **sidecar stderr observability hole** — `desktop/src-tauri/src/sidecar/mod.rs:139` still drops `_rx` (a Go panic in a packaged build leaves no trace). Separate fix.
  3. **Frontend rebuild.** This is a backend-only refactor. The FE keeps consuming the existing WS/hub. No FE feature changes; **no FE functional regressions allowed**.

---

## 2. Locked decisions (invariants the implementation MUST satisfy)

These are the agreed constraints. The reviewer loop checks the spec and implementation against this list.

1. **One `asynx` instance per aggregate TYPE**, held as an eager singleton for the process lifetime. No per-entity instances, **no `Registry`/LRU for asynx or event stores**, **no `writeMu`**.
2. **Aggregate types = {Workspace, ReviewThread}.** **Chat is deferred** (explicitly out of scope for this production milestone; its aggregate/read-model is not wired now). Projects, repos, terminals, settings, terminal-profiles remain **non-event-sourced** plain rows.
3. **Central per-type storage**, quiver-faithful two planes:
   - Write side (event logs): `<HOME>/state/events/<type>.db`
   - Read side (projections): `<HOME>/state/store/<type>.db`
   - Non-aggregate reference plane: `<HOME>/state/view.db` — the plain-CRUD GORM rows: projects, repos, terminal profiles, terminal **sessions**, and the workspace id↔path index (the four stores are exactly `Projects`/`Repositories`/`TerminalProfiles`/`TerminalSessions`, per `api/internal/app/gorm.go:15-18`). **Settings are non-event-sourced too but are NOT a view.db table:** they stay the plain `config.yaml` file at the home root (read via `core/config` → `metadata.GetConfigPath()`), exactly as §3.2 specifies and as the current code already does (there is no settings store, confirmed in `gorm.go`). This refactor does **not** relocate settings into view.db — that would be out-of-scope new work and an FE-visible change; the intent of "settings" in decisions 2/3 is *non-event-sourced*, a property `config.yaml` already satisfies. §3.2 is authoritative for the physical view.db schema the reviewer loop checks against.
     - **RATIFIED by decision owner (2026-07-07):** "settings" in decisions 2/3 means **non-event-sourced**, a property the plain `config.yaml` at the home root already satisfies. Settings are **NOT** a view.db table and are **not** relocated by this refactor (relocating them would be out-of-scope new work + an FE-visible change). Decisions 2/3 are read accordingly; §3.2 is authoritative for the physical view.db schema.
4. **Command flow:** pure `Command[T]` → HTTP validates synchronously → `Send` (async projections) → thin ack. Results reach clients via **projections → WS hub**, never the `Send` return. `SendWait` only in crash recovery.
5. **Two projections per aggregate, separated:** a durable **read-model projection** (→ `state/store/<type>.db`) and a distinct **hub projection** (→ WS). Both derive from `evt.Aggregate` so they cannot drift.
6. **Read models justified by access pattern.** Workspace and ReviewThread get durable read models (they are listed/filtered and/or accumulate long streams). Aggregates that are only fetched-by-id and short-history would use `Get(id)` + hub-only projection (none in this scope).
7. **Read-model rebuild is LAZY.** Normal boot re-opens durable read models with **zero replay**. `asynx.Replay` fires only as an **on-demand repair** when a read model is first accessed and found empty-but-its-event-log-has-events.
8. **Crash recovery is reconcile-only (no transient states added).** git/fs is the ground truth (worktree presence, `.git/MERGE_HEAD`, git status). Recovery = **lazy reconcile-on-open** (re-derive git+provider state → `SendWait` a sync command) + a **cheap boot orphan-sweep** (a `deleted` workspace whose worktree lingers → `Forget` + `rm`). This replaces the currently-broken "recovery sweep."
9. **Commands are pure.** `Validate`/`EmitEvent` do **no IO** — no network, no git, no filesystem. All network/git/fs side effects live in **cancelable, timeout-bounded reactors/sweeps** that then fire a pure command with the result. (This is the other half of the original wedge: untimed'd network git must never sit in the write path.)
10. **`writeMu` is deleted;** per-aggregate safety comes from shard routing + `(aggregate_id, version)` uniqueness, with **OCC retry** (retry ≤5× on `ErrPipelineFailed`, never on `ErrValidation`). The fs-watcher is **debounced/coalesced** to bound command volume.
11. **Graceful shutdown is ordered and bounded**, fixing quiver's prod gaps: `API.Shutdown(deadline)` → cancel exec ctx → **drain every asynx instance** (bounded by ctx) → `adapters.Close()` (WAL checkpoint + close all DBs). Every wait honors the deadline.
12. **Durability:** WAL + `busy_timeout(5s)` on every DB; read-model/view DBs get a **read pool** (multi-conn) so a single connection can't head-of-line-block reads (the original wedge); event logs stay single-writer.
13. **Worktree paths are human-readable, UUIDs banished from navigable paths.** `<HOME>/projects/<project>/<host>/<owner>/<repo>/<branch>/` is the worktree — the repo segment is the **full remote slug `host/owner/repo`** (globally unique), so slug-uniqueness is reflected on disk and there is no cross-host name clash. Identity stays UUID-keyed in `state/` + the `view.db` id↔path map. (RATIFIED 2026-07-07: `host` added to the template to reflect the slug identity on disk.)
14. **`CROWBAR_HOME` is kept** (dev isolation) and a `WithHomeDir` option is added for the integration test harness.
15. **Big-bang, no dead code.** Remove every old path (per-entity ES/view resolution, `Registry` for stores, `writeMu`, entity-scoped `storages/` layout, the broken recovery sweep). No migration, no deprecated shims, no dead branches. `go vet`/lint/`deadcode` clean.

---

## 3. Target architecture

### 3.1 Aggregate types
| Type | asynx instance | Event log | Read model | Notes |
|---|---|---|---|---|
| `Workspace` | `axWorkspace` (singleton) | `state/events/workspace.db` | `state/store/workspace.db` | Converted from per-entity. Read model doubles as the location index (carries `project_id`/`repo_id`; worktree path is derived). |
| `ReviewThread` | `axReviewThread` (singleton) | `state/events/review_thread.db` | `state/store/review_thread.db` | **Full conversion, equal to Workspace** (currently half-aligned). Read model **moves** out of the shared `view.db` → `state/store/review_thread.db`; event log **moves** `state/review_thread_event_stream.db` → `state/events/review_thread.db`; delete `writeMu`; `SendWait` → `Send`+OCC; split combined projector into store+hub. See §4. |
| ~~`Chat`~~ | deferred | — | — | Out of scope this milestone. Remove its wiring cleanly (no dead code) — including its **live** downstream consumer `branchreview` (Chat-removal scope in §4). |

### 3.2 On-disk layout (quiver-faithful)
```
<CROWBAR_HOME>/                                   # prod ~/.crowbar; dev <workspace>/.crowbar (CROWBAR_HOME)
  state/
    events/                                       # WRITE side — append-only asynx event logs (source of truth)
      workspace.db
      review_thread.db
    store/                                        # READ side — CQRS projections (rebuildable via lazy Replay)
      workspace.db
      review_thread.db
    view.db                                       # NON-aggregate reference (plain CRUD): projects, repos,
                                                  #   terminal profiles, terminal SESSIONS, workspace id↔path index
                                                  #   (the GORM stores are Projects/Repositories/TerminalProfiles/
                                                  #    TerminalSessions — all four in this one DB)
  projects/
    <project>/<host>/<owner>/<repo>/<branch>/     # the git worktree (friendly, navigable, no UUIDs; repo = full slug host/owner/repo)
  config.yaml                                     # "settings" — a plain YAML file at the home root (metadata
                                                  #   `Config` path), NOT a view.db table
  logs/
    daemon.log
```
- `core/metadata` is the single source of truth for the templated layout; `core/paths` is the single owner of lazy dir creation (`MkdirAll(0o750)` + per-path mutex). No new "home engine."
- Each HOME subtree has exactly one owner: **adapter** owns `state/`; the **worktree/git engine** owns `projects/.../worktree`; `core/logger` owns `logs/`.

### 3.3 Adapter layer (faithful to quiver's)
- Open a **fixed set of per-type event stores** at boot, exposed as typed fields (`WorkspaceES`, `ReviewThreadES`) — mirroring quiver's `ArrowES`/`RuntimeES`.
- Open the per-type **read-model DBs** and the global **`view.db`**.
- **Delete** `WorkspaceES(projectID, repoID, wsID)` per-entity resolution and the `Registry[asynxModels.Store]`/`Registry[view]` machinery entirely.
- `Container.Close()` = `errors.Join` over all event-store, read-model, and view closers (with WAL checkpoint). Unlike quiver, we close **all** planes, not just event stores.
- Keep `CROWBAR_HOME` resolution; add `WithHomeDir(dir)` for tests.

### 3.4 asynx instancing (app layer)
```go
// app.New — eager singletons, one per type
axWorkspace,    _ := newAsynx[domain.Workspace](adapters.WorkspaceES())
axReviewThread, _ := newAsynx[domain.ReviewThread](adapters.ReviewThreadES())

func newAsynx[T any](es asynxModels.Store) (asynx.Asynx[T], error) {
    return asynx.New[T]().
        WithEventStore(es).
        WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
        WithPanicHandler(...).          // keep Crowbar's existing observability (quiver lacks this)
        WithPublishErrorHandler(...).   // surface dropped projections
        Build()
}
```
The workspace repository holds `axWorkspace` (no registry, no per-entity resolution). `Get(id)` folds the aggregate from `events/workspace.db`. Commands route to a shard by `aggregateID` (the workspace UUID).

### 3.5 Command flow
```
POST …/workspaces           (or fs-watcher / provider-sweep firing a Sync command)
  1. decode + shape-validate                → 400 on bad input                 (fail fast)
  2. axWorkspace.Send(ctx, CreateWorkspace{…}) → Load→Validate→Append synchronous,
                                               returns (Event, err); projections async
  3. map err: ErrValidation→422 · ErrPipelineFailed→OCC retry ≤5×→409/500 · ErrQueueFull→503
  4. return 202 + {id}        (ack only; the full entity arrives via WS)
```
- **Command shape (pure):** `AggregateID()` (workspace UUID) · `EventName()` = `"workspace.<action>.<id>"` · `Validate(current)` (state-machine guard → `ErrValidation`) · `EmitEvent(current)` (pure next-state; asynx diffs to the event) · `ShouldSnapshot()`.
- **Delivery (two separate projections, both `Subscribe("workspace.*")`):**
  - `projections/store.go`: fold `evt.Aggregate` into `store/workspace.db` (durable read side).
  - `projections/hub.go`: derive the base WS frame from `evt.Aggregate`, then call an **injected enrichment callback** (owned by `repositories.Container`) to attach the two derived overlays before `hub.BroadcastWorkspace(frame)` — see "Hub-frame enrichment" below.
- **Hub-frame enrichment (preserving today's frame — no FE regression):** the current workspace hub frame is *not* bare `evt.Aggregate`. Today `repositories/container.go` `broadcastWorkspace` enriches every frame with two overlays that are **not part of `evt.Aggregate`** and must survive the store/hub split:
  - **Working/inflight overlay** (`ws.Working = c.IsWorking(id)`): request-bracketed, driven by `BeginWork`/`EndWork` rebroadcasts that fire on the **202 ack, not on any event**. A pure event-driven projection has no path for these spinner transitions, so `BeginWork`/`EndWork` keep a **direct rebroadcast** path: they call the same enrichment + `BroadcastWorkspace` on the container, independent of the projection bus.
  - **Merge-eligibility overlay** (`CanMergeLocally`/`ParentBranch` via a sibling `List` + `wsusecase.ResolveMergeEligibility`): needs the cross-sibling read model + the eligibility resolver, neither of which the hub-projection package owns.
  - **Mechanism:** the enrichment (inflight lookup + eligibility resolve) stays **owned by `repositories.Container`** and is passed into `projections/hub.go` as an injected callback. The hub projection and the `BeginWork`/`EndWork` rebroadcasts converge on **one** enrichment function, so the emitted frame is identical regardless of trigger. **Reconciles with decision 5:** the *durable read model* derives purely from `evt.Aggregate` (store and base frame cannot drift); the *WS frame* is that same base **plus** request-scoped/derived overlays (`Working`, eligibility) that are, by definition, not event state and therefore not expected to live in `evt.Aggregate`.
- **Error mapping** via `errors.Is` (never string compare): `ErrValidation`/`ErrPipelineFailed`/`ErrNotFound` → domain errors.

### 3.6 Cross-aggregate reactions (post-commit, wired once at startup)
`asynx` has no Saga/Reactor API — reactions are wired in an app-level `wireCallbacks`, and there are **two mechanically distinct kinds** that must not be conflated:

- **Synchronous `OnForget` handlers.** `Forget` is `SendWait`-backed (`asynx.go:317` → `proc.SendWait`): it runs every registered `OnForget` handler **to completion inside the `Forget` call**, then erases stored state. An `OnForget` handler must therefore be a **cheap, bounded read-model row delete only** — the store projection's `onForget` deletes the aggregate's row (as `reviewthread/internal/store/projections.go:75` already does). It does **no** fs/git/network IO and it **cannot** "outlive the request": it blocks the caller. (This is exactly why the read-model row is gone the instant `Forget` returns.)
- **Asynchronous `Subscribe` reactors.** All cross-aggregate cascades and every fs/git/network side effect live here — a `Subscribe` handler on the terminal delete event, **topic `"workspace.deleted.*"`**, dispatched off the projection bus, using `context.WithoutCancel` so post-commit work outlives the triggering request, cancelable + timeout-bounded, and registered on the drain WaitGroup for shutdown (decisions 9 + 11). **The emitted event name is `workspace.deleted.<id>`** (per §3.5's `EventName() = "workspace.<action>.<id>"` convention). asynx v0.6.2's `Topic()` compiles a subscription to a regex anchored at both ends, so the topic **must** be `"workspace.deleted.*"` (→ `^workspace\.deleted\..*$`, matches the id-suffixed event); a bare `"workspace.deleted"` compiles to `^workspace\.deleted$` and would **never** fire the reactor, silently leaking every deleted workspace's worktree until the next boot orphan-sweep. (Contrast the projections' `"workspace.*"` → `^workspace\..*$`, which correctly matches all actions.)

Commands never call into another aggregate. The delete cascade is an **async `Subscribe` reactor**, NOT an `OnForget` (an unbounded `rm -rf` must never be synchronous fs IO in the write path — decision 9):
```
Subscribe("workspace.deleted.*") → reactor (WithoutCancel, timeout-bounded, on drain WG):   // matches event workspace.deleted.<id>
    await store/workspace.db has Status="deleted" row for wsID   // GATE: persist-happens-before-purge (bounded wait; see ordering contract)
    reviewThreadAx.Forget(each thread of ws)   // each Forget's sync OnForget deletes that thread's row
    rm -rf <worktree>                          // bounded fs delete, OFF the synchronous write path
    axWorkspace.Forget(wsID)                    // TERMINAL step: purge the aggregate; its sync OnForget drops the ws tombstone row
    (chat deferred)
```
**Ordering / idempotency contract (why the tombstone cannot resurrect).** Two subscribers act on the *same* `workspace.deleted.<id>` event with **no asynx-guaranteed ordering** between them: the read-model store projection (`Subscribe("workspace.*")`) **persists** the `Status="deleted"` row, while this reactor (`Subscribe("workspace.deleted.*")`) ultimately **deletes** that row via `axWorkspace.Forget`'s synchronous `OnForget`. If the reactor's `Forget` ran before the store projection persisted, the projection would re-save a `deleted` tombstone **after** the worktree was already `rm -rf`'d — a resurrected orphan row that would survive until the next boot sweep. To force **persist to happen-before purge**, the reactor's **first step gates on observing the persisted tombstone**: it reads `store/workspace.db` (bounded wait) and proceeds only once the `Status="deleted"` row for `wsID` is present. Because `workspace.deleted.<id>` is the aggregate's **terminal** event, once that row is observed the store projection has **no further event to persist** for `wsID`, so the reactor's subsequent `Forget`-driven row-delete is the last write for the aggregate and no re-persist can race it. `Forget` is deliberately the **terminal** reactor step (after `rm -rf`): a crash between `rm` and `Forget` leaves at worst a stale `deleted` row (cheap, harmless, reaped by the boot sweep — §3.8), never the reverse (an orphan worktree with no row, which is the exact crash gap §3.8 closes). The whole reactor is **idempotent** and re-driven verbatim by the boot sweep after a crash: awaiting an already-present (or, on re-drive, still-present) row, `Forget`-ing an already-forgotten aggregate, and `rm -rf`-ing an already-gone worktree are all no-ops. **Invariant:** after a **successful (non-crash) delete** the end state is exactly **no read-model row AND no worktree**; a crash mid-cascade leaves a `deleted` row that the boot orphan-sweep (§3.8) re-drives to that same end state. The tombstone this reactor consumes is persisted by the delete lifecycle in §3.8.

### 3.7 Read models & lazy rebuild
- Read models are **durable** — normal boot re-opens them, **no replay**.
- **Lazy Replay repair (whole-model, multi-aggregate):** `asynx.Replay(ctx, aggregateID, from, to, fn)` rebuilds a **single** aggregate id, so a lost *List* read model cannot be healed by one call. The repair reuses the existing reviewthread `aggregateIDs()` / `serialize.AggregateLister` pattern (`reviewthread.go:112-138`, `internal/serialize/aggregate_lister.go`): enumerate every id the event log holds via `es.(serialize.AggregateLister).AggregateIDs(ctx)`, then `Replay` each id into `store/<type>.db`.
  - **Trigger:** a `List` that finds its durable read model **empty while the event log's id set is non-empty** (there is no single id to key "first access" on for a List, so the id-set check is the trigger). A per-id `Get` needs no rebuild — it folds the aggregate directly from the event log.
  - Startup never pays for this; it fires only on the first post-loss List. (A deliberate improvement over quiver, which cannot rebuild a lost read model.)
- The workspace read-model row carries `project_id`/`repo_id`, so it **doubles as the location index**; the worktree path is derived (§3.9), so **no separate location index table** is needed beyond `view.db`'s id↔path map for rename resilience.

### 3.8 Graceful shutdown & crash recovery
**Shutdown (ordered, bounded — fixes quiver's prod gaps):**
```
SIGINT/SIGTERM → ctx cancel → bounded shutdownCtx (~5s):
  1. API.Shutdown(deadline)        stop new reqs, drain in-flight HTTP
  2. cancel exec ctx               stop fs-watchers, provider sweeps, reactor goroutines
  3. drain EACH asynx              quiesce reactors (drain gate + WaitGroup, ctx-bounded) → ax.Shutdown
  4. adapters.Close()              WAL-checkpoint + close all event/read/view DBs
```
All waits honor the deadline (quiver's `drainWg.Wait()` is unbounded — we bound it).

**Recovery (synchronous entry in `Start`, reconcile-only):**
- **Delete lifecycle (persist-then-purge — so the sweep has a row to find):** `Delete` is a pure command that emits the terminal `workspace.deleted.<id>` event (per §3.5's `EventName()` convention); the read-model projection **persists the row with `Status = "deleted"`** (an existing terminal status — not a new transient state, so decision 8 holds) and does **NOT** `Forget` synchronously. Physical teardown (cascade thread `Forget`s + `rm -rf` worktree + `axWorkspace.Forget` to purge the aggregate and drop its tombstone row **as the terminal step**) runs in the async reactor that §3.6 subscribes on **topic `"workspace.deleted.*"`** (which matches the id-suffixed event name). The reactor **gates its purge on the persisted `Status="deleted"` tombstone** per §3.6's ordering/idempotency contract, so the store projection's persist happens-before the `Forget` that deletes the row — the tombstone can never be re-saved after the worktree is gone. This deliberately closes the current `Delete → forget → removeWorkspaceDir` crash gap (`workspace.go:665-674`), where the synchronous `forget` **already erased the row** before the `rm`, so a crash in between left a worktree on disk with **no row for any sweep to find**.
- **Reconcile-on-open (lazy, bounded, off the read path):** **"First accessed" means the first single-workspace `Get`/detail open** (the per-id read path) after boot — explicitly **NOT** a `List`. A `List` reads the durable `store/workspace.db` directly and MUST NOT trigger any per-workspace reconcile: a synchronous per-workspace network fan-out across a `List` would reintroduce exactly the blocking-network starvation / wake-storm wedge this refactor exists to kill (§1.1/§1.2, decision 9). On that first per-id open, reconcile does **not** run inline on the caller's read path (the `Get` returns immediately from the read model); it is dispatched as a **one-shot background task per workspace** (deduplicated so repeated opens don't stack tasks). That task performs the git+provider re-derivation as **cancelable, timeout-bounded IO** (decision 9 — network/git never sits unbounded in any path), then `SendWait`s a **pure** sync command (the command itself does no IO — this is decision 4's single sanctioned crash-recovery `SendWait`) and broadcasts the corrected frame to clients via the WS hub. No transient states; the aggregate state machine is unchanged.
- **Boot orphan-sweep (cheap, proactive):** iterate the workspace read model — reading the durable `store/workspace.db` **directly, WITHOUT triggering lazy Replay** (so boot stays cheap per decision 7). Any residual row still in `Status = "deleted"` means the async delete reactor didn't finish before a crash (its worktree may be lingering, or already `rm`'d with only the terminal `Forget` outstanding) → **re-drive the same idempotent purge** (cascade thread `Forget`s + `rm -rf` worktree + `axWorkspace.Forget`), which converges to the delete invariant (no row, no worktree) whichever step the crash interrupted; a half-provisioned worktree → complete/clean. **Accepted consequence:** a crash that *simultaneously* lost `store/workspace.db` leaves the sweep reading an empty model, so it reaps nothing; reaping such an orphan defers to the first lazy-Replay access that rebuilds the model (§3.7). This is deliberate — boot never pays the replay cost.
- **No boot event-log replay** to rebuild read models (that's the lazy repair path in §3.7).
- Idempotent by construction: skip on `ErrNotFound`; `Validate` guards reject non-applicable transitions; reconcile re-derives from reality every time.

### 3.9 Worktree naming
- Path: `<HOME>/projects/<project>/<host>/<owner>/<repo>/<branch>/` — the worktree itself. The repo is encoded as its **full remote slug `host/owner/repo`**, so the slug's global uniqueness is reflected on disk.
- Collision-free by **natural identity**: **project** unique-by-enforcement at creation; **repo** = its full remote slug `host/owner/repo` (globally unique, a quiver namespace — **encoded in full on disk**, so two repos differing only by host do not clash); **branch** unique per repo (git enforces; git's D/F rule prevents `feature` vs `feature/x`). **No short-id suffix.**
- No character sanitization: `git check-ref-format` already forbids unsafe branch names; `/` maps to clean nested dirs.
- **Two residual cases handled at creation** (not general suffixing):
  1. **Case-insensitive FS** (macOS APFS / Windows): git-distinct case-only identities collide on disk → **reject at creation** with a name-clash error. Disambiguation is **not** offered: the only mechanisms that could disambiguate — a short-id suffix or character rewriting — are both banned by decision 13, so reject-at-creation is the sole D13-consistent behavior.
  2. **Local repo with no remote** (no slug) → require a user-given unique name as its identity. That name occupies **the entire `<host>/<owner>/<repo>` slug position as one leaf segment** (there is no slug to split), so the worktree path is `<HOME>/projects/<project>/<name>/<branch>/` (and its repo-home leaf is `<HOME>/projects/<project>/<name>/.home/`, per the repo-home rule below). The name is unique-by-enforcement within its project, exactly like a slug.
  - **(The former "same `owner/repo` across different hosts" case is now ELIMINATED** by decision 13's full-slug path: encoding `host/owner/repo` on disk makes `github.com/acme/app` and `gitlab.com/acme/app` distinct paths, so there is no clash and no reject needed. RATIFIED 2026-07-07 as ratification path (a): `host` added to the template.)
- A small **path-derivation helper** owns id→path derivation, sanitization edge-cases, and rename (`git worktree move` + update the `view.db` id↔path map). Path is derived, not identity, so a failed move still resolves via the map.
- **The `view.db` id↔path map — owner, schema, lifecycle (previously unspecified):** a single minimal plain-CRUD table `workspace_paths(workspace_id TEXT PRIMARY KEY, worktree_path TEXT NOT NULL)`, **owned by the adapter** (which owns `view.db`) and exposed as a fifth `view.db` store alongside the existing four (`api/internal/app/gorm.go:15-18`). It is deliberately **distinct** from both the `store/workspace.db` read model (which carries `project_id`/`repo_id` for List + merge eligibility) and the old `internal/locations` table; it exists **only for rename resilience** (id → last-known path when the derived path has drifted). **Write points:** (a) the initial row is written on workspace **Create**, keyed by the workspace UUID, when the worktree is first provisioned; (b) it is **updated on rename** by this path-derivation helper after a successful `git worktree move` (§3.9); (c) it is **deleted** when the delete reactor purges the aggregate (§3.6). No other writer touches it.
- **Disposition of `internal/locations`:** the current `internal/locations` table (`workspace_id → {project_id, repo_id}`, today the `List`/`entityFor` authority) is **DELETED, not repurposed** (decision 15) — its id→{project_id,repo_id} role is subsumed by the `store/workspace.db` read model (§3.7), and its residual id→path role becomes the narrower `workspace_paths` id↔path map above. It appears in the §4 impact map.
- **Repo-home workspace — TWO cases (RATIFIED 2026-07-07, per the locked Crowbar workspace-model law):**
  - **Adopted repo-home / adopted-home (the common case): the repo home IS the user's real checkout** and stays rooted at the user's actual clone path (`repo.Path` / `project.Path` in `project_import.go:478,681`), detached to HEAD when on a protected branch. It is **never** relocated to a Crowbar-managed `.home` leaf — forcing `.home` would physically move the user's clone, violating the locked law.
  - **Net-new Crowbar-managed home worktree** (only when Crowbar itself materializes a home worktree under `<HOME>/projects/...`, not an adopted checkout) → the `.home` leaf `<HOME>/projects/<project>/<host>/<owner>/<repo>/.home/` (no-remote repo: `<HOME>/projects/<project>/<name>/.home/`). This is **not** the `<repo>/` root — that would nest the home worktree as the **parent** of the per-branch worktrees. The `.home` sentinel sits as a **sibling** of the branch leaves and is **provably collision-proof against every branch leaf**: a leading-dot refname component is rejected by `git check-ref-format` (`refs/heads/.home` fails), so no legal branch can ever derive it. (**Not `@home`:** `git check-ref-format refs/heads/@home` and even `refs/heads/@` are both *valid* (verified), so a branch named `@home` would collide; the leading dot, not the `@`, is what makes the guarantee hold.)

---

## 4. Impact map (what changes; what is deleted)

**Rewritten:**
- `api/internal/adapter/container.go` — per-type stores at boot, typed fields, WAL/pools, close-all. Delete per-entity resolution + `Registry`. **Route DB file paths through the home-parameterized `paths.EventsAt(c.crowbarHome)` / `paths.StoreAt(c.crowbarHome)` accessors** (the `…At(home)` variants §3.2 already notes exist) — **not** the home-agnostic `paths.Events()`/`paths.Store()`, which resolve from `metadata.resolveHome()` (the `CROWBAR_HOME` env var or the prod default) and are blind to the container's `cfg.homeDir`. `WithHomeDir(dir)` (decision 14, used by the §5 harness's `BuildEnv(t, home)`) sets `cfg.homeDir` only — it calls `GetStateDirPathAt(homeDir)` and does **not** export `CROWBAR_HOME` — so a test isolating via `WithHomeDir(tempDir)` without also setting the env var would otherwise land event/store DBs under the **production** `~/.crowbar` while the rest of state lands under the temp dir: split state + a write leak into prod home, breaking D14/§5 isolation. **The adapter MUST derive every state subtree from its resolved `crowbarHome`.** This replaces the current hand-built `filepath.Join(stateDir, "review_thread_"+eventStreamDBName)` / `chat_event_stream.db` / `view.db` construction and per-entity `storages/` dirs (`container.go:118-128,181-199`).
- `api/internal/app/repositories/workspace/**` — singleton `axWorkspace`; delete `Registry`/LRU/`writeMu`/`entityForLocation` per-entity path; commands under `internal/commands/`; projections split into `store.go` + `hub.go`; reactors.
- `api/internal/app/repositories/reviewthread/**` — **full conversion, equal to Workspace's** (it is currently half-aligned: global ES, but read model in the shared `view.db`, `writeMu` + `SendWait` on every mutation, one combined projector). Concrete deltas: relocate read model `view.db` → `state/store/review_thread.db`; relocate event log `state/review_thread_event_stream.db` → `state/events/review_thread.db`; delete the `serialize.KeyedMutex` `writeMu` (`reviewthread.go:93,145-258`); `SendWait`-everywhere → `Send` + OCC retry ≤5×; split the single combined projector (`internal/store/projections.go` — one `onEvent` that saves **and** broadcasts) into distinct `store.go` + `hub.go` projections, both off `evt.Aggregate`; **drop the eager reconcile-on-open** — `New` currently enumerates `aggregateIDs()` and reconciles every read-model row from the event log at open (`reviewthread.go:112-113` → `store.New(ctx, db, ax, broadcast, ids)`), a boot-time replay that violates decision 7. Move that id-enumeration + per-id `Replay` behind the **lazy empty-model / non-empty-log trigger** (§3.7), exactly as Workspace does, so normal boot re-opens the durable `store/review_thread.db` read model with **zero replay** and replay fires only as on-demand repair.
- `api/internal/app/asynx.go` — unchanged shape, applied per type.
- `api/internal/app/container.go` (the `app.New` shown in §3.4) — the app-layer composition root where three locked decisions physically land. Concrete deltas: (1) **D1 core change** — stop passing the per-entity `AsynxFactory` `newAsynx[domain.Workspace]` (`container.go:60`) into `repositories.New`; instead construct the eager `axWorkspace` **and** `axReviewThread` singletons here (per §3.4) and pass **those** in place of the factory. (2) **Chat removal** — delete the `axChat` construction (`container.go:39-42`) and its argument to `repositories.New` (`container.go:58`); these are hard compile blockers the instant the Chat repo and `adapters.ChatES()` are deleted (see Chat-removal scope). **Keep** the adjacent `axReviewThread` construction (`container.go:43-46`) and its argument (`container.go:59`) — do not delete by the old `39-46`/`59` range or you would remove `axReviewThread`. (3) **Recovery-sweep deletion** — this file both defines and starts the broken recovery sweep `startRecoverySweep`→`ucs.Worktree.ReconcileAll` (`container.go:77,139-146`); **delete it** (the "delete the broken recovery sweep" item below is located here), replaced by reconcile-on-open + orphan-sweep (§3.8).
- `api/internal/app/repositories/container.go` — wire singletons (`axWorkspace`, `axReviewThread`) + the per-type read-model DBs (drop reviewthread's `GlobalView()` DB source at `container.go:58,63`) + `wireCallbacks` (cross-aggregate reactions). **Retain the hub-frame enrichment here** (§3.5): the `Working`/inflight overlay (`IsWorking`) and merge-eligibility (`ResolveMergeEligibility` over a sibling `List`) stay **owned by the container** and are injected into `projections/hub.go` as an enrichment callback rather than emitted from the hub directly. `BeginWork`/`EndWork` keep their direct, request-bracketed rebroadcast (they fire on the 202, not on an event) and route through the same enrichment path, so the FE spinner and merge badges survive the store/hub split with zero functional regression.
- `api/internal/core/metadata`, `api/internal/core/paths` — **no new templates or accessors:** `events: {{home}}/state/events`, `store: {{home}}/state/store`, and `Events()/EventsAt()/Store()/StoreAt()/State()` already exist and `view.db` already lives under `state/`; the change is only that the **adapter must be wired to them** (it currently bypasses them, see above). The genuinely-new piece is the human-readable worktree path-derivation helper (§3.9).
- `api/internal/app/gorm.go` (the `view.db` store set) — add a **fifth plain-CRUD store `WorkspacePaths`** (the `workspace_paths` id↔path map, schema + CRUD) alongside the existing four (`gorm.go:15-18`), per §3.9. Wire its three write points — Create (initial row), rename (path-derivation helper), delete (delete reactor) — into the workspace repo/helper. This is the id→path role that the deleted `internal/locations` table used to hold; the id→{project_id,repo_id} role moves to the `store/workspace.db` read model (§3.7).
- Adapter sqlite helpers (`eventstore/sqlite`, `store/sqlite`) — keep the existing WAL + `busy_timeout(5s)` (both `store/sqlite/sqlite.go` `OpenDB` and `eventstore/sqlite/event_store.go` `NewEventStore` already set `journal_mode=WAL` + `busy_timeout=5000` under `SetMaxOpenConns(1)`); the change is replacing `SetMaxOpenConns(1)` with a multi-conn read pool on the read-model/view DBs (event logs stay single-writer).
- Recovery: delete the broken "recovery sweep" (located in `api/internal/app/container.go` — `startRecoverySweep`→`ucs.Worktree.ReconcileAll`, see that bullet above); add reconcile-on-open + orphan-sweep.
- Graceful shutdown wiring in the daemon `Start`.

**Deleted outright (no shim):** per-entity `event_stream.db`/`view.db` layout, `Registry[asynxModels.Store]` / `Registry[view]`, **both** `workspace.send()`'s `writeMu` **and** `reviewthread`'s `serialize.KeyedMutex` `writeMu` (`reviewthread.go:93`), `SendWait`-everywhere (both aggregates), the `internal/locations` id→{project_id,repo_id} table/package (subsumed by the `store/workspace.db` read model + the new `view.db` `workspace_paths` id↔path map, §3.7/§3.9), the entity-scoped `projects/<P>/<R>/workspaces/<W>/storages/` tree, UUID worktree folder names, and the deferred **Chat** aggregate's wiring (see Chat-removal scope).

**Chat-removal scope (deferred aggregate, but with a live downstream consumer):** Chat's own HTTP/WS routes are already dormant (`v0/router.go:115`, removed per **Crowbar design-doc D11** — the existing `router.go:115` code comment, **not** this spec's locked-decision 11 [graceful shutdown]; bare `D<n>`/`decision <n>` elsewhere in this spec always means the 15 locked decisions), so the endpoint surface is safe. But `app/usecases/branchreview/branch_review.go` is a **live** consumer of the Chat repo: it holds `chats chat.Chat`, calls `u.chats.ListByWorkspace(ctx, ws.ID)`, and feeds `branchchat.From(...)` into `BranchReview.Conversations` (wired at `app/usecases/container.go:125-128`, served at `v0/router.go:152 review.Register`, backing the FE Branch Review screen). **Disposition:** drop `Conversations` from Branch Review and remove the `chats` dependency (and the `branchchat` helper) from `branchreview`. Dropping the field **is a wire-contract change** — the `GET /review` JSON loses its `conversations` key — but it is **FE-safe** for a concrete reason: `web/src/features/git/api/review-api.ts:115` already reads it defensively as `(raw.conversations ?? []).map(mapConversation)`, so a missing key degrades to an empty list with no runtime error. (The field is also always-empty today — chat writes are dormant with routes gone per Crowbar design-doc D11, and prod `~/.crowbar` is wiped on ship, so `ListByWorkspace` already returns nothing — but the `?? []` guard, not the empty-data fact, is what makes removing the wire field safe.) Files that must change for the big-bang delete to compile:
- `api/internal/app/usecases/branchreview/branch_review.go` — drop the `chats` field, the `u.chats.ListByWorkspace(ctx, ws.ID)` call, and the `branchchat.From(...)` assembly into `BranchReview.Conversations`.
- `api/internal/app/usecases/container.go` — drop the Chat-repo wiring into `branchreview` (`container.go:125-128`).
- `api/internal/domain/branch_review.go:14` — drop the `Conversations []BranchChat` field from the `BranchReview` domain struct.
- `api/internal/api/v0/dto/review.go` — drop `Conversations` from `BranchReviewDTO` (the `[]domain.BranchChat` field at line 44) and from the `BranchReviewDTOFrom` mapping (the nil-guard + assignment at lines 103-113), so the served `/review` payload stops emitting the key.
- `api/internal/domain/branch_chat.go` — delete the now-orphaned `domain.BranchChat` type. Once `Conversations` is gone from both the domain struct and the DTO it has no remaining referent, so **decision 15 (no dead code) requires deleting it** (along with the `app/usecases/internal/branchchat/branchchat.go` helper).
- `api/internal/app/container.go` — delete the `axChat` construction (`container.go:39-42`) and the `axChat` argument passed to `repositories.New` (`container.go:58`). Both are hard compile blockers the instant `adapters.ChatES()` and the Chat repo are removed (also called out in that file's Rewritten bullet above). Leave the adjacent `axReviewThread` construction (`container.go:43-46`) and argument (`container.go:59`) intact.
- `api/internal/app/repositories/container.go` — drop the `axChat` parameter from `repositories.New`'s signature and its downstream use (`container.go:46,59`), so the composition root above compiles once it stops passing `axChat`.
- Delete the Chat repo/usecase itself — the Chat repo at `api/internal/app/repositories/chat` and the Chat usecase at `api/internal/app/usecases/chat`.

**Frontend:** no behavior change. Verify existing WS-driven flows still work (list/detail/status/tree, review threads, git panels, terminals) and that Branch Review renders correctly with no `Conversations` block (already empty today).

---

## 5. Testing strategy (quiver's integration kit, adapted)

- `//go:build integration`; `IntegrationSuite` + `TestMain → kit.Main` (silence logs, gin test mode).
- `BuildEnv(t, home)` wires the **real** adapter+app+api containers over a Unix socket with **`WithHomeDir`** isolation.
- **Restart = a second `Env` over the same home dir** (durable read models must survive). Three teardown variants: `Close` (graceful drain), `CloseCrashing` (skip drain), `CloseWithoutKilling` (leave procs). Async assertions via WS state-watchers.

| Test | Simulate | Assert |
|---|---|---|
| Graceful restart persists | `Close` after creating workspaces | read model intact via GET; **no replay ran** |
| Crash mid-provision | `CloseCrashing`, half-made worktree | reconcile completes/cleans; status correct |
| Crash mid-merge | `CloseCrashing`, `MERGE_HEAD` present | reconcile → `pr-conflicts`/aborted; read model correct |
| Provider drift while down | mutate remote, restart | reconcile re-fetches → read model updated |
| Deleted + lingering worktree | `Close` after `Delete` persists the `deleted` row but before the reactor purges (kill mid-cascade) | boot sweep finds the `Status="deleted"` row + lingering worktree → re-drives cascade `Forget`s + `rm` + `axWorkspace.Forget`; row and worktree gone |
| Graceful drain integrity | `Close` under load | all asynx drained + DBs closed within deadline; no lost writes; drain doesn't hang |
| Lazy read-model rebuild | delete `store/workspace.db`, restart, then **List** | empty model + non-empty event-log id set → `AggregateLister` enumerates ids, per-id `Replay` rebuilds each row; list correct |
| Friendly worktree path | create workspace | worktree at `projects/<project>/<host>/<owner>/<repo>/<branch>/` (full slug on disk); case-only clash rejected; two repos differing only by host resolve to distinct paths (no clash) |

Plus: keep all existing black-box regression tests green (`api/tests/integration`), unit + `-race`, lint, `go vet`, prettier/tsc on `web/`.

## 6. Verification (live, before "done")
- Build + run via **`make dev-desktop`** only. **Never** touch or override production `~/.crowbar` — dev roots at `<workspace>/.crowbar` via `CROWBAR_HOME`.
- Drive the real app with the **Tauri MCP server**: create/list/switch workspaces, open review, git panels, terminals; confirm WS-driven UI updates and **zero functional regressions** vs. current behavior.
- Confirm the goroutine/DB-handle footprint is now fixed-per-type (not per-open-workspace).

## 7. Risks / watch-items
- **fs-watcher storms** → OCC retry churn; mitigate with **debounce + OCC** (start quiver-faithful). Note that lowering workers-per-shard is **not** an in-repo knob: asynx v0.6.2's builder `ShardingOpts` exposes only `Shards`/`QueueDepth`, and `workersPerShard` is an internal processor default (8) with **no public builder setter** (matching the current `workspace.go` comment). A `WithWorkersPerShard(1)` therefore requires an **upstream asynx change** (add the builder option + bump the pinned dependency), which sits outside this backend-only milestone and cuts against the §1.1/§1.2 premise that asynx is not at fault. Treat it as a deferred upstream option, not a config we can flip; rely on debounce + OCC here.
- **Central DB contention** reintroducing the head-of-line wedge → mitigated by WAL + read pool; verify under load.
- **ReviewThread conversion** is now a first-class Rewritten item with concrete deltas (§4), not a lingering risk to remember — the residual watch-item is only that its long per-thread streams exercise the OCC/retry path harder than Workspace, so verify reply-heavy flows under `-race`.
- **Chat deferral** must be a clean removal, not commented-out dead code — and it has a **live** `branchreview` consumer that the delete must account for (§4 Chat-removal scope), not just its already-dormant routes.

## 8. References (quiver files to copy the pattern from)
- Instancing/newAsynx: `internal/app/container.go`; adapter stores: `internal/adapter/container.go`, `internal/adapter/eventstore/sqlite/*`, `internal/adapter/store/sqlite/*`.
- Command shape: `internal/app/repositories/*/internal/commands/*`; projections: `.../internal/store/internal/projections/*`; reactions: `.../runtime/internal/reactions.go`, container `wireCallbacks`.
- Recovery: `.../runtime/internal/recovery.go`. Shutdown drain: `.../runtime/runtime.go`, `cmd/quiver/daemon.go`, `internal/internal.go`.
- Test kit: `tests/kit/*`, `tests/integration/crash/*`, `tests/integration/lifecycle/*`.
- Paths/home: `internal/core/paths/paths.go`, `internal/core/metadata/*`.
