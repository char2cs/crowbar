# Crowbar Backend — Architecture & Domain Model

> **Status:** Approved
> **Date:** 2026-06-03
> **Scope:** Full rewrite of the Crowbar Go backend to serve the frontend
> capabilities described in `docs/v0/ux-capabilities.md`.
> **Companion specs:** see the other documents in `api/docs/specs/v0/`.

---

## 1. Purpose & Constraints

Crowbar is a **local, single-user desktop tool** (think VS Code). The backend
runs on the user's own machine alongside the web frontend.

- **No authentication.** No multi-tenancy, no per-user scoping on queries, no
  session isolation between users.
- **Single source of truth is the local machine** — the real filesystem, real
  git repositories, real PTY processes.
- The backend is a **complete rewrite**. The existing `api/` scaffold's
  architecture (layered containers, Asynx, GORM, topic-scoped WebSockets) is the
  pattern we keep; its current domain model (Task, KanbanItem, Flow) is discarded
  where it does not match the UX capabilities.

The reference implementation for **all** infrastructure patterns is
`quiver.core` (`/Users/char2cs/Projects/Rabbyte/quiver.core`). We replicate its
container wiring, `Broadcaster[T]`, `dispatch()` REST/WS dual-serve, Asynx event
sourcing, and hub-projection conventions.

---

## 2. Layer Order

Identical structure to `quiver.core`. Each layer receives only the layers below
it; lower layers never know about higher ones.

```
cmd/crowbar/          CLI entry point (cobra, signal handling)
internal/internal.go  Root container — wires layers in order:
                        1. engine.New()    AI Bridge engine, addon registry
                        2. adapter.New()    SQLite event stores + GORM DB
                        3. app.New(engines, adapters)   repositories, usecases, hub
                        4. api.New(appContainer)        handlers, broadcasters, routes
  engine/             Business engines (AI Bridge — see spike spec)
  adapter/            Persistence: SQLite event stores, GORM connection
  app/                Application layer: repositories, usecases, hub
  api/                Delivery layer: REST handlers, WS broadcasters, routing
  domain/             Aggregate types and state machines
  core/               Cross-cutting: config, paths, metadata
```

`api` knows about `app`; `app` knows about `adapter` and `engine`; neither knows
about `api`.

---

## 3. Identifiers

**All entity IDs are UUIDs** — every GORM row and every Asynx aggregate.
No exceptions. Path parameters carrying IDs are UUID strings.

---

## 4. Storage Tiers

The decision rule: **Asynx for entities with a meaningful state machine** (where
event history, projections, and crash recovery pay off) and **GORM for plain
CRUD**.

### Asynx (event-sourced aggregates)

| Aggregate | Why Asynx |
|-----------|-----------|
| `Workspace`    | Status state machine (new → pr-merged) + live overlays; also holds provider-synced PR state |
| `AgentRun`     | Lifecycle (pending → running → done/error/interrupted) + crash recovery |
| `ReviewThread` | open ↔ resolved transitions |
| `Chat`         | idle ↔ agent-running; projections drive the sidebar for free |

> **Revision (provider engine):** `PullRequest` is **not** a standalone
> aggregate. Crowbar only *reads* PR state from the git provider (`gh`/`glab`),
> never creates PRs. PR state is synced onto the **Workspace** aggregate by the
> Git Provider engine (`08-git-provider-engine.md`). See §5.3 and §6.4.

One SQLite event-store file per aggregate type, under
`~/.crowbar/state/events/{aggregate}.db`, configured with 8 shards / queue depth
1000 (same as quiver.core).

### GORM / SQLite (plain CRUD)

| Entity | Notes |
|--------|-------|
| `Project`         | id, name, path, lastActivity |
| `Repository`      | id, projectId, name, path, defaultBranch, avatarLabel, avatarColor |
| `TerminalProfile` | id, name, shell?, startupDirectory?, startupCommands[], icon?, color? |

Single GORM database at `~/.crowbar/state/store/crowbar.db`.

> **Note:** The contents *inside* a chat (turns, tool calls, widgets, agent TODO
> lists) are **deliberately excluded** from this spec. Their storage is decided
> after the Agentic Bridge spike (see `12-agentic-bridge-spike.md`).

### In-memory only

| Entity | Notes |
|--------|-------|
| `TerminalSession` | Exists only while its PTY process is alive; lost on restart |

---

## 5. Domain Entities

### 5.1 Project (GORM)

```
Project {
  id           uuid
  name         string
  path         string      // absolute path on disk
  lastActivity time.Time
}
```
`repoCount` is derived (count of repos with this `projectId`), not stored.

**Project is the org-level node.** The sidebar's top-level "Org name" row (UX §2)
and the org-switcher dropdown are the **Project** — a Project is a dumb top-level
container that groups repos to keep contexts together (it owns a folder; its repos
may even live in different folders). There is **no separate Org entity**; "switch
org" = switch project. `lastActivity` is bumped whenever any descendant
(repo/workspace) sees activity — a commit, a file write, or chat activity — so the
"last touched" label (UX §1) stays accurate. **Writer path:** Project is GORM, so
nothing fires on it automatically — the same activity that issues
`SyncWorkingTreeState` / `TouchActivity` to a Workspace (§5.3) also performs a
**GORM update of that workspace's Project (`repoId`→`projectId`) `lastActivity`**
(a cheap denormalized roll-up). This is the one place an Asynx command and a GORM
write are coupled in one usecase — the roll-up is **best-effort**: a GORM update
failure is logged but does **not** fail the Asynx command. Without this the §1
label would never move.

### 5.2 Repository (GORM)

```
Repository {
  id            uuid
  projectId     uuid
  name          string
  path          string       // absolute path on disk
  defaultBranch string       // resolved at import, never empty: git symbolic-ref
                             //   refs/remotes/origin/HEAD; else the first of the
                             //   08 §3 config list (main/develop/master) that
                             //   `git rev-parse --verify` confirms exists; else
                             //   the repo's current HEAD branch
                             //   (`git rev-parse --abbrev-ref HEAD`).
                             //   the base for root reviews (09 §2)
  avatarLabel   string       // single char for the avatar badge
  avatarColor   string       // color class for the badge background
}
```

### 5.3 Workspace (Asynx)

A workspace is a **`git worktree` (its own directory on disk) checked out to a
branch** inside a repo. Workspaces nest under repos and can be children of other
workspaces (branch-off-branch). Full hierarchy mechanics — worktree creation,
local child→parent merge, re-parenting — are in
`07-workspace-worktree-hierarchy.md`.

```
Workspace {
  id           uuid
  repoId       uuid
  projectId    uuid        // denormalized from the repo, so the Workspaces
                          //   broadcaster can filter by ?projectId= on the
                          //   payload alone (no join at filter time) — 03 §4.1
  branch       string
  worktreePath string      // the git worktree directory on disk
  forkPointSha string      // commit the branch was created from (recorded at worktree add)
  parentId     uuid?       // nested (fork-of-fork) workspaces
  status       WorkspaceStatus?   // new | pr-open | pr-merged | pr-closed; null once it has commits but no PR
  locked       bool        // provider-protected branch — chat-only, cannot delete/merge-into
  hasConflicts bool        // summary overlay (from git subsystem, via SyncWorkingTreeState)
  mergeStrategy merge|squash|rebase            // default merge; the branch-review selector (09 §4)
  pendingMerge { strategy, targetParentId }?   // set when a merge-into-parent conflicts (07 §3.1, 04 §6.1)
  added        int         // +N lines vs parent (from git subsystem, via SyncWorkingTreeState)
  deleted      int         // -N lines
  // provider-synced PR state (Git Provider engine — 08); empty when no provider access
  prUrl          string?
  prTitle        string?
  prTargetBranch string?
  lastActivity time.Time   // bumped on commit / file write / chat activity
  createdAt    time.Time
}
```

`age` (the relative-time string the UI renders) is derived from `lastActivity` /
`createdAt`, not stored. `locked` is resolved at creation via the Git Provider
engine (protected-branch detection, falling back to a config list when no
provider access). `forkPointSha` is recorded at `git worktree add` time and is the
authoritative fork point for re-parenting (`07` §4) — never recomputed via
`merge-base`.

**Single source of truth.** Every field on the workspace row is owned by this
aggregate and mutated **only through Asynx commands**:

| Command | Owns (writes) | Issued by |
|---------|---------------|-----------|
| `SyncWorkingTreeState{added, deleted, hasConflicts, hasCommits}` | `added`, `deleted`, `hasConflicts`; **clears `new`→null**; bumps `lastActivity` | the watcher **and** git write usecases (both recompute from git — per-field sources below) |
| `SyncProviderState{prInfo?, protected}` | `status` ∈ {pr-open,pr-merged,pr-closed}, `prUrl`, `prTitle`, `prTargetBranch`, `locked` | provider poller (`08`) |
| create / hierarchy / merge / reparent | `status` (`new` at create), `parentId`, `forkPointSha`, `branch`, `worktreePath`, `pendingMerge` (set on a conflicted merge-into-parent, cleared on continue/abort — `04` §6.1); bumps `lastActivity` | usecases (`07`) |
| `SetMergeStrategy{strategy}` | `mergeStrategy` only | branch-review usecase (`PATCH .../review`, `09` §4) |
| `TouchActivity` | `lastActivity` only | chat / AgentRun activity (`01`) |

Asynx **serializes commands per aggregate**, so no two ever interleave a
half-write. Field ownership is disjoint **except `status`**, whose writers act in
disjoint lifecycle phases and are guarded so they cannot fight (§6.1).

**`hasConflicts` and `SyncWorkingTreeState` have two issuers, one command.** Both
the watcher (on disk change) and the git write usecases (after a merge / rebase /
pull / conflict-resolve, `04` §5/§6) recompute the summary **from git** and issue
the **same** `SyncWorkingTreeState` command. Each field has a distinct source:

- `hasConflicts` ← `git status --porcelain=v2` (presence of unmerged paths)
- `added` / `deleted` ← `git diff --numstat <forkPointSha>` — a **single** diff
  from the fork point to the **working tree** (spans committed + uncommitted in one
  pass; adding a separate `..HEAD` numstat to a working-tree numstat would
  double-count lines that were committed and then further edited). Reported values
  are clamped `≥ 0` (a `git reset --hard` that moves the branch *below* its own
  `forkPointSha` could otherwise yield a negative/garbage count).
- `hasCommits` ← `git rev-list --count <forkPointSha>..HEAD > 0`

This is *not* a multi-writer hazard: there is exactly one command that writes
these fields, it is fully recompute-from-truth (idempotent), and Asynx serializes
it. The git usecase never sets `hasConflicts` "out of band" — it always goes
through the command. `hasCommits` is a **transient command input** (it decides the
`new`→null clear); it is **not** a stored field on the aggregate.

`lastActivity` is bumped by any command representing activity (the two
`SyncWorkingTreeState` issuers cover commits/file writes; `TouchActivity` covers
chat/agent activity). The aggregate therefore always holds a **complete** row, and
the Workspaces broadcaster emits that complete projected object. See §6.1 and
`03-realtime-websockets.md` §4. (The verbose `GitStatus` for the Git panel is a
separate, non-event-sourced channel — `04`/`05`.)

### 5.4 Chat (Asynx) — see `01-chat-lifecycle.md`

### 5.5 AgentRun (Asynx)

```
AgentRun {
  id        uuid
  wsId      uuid
  chatId    uuid
  status    AgentRunStatus
  createdAt time.Time
}
```

> **All AgentRun *writers* are bridge-owned (post-spike).** The command that
> creates an AgentRun and drives `pending → running → done|error|interrupted`
> lives in the deferred message-send/run surface (`12` §4.5). This spec fixes the
> aggregate's *shape* and its *crash recovery* (§6.2) and the *projections* it
> drives (Chat status `01` §5, Workspace `agent-running` overlay §6.1); the
> mutation commands themselves are designed in the Agentic Bridge spike.

### 5.6 TerminalProfile (GORM)

```
TerminalProfile {
  id               uuid
  name             string
  shell            string?
  startupDirectory string?
  startupCommands  []string
  icon             string?
  color            string?
}
```

### 5.7 Project Import — the writer for Repository (and adopted Workspaces)

`Repository` rows are not created by any workspace op — they are produced by the
**project-import** usecase behind `POST /v0/projects { name, path }` (UX §1). This
is the writer the rest of the model presupposes.

On import, the usecase:

1. Creates the `Project` row.
2. **Discovers git repos** under `path` (walk for `.git` directories, bounded
   depth) and creates a `Repository` row for each:
   - `defaultBranch` resolved per §5.2 (never empty).
   - `avatarLabel` / `avatarColor` are **generated** (not git-derived): `avatarLabel`
     = first alphanumeric char of the repo name (uppercased); `avatarColor` = a
     stable hash of the repo name into the theme's avatar-color palette.
3. **Adopts existing branches/worktrees** as `Workspace` rows. For each existing
   `git worktree` (and the primary checkout), it creates a Workspace with
   `branch`, `worktreePath`, `repoId`, `projectId`, and resolves `parentId` from
   the branch topology where determinable.
   - **`forkPointSha` for an adopted worktree** (no `git worktree add` event to
     record it) is the **one legitimate exception** to the "never recompute via
     `merge-base`" rule (§5.3): seed it as `git merge-base <branch> <parentBranch>`
     (or the repo `defaultBranch` for a root). From then on it is treated as the
     recorded fork point and never recomputed again.
   - `locked` resolved via the provider engine (`08`); the default/protected
     branch checkout is locked.

This makes every `Repository` and adopted `Workspace` field have a defined writer.

---

## 6. State Machines

### 6.1 Workspace

The Workspace aggregate owns the row the sidebar renders. The six UX badge states
map onto a small base lifecycle plus two overlays.

```
new ──► (status: null — has commits, no PR)
         │
         ├──► pr-open ──► pr-merged
         │           └──► pr-closed
         │
         ├─ overlay: agent-running  (a derived predicate: true iff ≥1 live
         │                           AgentRun for this ws — never a saved/restored
         │                           prior status; see below)
         │
         └─ overlay: hasConflicts   (recomputed from `git status` unmerged paths
                                      via SyncWorkingTreeState; cleared when resolved)
```

- Base `status` values: `new`, `pr-open`, `pr-merged`, `pr-closed`. A workspace is
  `new` until its **first commit**, then `null` (row shows only diff stats + age,
  no badge) until a PR appears. There is **no `active` state** — it was
  unreachable and is removed.
- **`status` is the one co-owned field, but its writers never fight** — they act
  in disjoint lifecycle phases and are guarded:
  - `SyncWorkingTreeState` carries `hasCommits` (detected as
    `git rev-list --count <forkPointSha>..HEAD > 0`). Its handler clears
    `status` **only if `status == new`** (`new → null`). It never touches a `pr-*`
    status. So a stray ref-watch firing after a PR opened cannot stomp `pr-open`
    back to null.
  - `SyncProviderState` only ever sets `pr-*` (you cannot have a PR without
    commits, so `new` is already gone by then). It sets **whatever the provider
    reports**, so `pr-closed → pr-open` (PR reopened) and `pr-open → pr-closed`
    are both allowed — there is no terminal `pr-*` state at the command level
    (the sweep treats closed/merged as terminal only as a polling optimization,
    `08` §4).
- `locked` is a **flag, not a state** — it gates deletion and cascade-delete,
  independent of `status`.
- `added` / `deleted` / `hasConflicts` are mutated **only** through
  `SyncWorkingTreeState` — issued by the watcher (debounced, change-only) *and* by
  git write usecases after a merge/rebase/pull/resolve (`04` §6), both recomputing
  from git (per-field sources in §5.3). One command, recompute-from-truth,
  serialized by Asynx → still single-source-of-truth (§5.3). They are *not* pushed
  to the broadcaster out-of-band.
- The `agent-running` badge is the live presence of an AgentRun projected onto the
  row (overlay), not a stored base status — the same string is a real `status`
  value on **Chat** (`01`) but only an overlay here. It is a **derived predicate
  `hasLiveAgentRun(ws)`** (true iff at least one AgentRun for the workspace is
  `running`), **not** a save/restore of a prior status. This is essential because a
  workspace can have **multiple** concurrent AgentRuns (many chats): the overlay
  clears only when the **last** live run ends, and the underlying `status`
  (`new`/null/`pr-*`) is never overwritten by the overlay, so there is nothing to
  "restore."

### 6.2 AgentRun

```
pending ──► running ──► done
                   ├──► error
                   └──► interrupted   (user clicked Stop)
```

**Crash recovery:** on startup, any AgentRun left in `running` is recovered to
`error` (same pattern as quiver.core's `ArrowRuntime` recovery — scan read model,
preload aggregate, send a fail command). The recovery command **flows through the
normal Asynx path**, so the AgentRun subscription fires and issues
`AgentRunCompleted` to the Chat projection — clearing its spinner. As a
belt-and-suspenders second pass, startup then **reconciles chats directly**: any
Chat in `agent-running` with no live AgentRun is reset to `idle`. Without this, a
chat mid-run at crash time would show a spinner forever.

Ordering and idempotency are defined to avoid a race between the two passes: the
direct Chat reconciliation runs **after** the AgentRun recovery commands have
**drained** (subscriptions processed), and the Chat idle-reset command is
**idempotent** (resetting an already-`idle` chat is a no-op). So whichever pass
clears a given chat first, the other is harmless.

### 6.3 ReviewThread

```
open ──► resolved ──► open      (re-openable)
```

### 6.4 Pull-Request State (provider-synced, not an aggregate)

PR state is **read-only, sourced from the git provider** (`gh`/`glab`) by the Git
Provider engine (`08-git-provider-engine.md`). Crowbar never creates or mutates
PRs. The engine polls (on-view + a slow background sweep of open-PR workspaces),
and on change issues a `SyncProviderState` command to the **Workspace**
aggregate, which updates `status` (`pr-open` / `pr-merged` / `pr-closed`) and the
`prUrl` / `prTitle` / `prTargetBranch` fields. The update then rides the normal
Workspace → hub → Workspaces-broadcaster path. There is no separate PR
aggregate and no PR-create endpoint.

---

## 7. Dependency-Injection Wiring

Hierarchical containers, mirroring quiver.core:

```
Root (internal.Container)
  ├── Engines     AI Bridge engine + addon registry
  ├── Adapters    event stores (one per Asynx aggregate) + GORM DB
  ├── App         Asynx instances, repositories, usecases, Hub
  │     └── RegisterHubProjections(hub)  — wires Asynx subscriptions → hub.BroadcastX
  └── API         v0 handlers + Broadcasters + routes
```

`app.New()` constructs the Asynx instances from the adapter event stores,
constructs repositories, calls `RegisterHubProjections(hub)` to connect domain
events to WebSocket broadcasts, then builds usecases. Crash recovery for
AgentRun runs synchronously before `app.New()` returns.

---

## 8. Out of Scope for This Spec Set

- **Conversation content** (turns, tool calls, widgets, agent TODO lists) —
  pending the Agentic Bridge spike.
- **Settings persistence** — the UX spec keeps all settings client-side
  (IndexedDB / localStorage). The only backend-stored settings are
  `TerminalProfile` rows, because the PTY layer needs them server-side. The UX
  "export/import all settings" (§14) is therefore client-side for everything
  *except* terminal profiles — those round-trip through the profiles API
  (`/v0/settings/terminal/profiles`, `06` §5), so an atomic export must include
  them via that API rather than silently omitting server-stored profiles.
- **Pane layout / open buffers / recent files** — client-only (IndexedDB), per
  UX spec §18.
