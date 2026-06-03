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
| `Repository`      | id, projectId, name, path, avatarLabel, avatarColor |
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
"last touched" label (UX §1) stays accurate.

### 5.2 Repository (GORM)

```
Repository {
  id          uuid
  projectId   uuid
  name        string
  path        string       // absolute path on disk
  avatarLabel string       // single char for the avatar badge
  avatarColor string       // color class for the badge background
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
  branch       string
  worktreePath string      // the git worktree directory on disk
  forkPointSha string      // commit the branch was created from (recorded at worktree add)
  parentId     uuid?       // nested (fork-of-fork) workspaces
  status       WorkspaceStatus?   // new | pr-open | pr-merged | pr-closed; null once it has commits but no PR
  locked       bool        // provider-protected branch — chat-only, cannot delete/merge-into
  hasConflicts bool        // summary overlay (from git subsystem, via SyncWorkingTreeState)
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
aggregate and mutated **only through Asynx commands** — `SyncWorkingTreeState`
(watcher: `added`/`deleted`/`hasConflicts`, debounced + emitted only on change),
`SyncProviderState` (provider poller: PR fields + `locked`), and the
status/hierarchy commands. The aggregate therefore always holds a **complete**
row, and the Workspaces broadcaster emits that complete projected object — there
is no second producer writing a half-populated object. See §6.1 and
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
         ├─ overlay: agent-running  (set while any AgentRun for this ws is live;
         │                           previous status restored when it stops)
         │
         └─ overlay: hasConflicts   (set by git subsystem on merge/rebase
                                      conflict; cleared when all resolved)
```

- Base `status` values: `new`, `pr-open`, `pr-merged`, `pr-closed`. A workspace is
  `new` until its **first commit** (UX §2: "new = just created, no commits yet"),
  at which point a command clears `status` to `null` (the row then shows only diff
  stats + age, no badge) until a PR appears. There is **no `active` state** — it
  was unreachable and is removed.
- `locked` is a **flag, not a state** — it gates deletion and cascade-delete,
  independent of `status`.
- `added` / `deleted` / `hasConflicts` **are** mutated through Asynx commands
  (`SyncWorkingTreeState`, issued by the watcher — debounced, change-only). This
  keeps the aggregate the single complete source of truth (§5.3). They are *not*
  pushed to the broadcaster out-of-band.
- The `agent-running` badge is the live presence of an AgentRun projected onto the
  row (overlay), not a stored base status — the same string is a real `status`
  value on **Chat** (`01`) but only an overlay here.

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
belt-and-suspenders second pass, startup also **reconciles chats directly**: any
Chat in `agent-running` with no live AgentRun is reset to `idle`. Without this, a
chat mid-run at crash time would show a spinner forever.

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
  `TerminalProfile` rows, because the PTY layer needs them server-side.
- **Pane layout / open buffers / recent files** — client-only (IndexedDB), per
  UX spec §18.
```
