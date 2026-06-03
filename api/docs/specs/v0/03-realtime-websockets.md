# Crowbar Backend — Real-time / WebSocket Topology

> **Status:** Approved
> **Date:** 2026-06-03
> **Depends on:** `00-architecture-and-domain.md`, `02-api-surface.md`
> **Scope:** How live data reaches the frontend — the Hub → Broadcaster
> pattern, the two classes of event producers, channel scoping, and resource
> lifecycle. This is the backbone for every "live" behavior in UX spec §17.

---

## 1. The Core Pattern (from quiver.core)

Three components, each with one job:

```
producer ──►  Hub  ──fan-out──►  Broadcaster[T]  ──filtered──►  WS clients
           (BroadcastX)        (one per topic)    (by namespace)
```

1. **Hub** (`app/hub/`) — a typed interface `WebSocketHub` with one
   `BroadcastX` method per event kind, implemented as a subscriber fan-out. The
   hub is the only thing producers call; it knows nothing about WebSockets.

2. **Broadcaster[T]** (`api/v0/ws/`) — generic, **one instance per topic**.
   Holds connected clients, each with a predicate built from the request's URL
   params. A `StreamDef[T]` declares how to extract the namespace key and how to
   serialize `T`.
   ```go
   type StreamDef[T any] struct {
     Namespace func(T) string             // extract the routing key
     Serialize func(T) ([]byte, error)    // → JSON DTO
     Filters   []FilterDef[T]             // optional query-param predicates
   }
   ```

3. **Hub projections** (`app/repositories/container.go`) —
   `RegisterHubProjections(hub)` wires each producer to the matching
   `hub.BroadcastX`. This is the single place where domain events meet transport.

The hub is constructed in `app.New()`; `RegisterHubProjections` is called there,
after repositories are built and before usecases. Broadcasters live in the API
layer and register themselves as hub subscribers.

---

## 2. Two Classes of Event Producer

This is the central design point. Not every live event is a domain event.

### Class A — Asynx-driven (domain state)

Real state transitions of aggregates. They flow the canonical event-sourced
path and are crash-safe, replayable, and projected for free.

```
command → Asynx event → Asynx subscription → hub.BroadcastX → Broadcaster
```

Producers: **Workspace**, **Chat**.

### Class B — Process-driven (live machine observation)

Live observations of the real machine — a filesystem watcher, a language
server, a PTY. These are **not** aggregates. There is no event to source; the
machine *is* the source of truth. They publish **straight to the hub**, bypassing
Asynx entirely.

```
fsnotify / LSP / PTY → producer → hub.BroadcastX → Broadcaster
```

Producers: **FileWatcher**, **LSP client**, **PTY session**.

> Mixing the two is fine and expected — e.g. a file write (Class B) also causes
> the workspace's diff stats to change, which is broadcast on the Workspaces
> topic that otherwise carries Class A events. The broadcaster does not care
> which class produced the event.

---

## 3. The Seven Broadcasters

| Broadcaster | `T` | Subscription scope (namespace) | Payload identifies | Class |
|-------------|-----|-------------------------------|--------------------|-------|
| **Workspaces** | `Workspace` (full object) | **global** (optional `?repoId=` filter) | `id` | A + B |
| **Chats**      | `ChatStatusEvent`         | **`wsId`**     | `chatId`    | A |
| **Git**        | `GitStatus` (full object) | `wsId`         | `wsId`      | B |
| **Files**      | `FileChangeEvent`         | `wsId`         | `path`      | B |
| **LSP**        | `Diagnostic[]`            | `wsId`         | `wsId`      | B |
| **Terminal**   | `PTYFrame`                | `sessionId`    | `sessionId` | B |
| **ChatStream** | `ChatFrame` *(post-spike)* | `chatId`      | `chatId`    | (bridge) |

---

## 4. Channel Scoping — Global vs. Per-Workspace

The frontend needs **two different scopes**, and getting this right is what
keeps the sidebar live without over-subscribing.

### 4.1 Workspaces — globally scoped

The sidebar renders the **entire** tree at once (`Org > Repo > branches >
workspaces`), and every row shows live badges regardless of which workspace is
active. So the frontend subscribes **once** to `/v0/ws/workspaces` and receives
events for **every** workspace.

- Subscription scope: **global**
- Payload: the **full `Workspace` object** (Q2 decision: full object, not
  granular deltas — idempotent, frontend replaces the row by `id`)
- Carries: status, `added`/`deleted`, `hasConflicts`, `agent-running` overlay
- Optional `?repoId=` filter narrows to one repo, but the default is all.

### 4.2 Chats — scoped per workspace

The chats sidebar panel only shows chats for the **active** workspace. The
frontend subscribes to `/v0/ws/chats?wsId=X` and receives status changes for
**all chats within workspace X** — not one chat, not all chats globally.

- Subscription scope: **`wsId`**
- Payload: `ChatStatusEvent { chatId, wsId, status }` — the `chatId` lives in
  the payload so the frontend knows which row's spinner to toggle.

### 4.3 The symmetry

The same entity exposes a **status list** channel and a **content** channel at
different scopes:

| Entity | List channel (status, many rows) | Content channel (one item open) |
|--------|----------------------------------|----------------------------------|
| Workspace | `/v0/ws/workspaces` — **global** | open one → `/v0/ws/git`, `/v0/ws/files` etc. scoped `wsId` |
| Chat      | `/v0/ws/chats?wsId=` — **per workspace** | `/v0/ws/chats/:chatId/stream` — **per chat** (post-spike) |

You subscribe broadly to keep lists live, and narrowly when you open one item.

---

## 5. One Filesystem Event → Three Broadcasts

A single file write fans out to three topics. UX spec §17 requires this — the
Git panel, the file tree, and the workspace sidebar badges all update from the
same disk event.

```
fsnotify fires (debounced)
  └─► FileWatcher
        ├─► hub.BroadcastFile(FileChangeEvent)        → Files broadcaster   (wsId)
        ├─► recompute GitStatus
        │     └─► hub.BroadcastGit(GitStatus)          → Git broadcaster     (wsId)
        └─► recompute +N/-N (and hasConflicts)
              └─► hub.BroadcastWorkspace(Workspace)    → Workspaces broadcaster (global)
```

The FileWatcher is therefore the busiest producer. Recomputation is debounced so
a burst of writes (e.g. an agent editing many files) collapses into a bounded
number of broadcasts.

---

## 6. Resource Lifecycle — Lazy per Subscription (Q1 = A)

The per-workspace **file watcher** and **LSP client** are expensive (inotify
handles, a language-server subprocess). They start **lazily on first WS
subscription** and tear down when the last client disconnects.

```
first client connects to /v0/ws/files?wsId=X (or /v0/ws/git, /v0/ws/lsp)
  └─► start the workspace's FileWatcher / LSP client if not already running

last client for that workspace disconnects
  └─► stop the FileWatcher / LSP client, release handles
```

- Cheap when idle: a workspace nobody is viewing consumes no watcher/LSP
  resources.
- The Files, Git, and LSP topics for a given `wsId` share **one** underlying
  FileWatcher / LSP client — reference-counted across those subscriptions, not
  one per topic.
- Terminal sessions are independent: their lifecycle is tied to the PTY process
  (created via REST, killed via REST or on disconnect), not to subscription
  counting.

---

## 7. Wiring Summary

```
app.New()
  ├─ build Asynx instances (Workspace, Chat, AgentRun, ReviewThread)
  ├─ build repositories
  ├─ hub := hub.New()
  ├─ RegisterHubProjections(hub):
  │     Workspace Asynx sub   → hub.BroadcastWorkspace
  │     Chat Asynx sub        → hub.BroadcastChat
  │     AgentRun Asynx sub    → (drives Chat + Workspace agent-running overlay)
  │     Git Provider engine   → SyncProviderState → Workspace (pr-open/merged/closed)
  │                              (read-only poll; not an aggregate — see 08)
  └─ build usecases

api.New(app)
  └─ construct 7 Broadcasters, each registers as a hub subscriber
     Process-driven producers (FileWatcher, LSP, PTY) publish to the hub
     directly when started (lazily, per §6).
```

---

## 8. Out of Scope

- **ChatStream** frame protocol (`ChatFrame`) — post-spike
  (`12-agentic-bridge-spike.md`).
- Detailed `GitStatus`, `FileChangeEvent`, `Diagnostic`, `PTYFrame` payload
  shapes — in their respective subsystem specs.
```
