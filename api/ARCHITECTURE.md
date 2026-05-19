# Crowbar API — Architecture Reference

Module: `github.com/char2cs/crowbar/api`  
Go: 1.26.2  
Specs: `../docs/superpowers/specs/2026-05-19-*.md`

> **Note:** the scaffold has a module path inconsistency — some files import
> `rabbytesoftware/crowbar/api`. Fix all imports to `char2cs/crowbar/api`.

---

## Layer Order

```
cmd/crowbar/         CLI entry point (cobra, signal handling)
internal/internal.go Root container — wires all layers in order:
                       1. engine.New()
                       2. adapter.New()
                       3. app.New(engines, adapters)
                       4. api.New(appContainer)
```

Each layer receives only the layers below it. `api` knows about `app`;
`app` knows about `adapter` and `engine`; neither knows about `api`.

---

## Packages Defined by Specs

The specs explicitly define package layouts for five areas. Everything else
(app/repositories/*, app/usecases/*, api/v0/* handlers) is implementation-defined.

### `internal/engine/flow/` — Flow Engine
*(spec: 2026-05-19-flow-engine-design.md §1)*
```
flow.go                         FlowDefinition, StateDefinition, AgentDef,
                                TransitionDef, EmitDef, UIMode, IntelligenceLevel,
                                FlowTool (open string type = alias, not defined type)
loader.go                       Load(flowPath) → builtin shortcut or disk parse+validate
translator/
  translator.go                 Parse([]byte) → FlowDefinition
  raw.go                        flat YAML structs (one-to-one with YAML shape)
  mapper.go                     rawFlow → FlowDefinition
  schema/flow.json              embedded JSON schema (structural validation)
validator/validator.go          Validate(FlowDefinition) → []ValidationError (concurrent rules)
evaluator/evaluator.go          Evaluate(flow, currentState, event) → (string, bool)
builtin/feature_development.go  package-level FlowDefinition var
```

### `internal/engine/agent/` — Agent Runtime
*(spec: 2026-05-19-agent-runtime-design.md §1)*
```
agent.go                        AgentRuntime interface; ChatFrame + ChatFrameType (re-exported)
internal/
  registry/registry.go          model prefix → AgentEntry (bin + args)
  resolver/resolver.go          intelligence → bin + args via registry
  session/session.go            ACP session lifecycle (RegisterSession, drain, signal)
  drain/drain.go                ACP events → events.jsonl + publish func(ChatFrame)
  artifacts/artifacts.go        git diff → diff.patch
  prompt/prompt.go              system prompt + prior AgentRunOutput assembly
```

### `internal/api/v0/chat/` — Chat WebSocket
*(spec: 2026-05-19-agent-runtime-design.md §7)*
```
chat.go                         Hub interface (RegisterSession, Subscribe, Forward, Publish)
internal/handler/handler.go     ChatHandler — readPump + writePump
```

### `internal/api/v0/terminal/` — PTY Terminal
*(spec: 2026-05-19-agent-runtime-design.md §8)*
```
terminal.go                     TerminalHandler interface
internal/handler/handler.go     PTY WebSocket handler (creack/pty)
```

### `internal/app/repositories/mcp/` — MCP Server
*(spec: 2026-05-19-mcp-server-design.md §1)*
```
mcp.go                          MCPRepository interface; Mount(rg *gin.RouterGroup)
internal/
  server/server.go              mcp-go server setup, tool registration, route mount
  auth/auth.go                  X-Agent-Run-Token → AgentContext (AgentRun, Task, StateDefinition)
  tools/
    signal.go                   crowbar_signal
    items.go                    crowbar_create_item, update_item_status, get_items
    threads.go                  crowbar_open_thread, reply_thread, get_threads, resolve_thread
```

### `tests/` — Integration Suite
*(spec: 2026-05-19-integration-suite-design.md)*
```
tests/integration/
  lifecycle/ chat/ flow/ kanban/ threads/ mcp/
  worktree/ git/ websocket/ crash/ guards/ concurrency/
tests/kit/
  suite.go env.go client.go ws_client.go mcp_client.go
  fixtures.go git.go agent_stub.go
```
Build tag: `//go:build integration`. Run: `go test -tags integration -race ./tests/...`

---

## Key Wiring Files

### `internal/internal.go`
Assembles all containers. Must:
- Pass `engines` and `adapters` into `app.New()`
- Start `hub.Run(ctx)` goroutine after `app.New()`
- Pass `appContainer` into `api.New()`

### `internal/app/repositories/container.go`
Densest wiring. Must:
- Construct 3 GORM repos (Project, Repository, ConversationMessage from `adapter.Store`)
- Construct 4 Asynx repos (Task, AgentRun, KanbanItem, ReviewThread from Asynx event stores)
- Construct `mcp.New(agentRunRepo, taskRepo, kanbanRepo, threadRepo, chatHub, flowLoader)`
- Call `agentRunRepo.RecoverOrphanedRuns(ctx)` synchronously before returning
- Call `RegisterHubProjections(hub)` — wires Asynx callbacks → `hub.BroadcastX()`

### `internal/api/v0/router.go`
Must:
- Register all REST routes (see domain-crud spec §3 for full route list)
- Implement `dispatch()` for routes that serve both REST + WS on the same URL
- Register `WS /api/v0/tasks/:id/chat` → `chat.Handler`
- Register `WS /api/v0/tasks/:id/terminal` → `terminal.Handler`
- Call `mcpRepo.Mount(router.Group("/mcp"))`

---

## Storage Tiers

| Repo | Backend | Location |
|------|---------|----------|
| Project, Repository, ConversationMessage | GORM/SQLite | `~/.crowbar/state/store/crowbar.db` |
| Task | Asynx | `~/.crowbar/state/events/tasks.db` |
| AgentRun | Asynx | `~/.crowbar/state/events/agent_runs.db` |
| KanbanItem | Asynx | `~/.crowbar/state/events/kanban_items.db` |
| ReviewThread | Asynx | `~/.crowbar/state/events/review_threads.db` |
| AgentRun artifacts | Disk | `~/.crowbar/runs/{run_id}/{events.jsonl,diff.patch}` |

Asynx config: 8 shards, queue depth 1000 (same as Quiver).

---

## Core Utilities (domain-crud spec §1)

`core/metadata/` — embedded `metadata.yaml`; `{{home}}` template resolution; OS-specific path via `resolve_home.go` / `resolve_home_windows.go`  
`core/config/` — loads `~/.crowbar/config.yaml` over embedded defaults; singleton via `sync.Once`; exposes intelligence tier → model name mapping  
`core/paths/` — `Events()`, `Store()`, `Runs()`, `Logs()`; lazy `mkdir` with per-path `sync.Mutex`; every container accepts `WithHomeDir(dir)` for test isolation

---

## Asynx API Reference

Import: `github.com/char2cs/asynx` (v0.6.2 in Quiver). Reference implementations: `/Users/char2cs/Projects/Rabbyte/quiver.core/internal/app/repositories/arrow/arrow.go` and `quiver.core/internal/adapter/container.go`.

**Event store (SQLite adapter)**
```go
// One SQLite file per aggregate type
es, err := sqlite.NewEventStore(filepath.Join(eventsPath, "tasks.db"))
// Sets max open conns to 1 (serialised writes); WAL checkpoint on Close()
```

**Asynx instance**
```go
ax, err := asynx.New[domain.Task]().
    WithEventStore(es).
    WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
    Build()
```

**Sending commands**
```go
evt, err := ax.Send(ctx, cmd)     // non-blocking; returns immediately with event
evt, err := ax.SendWait(ctx, cmd) // blocking; waits for full handler cycle
```
Commands implement `asynxModels.Command[T]`. Errors: `asynxModels.ErrNotFound`, `ErrValidation`, `ErrPipelineFailed`.

**Reading aggregates** (Asynx reconstructs from events automatically)
```go
aggregate, err := ax.Get(ctx, aggregateID)
exists, err    := ax.Exists(ctx, aggregateID)
err            := ax.Preload(ctx, aggregateID) // warm the cache; no-op if already loaded
err            := ax.Forget(ctx, aggregateID)  // delete aggregate + fire OnForget callbacks
```

**Reactions / callbacks**
```go
subID, err := ax.Subscribe(asynx.Topic("task.state_advanced.*"), func(
    ctx context.Context,
    evt asynxModels.Event[domain.Task],
) {
    hub.BroadcastTask(evt.Aggregate)
})
subID, err := ax.OnForget(func(ctx context.Context, evt asynxModels.Event[domain.Task]) { ... })
err         := ax.Unsubscribe(subID)
```

**Crash recovery pattern** (see Quiver `internal/app/repositories/runtime/internal/recovery.go`)
```go
// On startup: scan GORM read-model, preload each aggregate, check status, send recovery command
for _, run := range runningAgentRuns {
    ax.Preload(ctx, run.ID)
    agg, _ := ax.Get(ctx, run.ID)
    if agg.Status == domain.AgentRunStatusRunning {
        ax.Send(ctx, FailAgentRun{ID: run.ID})
    }
}
```

**Shutdown**
```go
err := ax.Shutdown(ctx)
```

---

## ACP SDK Status

`github.com/coder/acp-go-sdk` is **not yet available** — not in the Go module cache, not in Quiver. The package is the programmatic interface for spawning Claude Code (and other AI CLIs) as subprocesses over the ACP protocol.

**What this means for implementation:**

- `internal/engine/agent/` cannot be fully implemented until the SDK ships.
- The `AgentStub` in `tests/kit/agent_stub.go` replaces it in all integration tests — tests run without the real SDK.
- Define `AgentRuntime` as an interface (already done in the spec); stub and real implementation are plug-compatible.
- When the SDK arrives, wire it behind the interface; no other code changes required.

**Expected SDK API surface** (inferred from agent-runtime spec §4–5):
```go
// Spawn agent subprocess; returns event stream + session handle
session, events, err := acpsdk.Spawn(ctx, bin, args, workdir)

// Send user message to running session
err := session.Send(content string)

// Close session (graceful)
err := session.Close()

// SessionUpdate — events streamed on the events channel
type SessionUpdate struct {
    Type    string // "text_chunk" | "turn_end" | "tool_call" | "tool_result" | "ping"
    Delta   string // for text_chunk
    // ... other fields per type
}
```

---

## go.mod Changes Needed

```
go 1.25.0 → 1.26.2

add:
  github.com/char2cs/asynx          // confirmed v0.6.2 in Quiver
  github.com/gorilla/websocket
  github.com/mark3labs/mcp-go
  github.com/coder/acp-go-sdk       // not yet available — add when published
  github.com/creack/pty
  github.com/stretchr/testify
```

---

## Scaffold Files to Delete / Rename

| Current | Action |
|---------|--------|
| `internal/domain/workspace.go` | Delete — entity is `Repository`, not `Workspace` |
| `internal/app/hub/hub.go` | Replace — generic event hub becomes typed `WebSocketHub` with `BroadcastTask`, `BroadcastAgentRun`, `BroadcastKanbanItem`, `BroadcastReviewThread` |
| `internal/api/v0/events.go` | Delete — SSE replaced by proper WebSocket Broadcaster pattern |
