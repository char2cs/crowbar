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
| `Workspace`    | Status state machine (new → pr-merged) + live overlays |
| `AgentRun`     | Lifecycle (pending → running → done/error/interrupted) + crash recovery |
| `ReviewThread` | open ↔ resolved transitions |
| `PullRequest`  | open → merged/closed |
| `Chat`         | idle ↔ agent-running; projections drive the sidebar for free |

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
> after the Agentic Bridge spike (see `11-agentic-bridge-spike.md`).

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

A workspace is a checkout of a git branch inside a repo. Workspaces nest under
repos and can be children of other workspaces (branch-off-branch).

```
Workspace {
  id           uuid
  repoId       uuid
  branch       string
  parentId     uuid?       // nested (fork-of-fork) workspaces
  status       WorkspaceStatus
  locked       bool        // base/protected branch — cannot be deleted
  hasConflicts bool        // overlay flag
  added        int         // +N lines vs parent (live, from git subsystem)
  deleted      int         // -N lines  (live, from git subsystem)
  createdAt    time.Time
}
```

`age` (the relative-time string the UI renders) is derived from `createdAt` /
last activity, not stored.

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

The Workspace aggregate owns the status badge the sidebar renders. Six badge
states from the UX spec map onto a base lifecycle plus two overlays.

```
new ──► active
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

- `locked` is a **flag, not a state** — it gates deletion and cascade-delete,
  independent of `status`.
- `added` / `deleted` diff stats and `hasConflicts` are **not** mutated through
  Asynx commands. The git subsystem computes them from filesystem events and
  pushes them to the workspace broadcaster directly. (Detailed in the git and
  real-time specs.)

Base status values: `new`, `active`, `pr-open`, `pr-merged`, `pr-closed`.
The `agent-running` badge is the live presence of an AgentRun, projected onto the
row; it is not a stored base status.

### 6.2 AgentRun

```
pending ──► running ──► done
                   ├──► error
                   └──► interrupted   (user clicked Stop)
```

**Crash recovery:** on startup, any AgentRun left in `running` is recovered to
`error` (same pattern as quiver.core's `ArrowRuntime` recovery — scan read model,
preload aggregate, send a fail command).

### 6.3 ReviewThread

```
open ──► resolved ──► open      (re-openable)
```

### 6.4 PullRequest

```
open ──► merged
    └──► closed
```

PR transitions drive the workspace status (`pr-open` / `pr-merged` /
`pr-closed`) via hub projection.

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
