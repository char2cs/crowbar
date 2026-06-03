# Agent 15 — Integration Test Suite

**Working directory:** `/Users/char2cs/Projects/Rabbyte/crowbar/api`
**Module:** `github.com/char2cs/crowbar/api`

## Context

The integration suite tests the full Crowbar backend in blackbox fashion. The complete daemon stack runs in-process against a real SQLite database and a real git repo. No mocked repositories.

## Files to read before starting

- `docs/superpowers/specs/2026-05-19-integration-suite-design.md` — complete; this is your primary spec
- `api/ARCHITECTURE.md` §"Integration Suite", §"ACP SDK Status"
- `docs/superpowers/specs/2026-05-19-agent-runtime-design.md` §7 (Hub interface, ChatFrame)
- `docs/superpowers/specs/2026-05-19-domain-crud-design.md` §7 (HTTP shapes, request bodies)
- Quiver reference: `/Users/char2cs/Projects/Rabbyte/quiver.core/tests/kit/` — read all files

## What already exists

Agents 01–14 complete. The full daemon stack wires and builds. Nothing in `tests/` exists yet.

## Build tag

Every file starts with `//go:build integration`. Never omit it.

**Run:** `go test -tags integration -race ./tests/...`

## Tasks

### `tests/kit/suite.go`

```go
//go:build integration

package kit

type IntegrationSuite struct {
    testify.Suite
    Env *Env
}

func (s *IntegrationSuite) SetupTest()    { s.Env = NewEnv(s.T()) }
func (s *IntegrationSuite) TeardownTest() { s.Env.Close() }
```

### `tests/kit/env.go`

```go
//go:build integration

package kit

type Env struct {
    Client    *Client
    WSClient  *WSClient
    MCPClient *MCPClient
    Stub      *AgentStub
    close     func()
}

func NewEnv(t *testing.T) *Env
func NewEnvWithHome(t *testing.T, homeDir string) *Env
```

`NewEnv(t)` calls `NewEnvWithHome(t, t.TempDir())`.

`NewEnvWithHome`:
1. `stub := NewAgentStub()`
2. Wire full stack:
   ```go
   engines, _ := engine.New(ctx, engine.WithHomeDir(dir), engine.WithAgentRuntime(stub))
   adapters, _ := adapter.New(adapter.WithHomeDir(dir))
   appContainer, _ := app.New(ctx, engines, adapters, app.WithHomeDir(dir))
   apiContainer, _ := api.New(appContainer)
   ```
3. Start Gin on Unix socket at `filepath.Join(dir, "crowbar.sock")`
4. Return `Env` with typed clients pointed at that socket
5. `close` func: cancel ctx, wait for server shutdown, `adapters.Close()`, remove socket

### `tests/kit/client.go`

REST client over Unix socket. Custom `http.Transport` with `DialContext` using `net.Dial("unix", sockPath)`. Base URL: `http://unix`.

Implement all typed methods from the spec §2 client.go section. Each method:
- Marshals request body to JSON
- Sends HTTP request
- Asserts 2xx via `t.Fatalf` on error
- Unmarshals and returns domain struct

Helper request types:
```go
type CreateProjectRequest struct { Name string `json:"name"` }
type CreateRepositoryRequest struct { Name string `json:"name"`; Path string `json:"path"` }
type CreateTaskRequest struct { Title string `json:"title"`; BranchName string `json:"branch_name"`; BaseBranch string `json:"base_branch"`; FlowPath string `json:"flow_path"` }
type OpenThreadRequest struct { File string `json:"file"`; Line int `json:"line"`; Content string `json:"content"` }
```

### `tests/kit/ws_client.go`

WebSocket client over Unix socket. Use `gorilla/websocket` with `NetDial` set to `net.Dial("unix", sockPath)`.

Implement `SubscribeTasks`, `SubscribeAgentRuns`, `SubscribeKanbanItems`, `SubscribeThreads`, `ConnectChat`. Each Subscribe:
- Dials the WS endpoint
- Reads initial sync frame
- Goroutine unmarshals incoming messages into typed channel (cap 64)
- Returns channel + close func

`ChatConn`:
```go
type ChatConn struct{ conn *websocket.Conn; frames chan agent.ChatFrame }
func (c *ChatConn) Send(content string) error
func (c *ChatConn) Frames() <-chan agent.ChatFrame
```

### `tests/kit/mcp_client.go`

Calls MCP tools over HTTP (Streamable HTTP transport) on the Unix socket. `WithToken(token string) *MCPClient` returns a copy with the token pinned in `X-Agent-Run-Token`.

Each method sends JSON-RPC `tools/call` to `/mcp` and parses the response.

```go
type MCPClient struct {
    httpClient *http.Client
    token      string
}
func (c *MCPClient) WithToken(token string) *MCPClient
func (c *MCPClient) Signal(event string, output string) error
func (c *MCPClient) CreateItem(title string) (string, error)
func (c *MCPClient) UpdateItemStatus(itemID string, status string) error
func (c *MCPClient) GetItems() ([]domain.KanbanItem, error)
func (c *MCPClient) OpenThread(file string, line int, content string) (string, error)
func (c *MCPClient) ReplyThread(threadID string, content string) error
func (c *MCPClient) GetThreads(status string, phase string) ([]domain.ReviewThread, error)
func (c *MCPClient) ResolveThread(threadID string, emoji string) error
```

### `tests/kit/agent_stub.go`

Implements `engine/agent.AgentRuntime`.

```go
//go:build integration

package kit

type AgentStub struct {
    mu       sync.Mutex
    sessions map[string]*stubSession
    waiting  chan string  // sends token when session starts
}

type stubSession struct {
    token   string
    publish func(agent.ChatFrame)
    done    chan struct{}
    fail    chan struct{}
}

func NewAgentStub() *AgentStub

func (s *AgentStub) Run(ctx context.Context, run domain.AgentRun, task domain.Task, state flow.StateDefinition) error
func (s *AgentStub) WaitForRun(ctx context.Context) string
func (s *AgentStub) StreamChunks(chunks ...string)
func (s *AgentStub) Fail()
```

`Run`:
1. Generate a UUID token
2. Register session in map
3. Send token on `waiting` channel
4. Block until `done`, `fail`, or `ctx.Done()`
5. On `fail`: return error
6. On `ctx.Done()`: return ctx.Err()

`WaitForRun`: read from `waiting` with `time.After(2*time.Second)` timeout; call `t.Fatal` on timeout.

`StreamChunks`: calls `session.publish` with `agent_chunk` frames, then `agent_turn_end`.

`Fail`: closes `session.fail`.

### `tests/kit/fixtures.go`

```go
//go:build integration

package kit

func DefaultProject(c *Client) domain.Project
func DefaultRepository(c *Client, projectID string, repoPath string) domain.Repository
func DefaultTask(c *Client, repoID string, branch string) domain.Task
```

`DefaultProject`: `c.CreateProject(CreateProjectRequest{Name: fmt.Sprintf("test-%d", time.Now().UnixNano())})`.

`DefaultTask`: `branch` is the branch name; `base_branch` defaults to `"main"`; `flow_path` is `""` (builtin).

### `tests/kit/git.go`

```go
//go:build integration

package kit

type GitRepo struct{ t *testing.T; path string }

func NewGitRepo(t *testing.T) *GitRepo
func (r *GitRepo) Path() string
func (r *GitRepo) Commit(message string, files map[string]string) string
```

`NewGitRepo`: `git init` + `git config user.email test@test.com` + `git config user.name Test` + initial empty commit in `t.TempDir()`.

`Commit`: write files to disk, `git add -A`, `git commit -m <message>`, return `git rev-parse HEAD`.

Use `exec.Command` with `t.Fatal` on any non-zero exit.

### Integration suites — all 12

Write all suite files. Each file:
```go
//go:build integration

package <suite>

import (
    "testing"
    "github.com/stretchr/testify/suite"
    "github.com/char2cs/crowbar/api/tests/kit"
)

func TestXxx(t *testing.T) { suite.Run(t, new(XxxSuite)) }

type XxxSuite struct{ kit.IntegrationSuite }
```

**lifecycle/** — full Task state machine through all 7 states using `Stub.WaitForRun` + `Stub.FireSignal`. Assert WS `task` events for each transition. Assert `AgentRun.Outputs` accumulate.

**chat/** — `ChatConn.Send` → SQLite ConversationMessage. `Stub.StreamChunks` → WS frames received. Reconnect and reload history via `GetMessages`.

**flow/** — Load builtin (7 states, 1 terminal). YAML missing `name` → structural error. Transition to nonexistent state → `TransitionTargetsExist` error. No terminal state → `AtLeastOneTerminal` error. `Evaluate("brainstorming", "user_approved")` → `"spec"`.

**kanban/** — Force-transition to `implementation`. `MCPClient.CreateItem` → in ListKanbanItems + WS event. `UpdateItemStatus` → status updated + WS event. `CreateItem` in `brainstorming` state → 403.

**threads/** — Human opens thread (REST) → `opened_by: human`. Agent replies (MCP) → `role: implementer`. `ResolveThread` → `status: agreed`. `ForceApproveThread` → `status: force_approved`. `GetThreads` filtering by status/phase.

**mcp/** — Unknown token → 401. Completed run token → 401. Tool not in state → 403. Invalid event → structured error. Valid `crowbar_signal` → task advances + `state_transition` frame on chat WS.

**worktree/** — `CreateTask` → worktree directory exists. `git worktree list` confirms it. `ArchiveTask` → directory removed. Duplicate branch name → error.

**git/** — 3-commit `GitRepo` + one agent commit via worktree. `GetGitLog` returns all SHAs in chronological order. `GetGitDiff` returns correct file names and hunk counts.

**websocket/** — Per-task isolation (task A events don't reach task B subscriber). Unsubscribe stops delivery. Slow consumer doesn't block other subscribers.

**crash/** — Does NOT embed `IntegrationSuite`. Uses `NewEnvWithHome` with shared `homeDir`. First env starts task, confirms AgentRun running, then `Close()` without signal. Second env restarts; asserts orphaned run is `failed` and task is `paused`. `RetryTask` creates new run.

**guards/** — Nonexistent repoID on create → 404. Invalid flow YAML → 400 with details. ForceTransition to unknown state → 400. Invalid item status → 400. `GET /tasks/nonexistent` → 404.

**concurrency/** — 10 goroutines creating tasks simultaneously → all succeed, distinct worktree paths. 5 goroutines creating kanban items under same AgentRun token → all 5 items, no duplicates.

## Verification

```bash
cd /Users/char2cs/Projects/Rabbyte/crowbar/api && go build -tags integration ./tests/...
go vet -tags integration ./tests/...
```

Do NOT run the tests — no real ACP subprocess in the build environment. Clean compile + vet is the goal.
