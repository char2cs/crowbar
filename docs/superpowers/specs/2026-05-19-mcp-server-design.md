# Crowbar — MCP Server Spec

**Date:** 2026-05-19
**Status:** Approved
**Sprint:** v0 — Initial Backend

---

## Overview

The MCP repository lives in `app/repositories/mcp/` and is the coordination layer between AI agents and Crowbar's domain. It receives MCP tool calls from running agents, validates the calling agent's identity, and dispatches to the appropriate domain repositories. It is wired in `repositories/container.go` alongside the task, agentrun, kanban, and thread repositories — receiving them as injected dependencies.

This is the same pattern as Quiver's `graph/` repository: a coordinator that takes multiple repositories as dependencies and exposes a clean interface. The MCP protocol is its transport, not its identity.

**Key library:** `github.com/mark3labs/mcp-go` — MCP server implementation for Go.

**Out of scope this sprint:** `crowbar_write_memory` (Memory out of scope), improvement agent tool authorization.

---

## 1. Package Layout

```
internal/app/repositories/mcp/
├── mcp.go                          // MCPRepository interface (exported) + re-exports
└── internal/
    ├── server/
    │   └── server.go               // MCP server setup, tool registration, Gin route mount
    ├── auth/
    │   └── auth.go                 // AgentRun token → (AgentRun, Task) resolution
    └── tools/
        ├── signal.go               // crowbar_signal
        ├── items.go                // crowbar_create_item, update_item_status, get_items
        └── threads.go              // crowbar_open_thread, reply_thread, get_threads, resolve_thread
```

`mcp.go` exports the `MCPRepository` interface and re-exports types consumers need. The unexported `mcpRepository` struct holds injected dependencies and implements the interface.

---

## 2. Transport & Mounting

The MCP server runs as an HTTP endpoint mounted on the existing Gin daemon — no separate port, no separate process. Routes are registered at `/mcp` by the API container calling `mcpRepo.Mount(routerGroup)`.

**MCP transport:** Streamable HTTP (the current MCP standard). The `mcp-go` library handles the protocol framing — tool listing, tool call dispatch, and result encoding.

**MCP config injected into each ACP session:**

```json
{
  "mcpServers": {
    "crowbar": {
      "url": "http://localhost/mcp",
      "headers": { "X-Agent-Run-Token": "{agent_run_token}" }
    }
  }
}
```

The token is generated per AgentRun at session start by `engine/agent`. It is stored on the `AgentRun` record and used by `auth` to resolve the calling context on every tool call.

---

## 3. Agent Identity (`internal/auth/auth.go`)

Every tool call arrives with `X-Agent-Run-Token` in the request header. The `auth` package resolves this to the full execution context before any tool handler runs.

```go
type AgentContext struct {
    AgentRun domain.AgentRun
    Task     domain.Task
    State    flow.StateDefinition  // resolved from Task.CurrentState + loaded flow
}

func Resolve(
    token       string,
    agentRunRepo repositories.AgentRun,
    taskRepo     repositories.Task,
    flowLoader   flow.Loader,
) (AgentContext, error)
```

**Token validation:**
1. Look up `AgentRun` by token — returns `ErrUnauthorized` if not found
2. Verify `AgentRun.Status == running` — stale or completed tokens are rejected
3. Load `Task` from `AgentRun.TaskID`
4. Load `FlowDefinition` via `flow.Load(task.FlowPath)`
5. Resolve `StateDefinition` for `task.CurrentState`
6. Return `AgentContext`

**Tool authorization:** every tool handler checks that the requested tool is in `ctx.State.Agent.Tools` before executing. Calls to tools not declared in the state's tool list return `ErrForbidden`. This enforces the Flow's declared tool surface at runtime.

---

## 4. MCPRepository Interface

```go
type MCPRepository interface {
    // Mount registers the MCP HTTP routes on the provided Gin router group.
    Mount(rg *gin.RouterGroup)
}

func New(
    agentRunRepo repositories.AgentRun,
    taskRepo     repositories.Task,
    kanbanRepo   repositories.KanbanItem,
    threadRepo   repositories.ReviewThread,
    chatHub      chat.Hub,
    flowLoader   flow.Loader,
) MCPRepository
```

`chatHub` is the `chat.Hub` defined in `internal/api/v0/chat/`. `crowbar_signal` calls `chatHub.Publish(task.ID, stateTransitionFrame)` to push a `state_transition` frame to all connected WebSocket clients before the session closes. If no clients are subscribed, the publish is a no-op.

---

## 5. Tool Definitions

All tools follow the same shape: `auth.Resolve(token)` → check tool authorization → execute → return result.

---

### `crowbar_signal` (`tools/signal.go`)

**Signature:** `crowbar_signal(event: string, output?: string)`

**Purpose:** Trigger a state transition or emit an event. `output` is a markdown summary stored on the AgentRun and injected into all subsequent states' context.

**Execution:**
1. `auth.Resolve(token)` → `AgentContext`
2. `flow.Evaluate(ctx.State.Flow, task.CurrentState, event)` → `nextState`
3. If no matching transition: return error (invalid event for current state)
4. `agentRunRepo.CompleteAgentRun(ctx.AgentRun.ID, output)` — stores output, marks complete
5. Push `state_transition` frame to `chatHub` for this task
6. `taskRepo.AdvanceState(task.ID, nextState, event)` — updates Task aggregate
7. Return `{ok: true, next_state: nextState}`

`crowbar_signal` is the only tool that drives state transitions. All other tools are read/write operations on domain entities within the current state.

---

### `crowbar_create_item` (`tools/items.go`)

**Signature:** `crowbar_create_item(title: string, description?: string) → {item_id: string}`

**Purpose:** Create a KanbanItem for the current Task. Only valid in states where `items: true`.

**Execution:**
1. `auth.Resolve(token)` → `AgentContext`
2. Check `ctx.State.Items == true` — return `ErrForbidden` otherwise
3. `kanbanRepo.CreateKanbanItem(task.ID, title)` → `item_id`
4. Return `{item_id}`

---

### `crowbar_update_item_status` (`tools/items.go`)

**Signature:** `crowbar_update_item_status(item_id: string, status: string)`

**Purpose:** Move a KanbanItem to a new status.

**Execution:**
1. `auth.Resolve(token)` → `AgentContext`
2. Verify `item.TaskID == task.ID` — agents cannot update items from other tasks
3. Verify `status` is in `ctx.State.Flow.ItemStatuses` — rejects unknown status values
4. `kanbanRepo.UpdateKanbanItemStatus(item_id, status, agentRun.ID)`

---

### `crowbar_get_items` (`tools/items.go`)

**Signature:** `crowbar_get_items() → Item[]`

**Purpose:** List all KanbanItems for the current Task.

**Execution:**
1. `auth.Resolve(token)` → `AgentContext`
2. `kanbanRepo.ListByTask(task.ID)` → `[]KanbanItem`
3. Return serialised item list

---

### `crowbar_open_thread` (`tools/threads.go`)

**Signature:** `crowbar_open_thread(file: string, line: int, content: string) → {thread_id: string}`

**Purpose:** Open a ReviewThread anchored to a file and line. Crowbar sets `phase` and `opened_by` automatically from the calling agent's state context.

**Execution:**
1. `auth.Resolve(token)` → `AgentContext`
2. Derive `phase` from `ctx.State.Name`: `ai_review` → `ReviewPhaseAIReview`; any other state → `ReviewPhaseHumanReview` (defensive fallback — agents in non-review states should not open threads, but this is not blocked at the tool layer)
3. `threadRepo.OpenThread(task.ID, nil, file, line, phase, "reviewer", content)` → `thread_id`
4. Return `{thread_id}`

---

### `crowbar_reply_thread` (`tools/threads.go`)

**Signature:** `crowbar_reply_thread(thread_id: string, content: string)`

**Purpose:** Append a message to a ReviewThread. Role is set from the calling agent's state.

**Execution:**
1. `auth.Resolve(token)` → `AgentContext`
2. Verify `thread.TaskID == task.ID`
3. Derive `role` from `ctx.State.Name`: `ai_review` → `reviewer`; `implementation` → `implementer`
4. `threadRepo.PostMessage(thread_id, role, content)`

---

### `crowbar_get_threads` (`tools/threads.go`)

**Signature:** `crowbar_get_threads(status?: string, phase?: string) → Thread[]`

**Purpose:** List ReviewThreads for the current Task. Filterable by `status` and `phase`.

**Execution:**
1. `auth.Resolve(token)` → `AgentContext`
2. `threadRepo.ListByTask(task.ID, status, phase)` → `[]ReviewThread` with messages included
3. Return serialised thread list

---

### `crowbar_resolve_thread` (`tools/threads.go`)

**Signature:** `crowbar_resolve_thread(thread_id: string, emoji?: string)`

**Purpose:** Mark a thread `agreed`. Only callable by agents — human resolution is done via the UI (`POST /api/v0/threads/:id/force-approve`). If `emoji` is provided, the frontend renders it as an emoji react bubble instead of a plain checkmark.

**Execution:**
1. `auth.Resolve(token)` → `AgentContext`
2. Verify `thread.TaskID == task.ID`
3. Verify `thread.Status == open` — resolving an already-resolved thread is a no-op, not an error
4. `threadRepo.ResolveThread(thread_id, emoji)`

---

## 6. Wiring in `repositories/container.go`

```go
mcpRepo := mcp.New(
    agentRunRepo,
    taskRepo,
    kanbanRepo,
    threadRepo,
    chatHub,
    flowLoader,
)
```

`mcpRepo.Mount(router.Group("/mcp"))` is called from the API container after all repositories are constructed — same pattern as how API v0 handlers are wired.

---

## 7. Key Dependencies to Add

- `github.com/mark3labs/mcp-go` — MCP server library
