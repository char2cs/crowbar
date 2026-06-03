# Agent 14 — Wiring

**Working directory:** `/Users/char2cs/Projects/Rabbyte/crowbar/api`
**Module:** `github.com/char2cs/crowbar/api`

## Context

All individual packages are implemented. This agent wires them together into the five container files that compose the full daemon stack.

## Files to read before starting

- `api/ARCHITECTURE.md` §"Key Wiring Files", §"Layer Order", §"Storage Tiers", §"Asynx API Reference"
- `docs/superpowers/specs/2026-05-19-domain-crud-design.md` §3 (full route list)
- `docs/superpowers/specs/2026-05-19-mcp-server-design.md` §6
- `docs/superpowers/specs/2026-05-19-agent-runtime-design.md` §4 (execution steps), §7 (Hub interface)
- Quiver reference: `/Users/char2cs/Projects/Rabbyte/quiver.core/internal/internal.go`
- Quiver reference: `/Users/char2cs/Projects/Rabbyte/quiver.core/internal/app/container.go`
- Quiver reference: `/Users/char2cs/Projects/Rabbyte/quiver.core/internal/engine/container.go`

## What already exists

Agents 01–13 complete. Every individual package compiles independently.

## Files to write

### `internal/engine/container.go`

Constructs the flow loader and agent runtime. Accept `WithHomeDir` and `WithAgentRuntime` options.

```go
type Container struct {
    FlowLoader   flow.Loader
    AgentRuntime agent.AgentRuntime  // nil if not injected (ACP SDK not available)
}

type Option func(*engineOpts)
func WithHomeDir(dir string) Option
func WithAgentRuntime(rt agent.AgentRuntime) Option

func New(ctx context.Context, opts ...Option) (*Container, error)
```

`New`: construct `flow.NewLoader()`. If `WithAgentRuntime` option provided, use it; otherwise leave `AgentRuntime` nil (logged as a warning).

### `internal/adapter/container.go`

Fully implement per Agent 05 spec. Accept `WithHomeDir`. Opens GORM DB and four Asynx event stores. Expose `Close() error`.

### `internal/app/repositories/container.go`

```go
func New(
    ctx      context.Context,
    engines  *engine.Container,
    adapters *adapter.Container,
    chatHub  chat.Hub,
) (*Container, error)
```

Steps:
1. `newAsynx[domain.Task](adapters.TaskES)` → `axTask` (8 shards, depth 1000)
2. Same for `axAgentRun`, `axKanban`, `axThread`
3. Construct all GORM repos passing `adapters.Store`
4. Construct all Asynx repos passing the relevant `ax` instances
5. Construct `mcp.New(agentRunRepo, taskRepo, kanbanRepo, threadRepo, chatHub, engines.FlowLoader)`
6. `agentRunRepo.RecoverOrphanedRuns(ctx)` — synchronously
7. Return `&Container{...}`

`RegisterHubProjections(hub app_hub.WebSocketHub) error`:
- Subscribe `axTask` to `"task.*"` → `hub.BroadcastTask(evt.Aggregate)`
- Subscribe `axAgentRun` to `"agent_run.*"` → `hub.BroadcastAgentRun(evt.Aggregate)`
- Subscribe `axKanban` to `"kanban_item.*"` → `hub.BroadcastKanbanItem(evt.Aggregate)`
- Subscribe `axThread` to `"review_thread.*"` → `hub.BroadcastReviewThread(evt.Aggregate)`

### `internal/app/container.go`

```go
func New(
    ctx      context.Context,
    engines  *engine.Container,
    adapters *adapter.Container,
    opts     ...Option,
) (*Container, error)
```

1. Construct `chatHub := chat_impl.NewHub()` (the concrete Hub from api/v0/chat)
2. Construct `webSocketHub := hub.NewHub()` (the typed hub from app/hub)
3. Call `repositories.New(ctx, engines, adapters, chatHub)` → `repos`
4. Call `repos.RegisterHubProjections(webSocketHub)`
5. Return `&Container{Repos: repos, Hub: webSocketHub, ChatHub: chatHub}`

**Import cycle note:** `api/v0/chat` imports `engine/agent`. `app` imports `api/v0/chat` for the Hub implementation. If this creates a cycle with `api` importing `app`, move the `chat.Hub` interface to a shared package `internal/hub/chat/` with no upward imports, and have both the concrete implementation and the consumer import from there.

### `internal/api/container.go`

```go
func New(appContainer *app.Container) (*Container, error)
```

1. Create Gin engine with logger and recovery middleware
2. `wsHandler := ws.NewWSHandler()`
3. `appContainer.Hub.Register(wsHandler)` — wire hub → WS handler
4. `v0Router := v0.NewRouter(appContainer, wsHandler)`
5. `v0Router.Register(r.Group("/api/v0"))`
6. Mount MCP at root: `appContainer.Repos.MCP.Mount(r.Group("/mcp"))`
7. Expose `Run(net.Listener) error` and `Shutdown(ctx) error`

### `internal/api/v0/router.go`

`Register(rg *gin.RouterGroup)` — full route table:

```
POST   /projects                               → projectHandler.Create
GET    /projects                               → projectHandler.List
GET    /projects/:id                           → projectHandler.Get
DELETE /projects/:id                           → projectHandler.Delete

POST   /projects/:id/repositories             → repositoryHandler.Create
GET    /projects/:id/repositories             → repositoryHandler.List
GET    /repositories/:id                      → repositoryHandler.Get
DELETE /repositories/:id                      → repositoryHandler.Delete

POST   /repositories/:id/tasks               → taskHandler.Create
GET    /tasks/:id         [Dispatch]          → taskHandler.Get OR wsHandler.HandleTask
POST   /tasks/:id/archive                    → taskHandler.Archive
POST   /tasks/:id/pause                      → taskHandler.Pause
POST   /tasks/:id/resume                     → taskHandler.Resume
POST   /tasks/:id/retry                      → taskHandler.Retry
POST   /tasks/:id/force-transition           → taskHandler.ForceTransition

GET    /tasks/:id/agent-runs                 → agentRunHandler.List
POST   /agent-runs/:id/interrupt             → agentRunHandler.Interrupt

GET    /tasks/:id/kanban-items  [Dispatch]   → kanbanHandler.List OR wsHandler.HandleKanbanItems
GET    /tasks/:id/threads       [Dispatch]   → threadHandler.List OR wsHandler.HandleThreads
POST   /tasks/:id/threads                    → threadHandler.Open
POST   /threads/:id/messages                 → threadHandler.PostMessage
POST   /threads/:id/force-approve            → threadHandler.ForceApprove

GET    /tasks/:id/git/log                    → gitHandler.Log
GET    /tasks/:id/git/diff                   → gitHandler.Diff
GET    /tasks/:id/files                      → gitHandler.ListFiles
GET    /tasks/:id/messages                   → conversationHandler.List

WS     /tasks/:id/chat                       → chatHandler.Handle
WS     /tasks/:id/terminal                   → terminalHandler.Handle
```

Use `ws.Dispatch(restHandler, wsHandler)` for routes marked `[Dispatch]`.

### `internal/internal.go`

```go
type Container struct {
    Engines  *engine.Container
    Adapters *adapter.Container
    App      *app.Container
    API      *api.Container
}

func New(ctx context.Context, opts ...Option) (*Container, error)

func (c *Container) Start(ctx context.Context, host string) error
```

Layer order in `New`:
1. `engine.New(ctx, engineOpts...)`
2. `adapter.New(adapterOpts...)`
3. `app.New(ctx, engines, adapters, appOpts...)`
4. `api.New(appContainer)`

`Start`:
1. Open listener (Unix socket path from config, or `host` override)
2. `go api.Run(listener)` 
3. `<-ctx.Done()`
4. `api.Shutdown(5s timeout)`
5. `adapters.Close()`

## Verification

```bash
cd /Users/char2cs/Projects/Rabbyte/crowbar/api && go build ./...
```

All packages must compile. Fix any import cycles — move shared interfaces to a neutral package if needed.
