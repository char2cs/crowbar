# Crowbar — Domain CRUD Spec

**Date:** 2026-05-19
**Status:** Approved
**Sprint:** v0 — Initial Backend

---

## Overview

This spec defines the full persistence layer, REST API surface, Asynx aggregate design, and WebSocket broadcast pattern for all Crowbar domain entities. It is the foundation every other backend subsystem builds on.

**Out of scope for this sprint:** Memory entity, LanceDB/vector embeddings, improvement agents, Docker container pool, agent context injection.

**Updated:** Added `ConversationMessage` entity and chat WebSocket protocol.

---

## 1. Home Directory & Path Management

Mirrors Quiver's three-module pattern exactly:

- **`core/metadata/`** — declares all paths in an embedded `metadata.yaml`; resolves `{{home}}` templates; OS-specific home via `resolve_home.go` / `resolve_home_windows.go`
- **`core/config/`** — loads `~/.crowbar/config.yaml` (intelligence tier → model mapping) over embedded defaults; singleton via `sync.Once`
- **`core/paths/`** — lazy directory creation with per-path `sync.Mutex`; public functions (`Events()`, `Store()`, `Runs()`, `Logs()`); `WithHomeDir(dir)` option on every container for test isolation

**Directory layout:**

```
~/.crowbar/
├── state/
│   ├── events/               # Asynx event stores
│   │   ├── tasks.db
│   │   ├── agent_runs.db
│   │   ├── kanban_items.db
│   │   └── review_threads.db
│   └── store/                # GORM read models
│       └── crowbar.db        # Project + Repository tables
├── runs/                     # AgentRun artifacts: {run_id}/events.jsonl, diff.patch
├── logs/
│   └── crowbar.log
└── config.yaml               # Intelligence tier → model name mapping
```

**`metadata.yaml` path declarations:**

```yaml
paths:
  home:
    default: "~/.crowbar"
    windows: 'C:\Users\{{USER}}\Documents\.crowbar'
  events: "{{home}}/state/events"
  store:  "{{home}}/state/store"
  runs:   "{{home}}/runs"
  logs:   "{{home}}/logs"
  config: "{{home}}/config.yaml"
```

All directories created on first access via `paths.*()` functions. Config file is not created by `paths` — it is owned by the `config` package.

---

## 2. Entities & Storage Tiers

### Plain GORM (config records — no state machine)

Stored in `~/.crowbar/state/store/crowbar.db`. Auto-migrated via `gorm.AutoMigrate()` at startup.

**Project**

```go
type Project struct {
    ID          string    `gorm:"primaryKey"`
    Name        string    `gorm:"not null"`
    Description string
    CreatedAt   time.Time
}
```

**Repository**

```go
type Repository struct {
    ID          string    `gorm:"primaryKey"`
    ProjectID   string    `gorm:"not null;index"`
    Name        string    `gorm:"not null"`
    LocalPath   string    `gorm:"not null"`
    DefaultFlow string
    CreatedAt   time.Time
}
```

**ConversationMessage**

Append-only chat history for chat-state agent sessions. Plain GORM — no state machine, no Asynx. Stored in `crowbar.db` alongside Project and Repository.

```go
type ConversationRole string

const (
    ConversationRoleUser  ConversationRole = "user"
    ConversationRoleAgent ConversationRole = "agent"
)

type ConversationMessageType string

const (
    ConversationMessageTypeText       ConversationMessageType = "text"
    ConversationMessageTypeToolCall   ConversationMessageType = "tool_call"
    ConversationMessageTypeToolResult ConversationMessageType = "tool_result"
)

type ConversationMessage struct {
    ID         string                  `gorm:"primaryKey"`
    TaskID     string                  `gorm:"not null;index"`
    AgentRunID string                  // nullable — user messages have no AgentRun
    Role       ConversationRole        `gorm:"not null"`
    Type       ConversationMessageType `gorm:"not null"`
    MessageID  string                  // groups chunks of the same agent turn
    Content    string                  `gorm:"not null"` // full text for text type; JSON for tool_call/result
    CreatedAt  time.Time
}
```

Written in two places:
- **User message:** stored immediately when the chat WebSocket receives a user frame, before forwarding to the ACP session
- **Agent turn:** assembled from streaming chunks; written to SQLite when the turn ends (`agent_turn_end` frame). Tool calls and tool results written as individual rows as they arrive.

### Asynx Aggregates (state-bearing — event-sourced)

Each aggregate has its own SQLite event store under `~/.crowbar/state/events/`. One Asynx instance per aggregate type, configured with 8 shards and queue depth 1000 — same as Quiver.

**Task**

Carries `worktree_path` directly — no separate Worktree entity.

```go
type Task struct {
    ID            string
    RepoID        string
    Title         string
    FlowPath      string
    CurrentState  string
    Status        TaskStatus   // active | paused | complete
    BranchName    string
    BaseBranch    string
    StartSHA      string
    EndSHA        string       // set when Task reaches terminal state
    WorktreePath  string       // absolute path to git worktree on disk
    VisitedStates []string
    CreatedAt     time.Time
}
```

Commands: `CreateTask`, `AdvanceState`, `PauseTask`, `ResumeTask`, `CompleteTask(endSHA)`, `ArchiveTask`

Events: `TaskCreated`, `TaskStateAdvanced`, `TaskPaused`, `TaskResumed`, `TaskCompleted`, `TaskArchived`

**AgentRun**

```go
type AgentRun struct {
    ID           string
    TaskID       string
    Token        string          // MCP auth token; generated at session start; never serialised to REST/WS
    StateName    string
    Intelligence string
    Model        string
    Status       AgentRunStatus  // running | complete | failed | interrupted
    Output       string          // nullable; markdown summary from crowbar_signal()
    StartedAt    time.Time
    CompletedAt  *time.Time
}
```

Commands: `CreateAgentRun`, `CompleteAgentRun(output)`, `FailAgentRun`, `InterruptAgentRun`

Events: `AgentRunCreated`, `AgentRunCompleted`, `AgentRunFailed`, `AgentRunInterrupted`

**Crash recovery:** On startup, any AgentRun whose last event is `AgentRunCreated` (no terminal event follows) is marked `failed` automatically. Mirrors Quiver's runtime crash recovery pattern.

**KanbanItem**

```go
type KanbanItem struct {
    ID         string
    TaskID     string
    Title      string
    Status     string    // free-form; valid values defined by the Flow
    AgentRunID string    // nullable; the AgentRun currently working this item
    CreatedAt  time.Time
    UpdatedAt  time.Time
}
```

Commands: `CreateKanbanItem(taskID, title)`, `UpdateKanbanItemStatus(status, agentRunID)`

Events: `KanbanItemCreated`, `KanbanItemStatusUpdated`

**ReviewThread**

`ReviewMessage` entries are events within this aggregate — not a separate aggregate. Reading a thread's messages means replaying its event log in order.

```go
type ReviewThread struct {
    ID            string
    TaskID        string
    KanbanItemID  string          // nullable
    FilePath      string
    LineNumber    int
    Phase         ReviewPhase     // ai_review | human_review
    OpenedBy      string          // reviewer | human
    Status        ThreadStatus    // open | agreed | force_approved
    Messages      []ReviewMessage // rebuilt from events; not a separate table
    CreatedAt     time.Time
}

type ReviewMessage struct {
    ID        string
    ThreadID  string
    Role      ReviewMessageRole  // human | reviewer | implementer
    Content   string
    CreatedAt time.Time
}
```

Commands: `OpenThread(taskID, kanbanItemID?, filePath, lineNumber, phase, openedBy, content)`, `PostMessage(role, content)`, `ResolveThread(emoji?)`, `ForceApproveThread`

Events: `ThreadOpened`, `MessagePosted`, `ThreadResolved`, `ThreadForceApproved`

---

## 3. REST API Surface

All routes prefixed `/api/v0/`. Routes that have a WebSocket variant are served by a `dispatch()` function that checks the `Upgrade: websocket` header — same URL, two handlers.

### Projects

```
GET    /api/v0/projects
POST   /api/v0/projects
GET    /api/v0/projects/:id
PUT    /api/v0/projects/:id
DELETE /api/v0/projects/:id
```

### Repositories

```
GET    /api/v0/projects/:project_id/repositories
POST   /api/v0/projects/:project_id/repositories
GET    /api/v0/repositories/:id
PUT    /api/v0/repositories/:id
DELETE /api/v0/repositories/:id
```

### Tasks

```
GET    /api/v0/repositories/:repo_id/tasks
POST   /api/v0/repositories/:repo_id/tasks     ← runs git worktree add; records start_sha
GET    /api/v0/tasks/:id
DELETE /api/v0/tasks/:id                        ← archive: runs git worktree remove
POST   /api/v0/tasks/:id/pause
POST   /api/v0/tasks/:id/resume
POST   /api/v0/tasks/:id/retry
POST   /api/v0/tasks/:id/transition             ← force transition (user override, bypasses agent)
```

### AgentRuns

```
GET    /api/v0/tasks/:task_id/agent-runs
GET    /api/v0/agent-runs/:id
POST   /api/v0/agent-runs/:id/interrupt
```

### KanbanItems *(read-only from REST — writes via MCP tools only)*

```
GET    /api/v0/tasks/:task_id/kanban-items
```

### ReviewThreads & Messages

```
GET    /api/v0/tasks/:task_id/threads           ← ?status=open&phase=ai_review
POST   /api/v0/tasks/:task_id/threads           ← human opens a thread
GET    /api/v0/threads/:id
POST   /api/v0/threads/:id/messages             ← human posts a reply
POST   /api/v0/threads/:id/force-approve
```

### Git

```
GET    /api/v0/tasks/:id/git/log                ← commit history on task branch
GET    /api/v0/tasks/:id/git/diff               ← structured diff JSON (file/hunk/line)
```

Diff format: array of files, each containing an array of hunks, each containing an array of lines with old/new line numbers and a `type` (`context | added | removed`).

### File Explorer

```
GET    /api/v0/tasks/:id/files                  ← directory listing at worktree root
GET    /api/v0/tasks/:id/files/*path            ← raw file contents
```

### Conversation

```
GET    /api/v0/tasks/:id/messages               ← full conversation history (reload on reconnect)
WS     /api/v0/tasks/:id/chat                   ← bidirectional chat session (see §4)
```

---

## 4. WebSocket Channel Pattern

Mirrors Quiver's `Broadcaster[T]` pattern exactly. The app-layer `Hub` is version-agnostic; the v0 WS `Handler` implements `Subscriber` and bridges broadcasts to per-entity-type `Broadcaster[T]` instances.

### Broadcaster instances (v0 WS Handler)

| Broadcaster | Routes (dispatch — REST + WS on same URL) | Predicate |
|---|---|---|
| `Broadcaster[Task]` | `GET /api/v0/tasks`, `GET /api/v0/tasks/:id` | filter by `:id` if present |
| `Broadcaster[AgentRun]` | `GET /api/v0/tasks/:task_id/agent-runs`, `GET /api/v0/agent-runs/:id` | filter by `task_id` or `:id` |
| `Broadcaster[KanbanItem]` | `GET /api/v0/tasks/:task_id/kanban-items` | filter by `task_id` |
| `Broadcaster[ReviewThread]` | `GET /api/v0/tasks/:task_id/threads` | filter by `task_id`; optional `?phase`, `?status` query predicates |

### Hub Subscriber interface (app layer)

```go
type Subscriber interface {
    PushTask(domain.Task)
    PushAgentRun(domain.AgentRun)
    PushKanbanItem(domain.KanbanItem)
    PushReviewThread(domain.ReviewThread)
}
```

### Event flow

```
Asynx subscription fires
  → hub.BroadcastX(aggregate)
    → subscriber.PushX(aggregate)            // v0 WS Handler
      → broadcaster.Push(aggregate)
        → predicate(aggregate) per client
          → non-blocking send to client buffer (cap 64)
            → writePump → websocket.WriteMessage
```

Slow clients are dropped (non-blocking send, buffer full → skip). Same as Quiver.

### Chat WebSocket

`WS /api/v0/tasks/:id/chat` is a standalone handler — not connected to the domain Hub or any domain Broadcaster. It is the bidirectional pipe between the frontend chat UI and the running ACP session. The handler interacts with the `ChatHub` (defined in `internal/api/v0/chat/`) which fans out session frames to all connected WebSocket clients and routes client messages to the running session. Multiple clients may connect to the same task's chat simultaneously — all receive the same frames. Full protocol and ChatHub interface defined in the Agent Runtime spec.

**Frame envelope** (all frames share this shape, direction noted per type):

| `type` | Direction | Key payload fields |
|---|---|---|
| `user_message` | up | `content: string` |
| `agent_chunk` | down | `message_id: string`, `delta: string` |
| `agent_turn_end` | down | `message_id: string` |
| `tool_call` | down | `message_id: string`, `tool: string`, `args: object` |
| `tool_result` | down | `message_id: string`, `tool: string`, `result: object` |
| `state_transition` | down | `new_state: string` |

`crowbar_signal` tool calls receive prominent visual treatment on the frontend — they drive state transitions and should not be rendered as a regular collapsed tool card.

### Terminal WebSocket

`GET /api/v0/tasks/:id/terminal` is a standalone handler — not connected to the Hub or any Broadcaster. It wires PTY I/O directly over the WebSocket connection. Handled separately in the Agent Runtime spec.

### Domain WebSocket Event Envelope

All domain Broadcaster frames (Task, AgentRun, KanbanItem, ReviewThread) share a single envelope:

```json
{ "type": "<entity_type>", "data": { ...entity fields... } }
```

| `type` value | Payload |
|---|---|
| `"task"` | `domain.Task` (all fields; `Token` omitted) |
| `"agent_run"` | `domain.AgentRun` (all fields; `Token` omitted) |
| `"kanban_item"` | `domain.KanbanItem` |
| `"review_thread"` | `domain.ReviewThread` (includes `Messages` array) |

**Initial sync on WS connect:** the handler immediately sends the current state of all matching entities as one frame per entity before entering the streaming loop. The client processes initial frames identically to update frames — no special `init` flag. Clients replace their local copy on every received frame.

**`dispatch()` behaviour:** routes listed in the Broadcaster table are served by a single handler that inspects the `Upgrade: websocket` request header. Without `Upgrade`: responds with normal JSON (REST semantics). With `Upgrade: websocket`: performs the WS upgrade, sends initial sync frames, then streams Broadcaster output until the client disconnects.

---

## 5. Asynx Projection Registration

Projections are registered in `app/repositories/container.go` via a `RegisterHubProjections(hub)` call, identical to Quiver's `RegisterHubProjections` pattern. Each aggregate repository exposes typed `OnX` callback registration methods:

```go
// Task example
taskRepo.OnTaskStateAdvanced(func(ctx context.Context, t domain.Task) {
    hub.BroadcastTask(t)
})

// AgentRun crash recovery — registered at startup
agentRunRepo.ScanOrphanedRuns(ctx, func(run domain.AgentRun) {
    agentRunRepo.FailAgentRun(ctx, run.ID)
})
```

All four aggregates follow this pattern. Projections are the only place that calls `hub.BroadcastX()` — usecases never touch the hub directly.

**Orphaned run recovery** is separate from hub projections. `app.New()` calls `agentRunRepo.RecoverOrphanedRuns(ctx)` immediately after building repositories — before the HTTP server starts accepting connections. Any AgentRun whose last event is `AgentRunCreated` is transitioned to `failed` synchronously at startup.

---

## 6. Module & Key Dependencies

- **Module:** `github.com/char2cs/crowbar/api`
- **Go version:** 1.26.2
- **Key deps:** `gin-gonic/gin`, `gorm.io/gorm`, `glebarez/sqlite`, `char2cs/asynx`, `spf13/cobra`, `gorilla/websocket`
- **go.mod corrections needed before implementation:** bump Go version from `1.25.0` → `1.26.2`; add `char2cs/asynx`; add `gorilla/websocket`
- **Scaffolding spec correction:** module name in scaffolding spec reads `rabbytesoftware/crowbar/api` — actual module is `char2cs/crowbar/api` (already correct in go.mod)

---

## 7. HTTP Shapes

### Response conventions

- All responses are the domain struct (or `[]T` for lists) serialised as JSON — no separate DTOs.
- `AgentRun.Token` is **never** included in any REST or WebSocket serialisation (`json:"-"` tag or manual omission). It is internal to the engine layer.
- `POST` (create) → `201 Created` + entity body.
- `POST` (action: pause, resume, retry, interrupt, transition, force-approve) → `200 OK` + updated entity body.
- `PUT` → `200 OK` + updated entity body.
- `DELETE` → `204 No Content`.
- `GET` (single) → `200 OK` + entity, or `404` if not found.
- `GET` (list) → `200 OK` + array (empty array, not 404, when no items exist).

### Error envelope

```json
{ "error": "human-readable message" }
```

Validation errors (400) include a `details` array:

```json
{ "error": "validation failed", "details": ["TransitionTargetsExist: state 'foo' not found", "..."] }
```

### Request bodies

**`POST /api/v0/projects`**
```json
{ "name": "string (required)", "description": "string (optional)" }
```

**`PUT /api/v0/projects/:id`**
```json
{ "name": "string (optional)", "description": "string (optional)" }
```

**`POST /api/v0/projects/:project_id/repositories`**
```json
{ "name": "string (required)", "local_path": "string (required)", "default_flow": "string (optional)" }
```

**`PUT /api/v0/repositories/:id`**
```json
{ "name": "string (optional)", "local_path": "string (optional)", "default_flow": "string (optional)" }
```

**`POST /api/v0/repositories/:repo_id/tasks`**
```json
{ "title": "string (required)", "branch_name": "string (required)", "base_branch": "string (required)", "flow_path": "string (optional — empty uses builtin)" }
```

**`POST /api/v0/tasks/:id/transition`**
```json
{ "state": "string (required)" }
```

**`POST /api/v0/tasks/:task_id/threads`** (human opens thread)
```json
{ "file_path": "string (required)", "line_number": "int (required)", "content": "string (required)" }
```

**`POST /api/v0/threads/:id/messages`** (human posts reply)
```json
{ "content": "string (required)" }
```

All other `POST` action endpoints (pause, resume, retry, interrupt, force-approve) take **no request body**.
