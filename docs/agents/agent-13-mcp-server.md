# Agent 13 — MCP Server

**Working directory:** `/Users/char2cs/Projects/Rabbyte/crowbar/api`
**Module:** `github.com/char2cs/crowbar/api`

## Context

The MCP server is the coordination layer between AI agents and Crowbar's domain. It receives MCP tool calls from running agents, validates identity via the AgentRun token, and dispatches to domain repositories. Mounted as a Gin route group.

## Files to read before starting

- `docs/superpowers/specs/2026-05-19-mcp-server-design.md` — complete; this is your primary spec
- `api/ARCHITECTURE.md` §"app/repositories/mcp/"
- `github.com/mark3labs/mcp-go` documentation — check the installed package for its API

## What already exists

Agents 01–12 complete. All repositories implemented. ChatHub interface defined in `api/v0/chat/`. Flow engine implemented.

## Package layout

```
internal/app/repositories/mcp/
├── mcp.go
└── internal/
    ├── server/
    │   └── server.go
    ├── auth/
    │   └── auth.go
    └── tools/
        ├── signal.go
        ├── items.go
        └── threads.go
```

## Tasks

### `mcp.go`

```go
package mcp

import "github.com/gin-gonic/gin"

type MCPRepository interface {
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

`New` constructs the unexported `mcpRepository` struct and wires the MCP server internally.

### `internal/auth/auth.go`

```go
type AgentContext struct {
    AgentRun domain.AgentRun
    Task     domain.Task
    State    flow.StateDefinition
}

var ErrUnauthorized = errors.New("unauthorized")
var ErrForbidden = errors.New("forbidden")

func Resolve(
    token       string,
    agentRunRepo repositories.AgentRun,
    taskRepo     repositories.Task,
    flowLoader   flow.Loader,
) (AgentContext, error)
```

Steps:
1. `agentRunRepo.GetByToken(ctx, token)` → `ErrUnauthorized` if not found
2. Check `AgentRun.Status == running` → `ErrUnauthorized` otherwise
3. `taskRepo.Get(ctx, agentRun.TaskID)` → load task
4. `flowLoader.Load(task.FlowPath)` → load flow definition
5. Find `StateDefinition` where `state.Name == task.CurrentState`
6. Return `AgentContext{AgentRun, Task, State}`

### `internal/server/server.go`

Set up the `mcp-go` server. Register all 8 tools. Mount on the provided Gin router group.

Check the `mcp-go` library API — it likely has a `Server` type with `AddTool(name, description, handler)` or similar. Mount via `server.ServeHTTP` wrapped as a Gin handler, or use the library's built-in HTTP mounting if available.

All tool handlers receive the raw `X-Agent-Run-Token` header value (extract from HTTP request context) and call `auth.Resolve` before any business logic.

### `internal/tools/signal.go` — `crowbar_signal`

Parameters: `event string`, `output string (optional)`.

Execution:
1. `auth.Resolve(token)` → `AgentContext`
2. Check `"crowbar_signal"` in `ctx.State.Agent.Tools`; return `ErrForbidden` if not
3. `flow.Evaluate(loadedFlow, task.CurrentState, event)` → `nextState`
4. If no match: return error `"invalid event for current state"`
5. `agentRunRepo.CompleteAgentRun(ctx, agentRun.ID, output)`
6. `chatHub.Publish(task.ID, agent.ChatFrame{Type: agent.ChatFrameTypeStateTransition, NewState: nextState})`
7. `taskRepo.AdvanceState(ctx, task.ID, nextState, event)`
8. Return `{"ok": true, "next_state": nextState}`

### `internal/tools/items.go` — `crowbar_create_item`, `crowbar_update_item_status`, `crowbar_get_items`

See spec §5. Key points:
- `crowbar_create_item`: check `ctx.State.Items == true` before creating; return `ErrForbidden` otherwise
- `crowbar_update_item_status`: verify `item.TaskID == task.ID`; verify `status` is in `ctx.State` flow's `ItemStatuses`
- `crowbar_get_items`: no auth check beyond token validation

### `internal/tools/threads.go` — `crowbar_open_thread`, `crowbar_reply_thread`, `crowbar_get_threads`, `crowbar_resolve_thread`

See spec §5. Key points:
- `crowbar_open_thread`: derive `phase` from `ctx.State.Name` (`ai_review` → `ReviewPhaseAIReview`, else `ReviewPhaseHumanReview`); `opened_by = "reviewer"`
- `crowbar_reply_thread`: derive `role` from state name (`ai_review` → `"reviewer"`, `implementation` → `"implementer"`)
- `crowbar_resolve_thread`: only resolve if `thread.Status == open`; if already resolved, return success (no-op not error)

## Verification

```bash
cd /Users/char2cs/Projects/Rabbyte/crowbar/api && go build ./internal/app/repositories/mcp/...
go vet ./internal/app/repositories/mcp/...
```
