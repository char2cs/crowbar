# Crowbar — Integration Suite Spec

**Date:** 2026-05-19
**Status:** Approved
**Sprint:** v0 — Initial Backend

---

## Overview

The integration suite tests the full Crowbar backend in blackbox fashion — the complete daemon stack runs in-process against a real SQLite database, real git operations, and a real MCP server. No mocked repositories, no mocked engines, no unit-test substitutes. The suite is the contract.

Mirrors Quiver's `tests/` structure: a shared `kit/` test harness plus per-concern suites in `tests/integration/`.

**Build tag:** `//go:build integration` on all files.
**Run:** `go test -tags integration -race ./tests/...`

**AI agents do not run in CI.** The `AgentStub` replaces `engine/agent.AgentRuntime` in the engine container for all tests — it is a controllable fake that fires signals, streams chunks, and provides synthesized AgentRun tokens on demand.

---

## 1. Directory Layout

```
tests/
├── integration/
│   ├── lifecycle/
│   │   └── lifecycle_test.go
│   ├── chat/
│   │   └── chat_test.go
│   ├── flow/
│   │   └── flow_test.go
│   ├── kanban/
│   │   └── kanban_test.go
│   ├── threads/
│   │   └── threads_test.go
│   ├── mcp/
│   │   └── mcp_test.go
│   ├── worktree/
│   │   └── worktree_test.go
│   ├── git/
│   │   └── git_test.go
│   ├── websocket/
│   │   └── websocket_test.go
│   ├── crash/
│   │   └── crash_test.go
│   ├── guards/
│   │   └── guards_test.go
│   └── concurrency/
│       └── concurrency_test.go
└── kit/
    ├── suite.go
    ├── env.go
    ├── client.go
    ├── ws_client.go
    ├── mcp_client.go
    ├── fixtures.go
    ├── git.go
    └── agent_stub.go
```

Each suite file starts with:
```go
//go:build integration

func TestXxx(t *testing.T) { testify.Run(t, new(XxxSuite)) }
```

---

## 2. Test Kit (`tests/kit/`)

### `suite.go` — base suite

```go
type IntegrationSuite struct {
    testify.Suite
    Env *Env
}

func (s *IntegrationSuite) SetupTest() {
    s.Env = NewEnv(s.T())
}

func (s *IntegrationSuite) TeardownTest() {
    s.Env.Close()
}
```

Every suite embeds `IntegrationSuite`. `SetupTest` creates a fresh isolated environment per test — no shared state between tests.

---

### `env.go` — in-process server

```go
type Env struct {
    Client    *Client
    WSClient  *WSClient
    MCPClient *MCPClient
    Stub      *AgentStub
    close     func()
}

func NewEnv(t *testing.T) *Env
```

Builds the full container stack with `WithHomeDir(t.TempDir())` on every container. Starts the Gin server on a Unix socket. Returns typed clients pointed at the socket. `Close()` shuts down gracefully and cleans up the socket file.

**Wiring order:**
1. `engine.New(ctx, engine.WithHomeDir(dir), engine.WithAgentRuntime(stub))`
2. `adapter.New(adapter.WithHomeDir(dir))`
3. `app.New(engines, adapters, app.WithHomeDir(dir))`
4. `api.New(appContainer)`
5. Start listener on `{dir}/crowbar.sock`

`engine.WithAgentRuntime(stub)` replaces the real `AgentRuntime` with the `AgentStub` for all tests.

---

### `client.go` — REST client

Routed over the Unix socket. One typed method per REST endpoint — no raw HTTP calls in test bodies.

```go
type Client struct{ ... }

func (c *Client) CreateProject(
    req CreateProjectRequest,
) (domain.Project, error)

func (c *Client) CreateRepository(
    projectID string,
    req       CreateRepositoryRequest,
) (domain.Repository, error)

func (c *Client) CreateTask(
    repoID string,
    req    CreateTaskRequest,
) (domain.Task, error)

func (c *Client) GetTask(taskID string) (domain.Task, error)
func (c *Client) ArchiveTask(taskID string) error
func (c *Client) PauseTask(taskID string) error
func (c *Client) ResumeTask(taskID string) error
func (c *Client) RetryTask(taskID string) error
func (c *Client) ForceTransition(taskID string, toState string) error

func (c *Client) ListAgentRuns(taskID string) ([]domain.AgentRun, error)
func (c *Client) InterruptAgentRun(agentRunID string) error

func (c *Client) ListKanbanItems(taskID string) ([]domain.KanbanItem, error)

func (c *Client) ListThreads(
    taskID string,
    status string,
    phase  string,
) ([]domain.ReviewThread, error)

func (c *Client) OpenThread(
    taskID  string,
    req     OpenThreadRequest,
) (domain.ReviewThread, error)

func (c *Client) PostMessage(
    threadID string,
    content  string,
) error

func (c *Client) ForceApproveThread(threadID string) error

func (c *Client) GetGitLog(taskID string) ([]GitCommit, error)
func (c *Client) GetGitDiff(taskID string) (GitDiff, error)
func (c *Client) ListFiles(taskID string, path string) ([]FileEntry, error)
func (c *Client) GetMessages(taskID string) ([]domain.ConversationMessage, error)
```

---

### `ws_client.go` — WebSocket client

```go
type WSClient struct{ ... }

func (c *WSClient) SubscribeTasks(
    taskID string,
) (<-chan domain.Task, func())

func (c *WSClient) SubscribeAgentRuns(
    taskID string,
) (<-chan domain.AgentRun, func())

func (c *WSClient) SubscribeKanbanItems(
    taskID string,
) (<-chan domain.KanbanItem, func())

func (c *WSClient) SubscribeThreads(
    taskID string,
) (<-chan domain.ReviewThread, func())

func (c *WSClient) ConnectChat(
    taskID string,
) (*ChatConn, func())
```

`ChatConn` exposes:
```go
func (c *ChatConn) Send(content string) error
func (c *ChatConn) Frames() <-chan agent.ChatFrame
```

The second return value of each Subscribe/Connect call is an unsubscribe/close function. Channels are buffered (cap 64) — tests must drain them promptly.

---

### `mcp_client.go` — MCP tool caller

Calls MCP tools directly with a synthesized AgentRun token. `WithToken` pins the client to a specific token — obtained from `AgentStub.WaitForRun`.

```go
type MCPClient struct{ ... }

func (c *MCPClient) WithToken(token string) *MCPClient

func (c *MCPClient) Signal(
    event  string,
    output string,
) error

func (c *MCPClient) CreateItem(title string) (string, error)

func (c *MCPClient) UpdateItemStatus(
    itemID string,
    status string,
) error

func (c *MCPClient) GetItems() ([]domain.KanbanItem, error)

func (c *MCPClient) OpenThread(
    file    string,
    line    int,
    content string,
) (string, error)

func (c *MCPClient) ReplyThread(
    threadID string,
    content  string,
) error

func (c *MCPClient) GetThreads(
    status string,
    phase  string,
) ([]domain.ReviewThread, error)

func (c *MCPClient) ResolveThread(
    threadID string,
    emoji    string,
) error
```

---

### `agent_stub.go` — controllable AgentRuntime

Implements `engine/agent.AgentRuntime`. When `Run()` is called, it registers the AgentRun token internally and blocks until told what to do.

```go
type AgentStub struct{ ... }

// WaitForRun blocks until an AgentRun session is started, then returns its token.
// Fails the test if no run starts within the timeout.
func (s *AgentStub) WaitForRun(
    ctx context.Context,
) string

// FireSignal causes the active session to call crowbar_signal.
func (s *AgentStub) FireSignal(
    event  string,
    output string,
)

// StreamChunks sends a sequence of text chunks to the chat channel,
// then sends agent_turn_end.
func (s *AgentStub) StreamChunks(chunks ...string)

// Fail causes the active session to exit without signalling (simulates crash).
func (s *AgentStub) Fail()
```

---

### `fixtures.go` — standard test data

```go
// DefaultProject creates a Project with a generated name.
func DefaultProject(c *Client) domain.Project

// DefaultRepository creates a Repository under project using a real git repo at path.
func DefaultRepository(
    c         *Client,
    projectID string,
    repoPath  string,
) domain.Repository

// DefaultTask creates a Task on a Repository using the builtin feature-development flow.
func DefaultTask(
    c      *Client,
    repoID string,
    branch string,
) domain.Task
```

---

### `git.go` — real git repo builder

```go
type GitRepo struct{ ... }

func NewGitRepo(t *testing.T) *GitRepo

func (r *GitRepo) Path() string

func (r *GitRepo) Commit(
    message string,
    files   map[string]string,
) string // returns commit SHA
```

Creates a real `git init` repo in `t.TempDir()`. Commits write files and run `git add + git commit`. Used by `worktree/` and `git/` suites to produce real diffs and commit history.

---

## 3. Suite Descriptions

### `lifecycle/`

Full Task lifecycle from creation to completion using the builtin flow.

- Create Project → Repository (real git repo) → Task; assert worktree path exists on disk
- `Stub.WaitForRun` → `Stub.FireSignal("user_approved", "brainstorming done")` → assert Task advances to `spec`
- Repeat through `implementation_complete` → `ai_review` → `review_passed` → `human_review`
- Client calls `ForceTransition(taskID, "complete")` → assert Task status is `complete`
- Assert `AgentRun.Output` values accumulate in order across states
- Assert WebSocket `task.state_advanced` events delivered for each transition

### `chat/`

Chat WebSocket bidirectional flow.

- Connect `ChatConn` to running task
- `ChatConn.Send("hello")` → assert `ConversationMessage{role: user}` written to SQLite
- `Stub.StreamChunks("hello ", "world")` → assert two `agent_chunk` frames received
- Assert `agent_turn_end` frame received
- Assert `ConversationMessage{role: agent, type: text, content: "hello world"}` in SQLite
- `Client.GetMessages(taskID)` returns both messages in order
- Reconnect chat WebSocket → history loads cleanly via REST; new chunks stream correctly

### `flow/`

Flow YAML validation and state machine evaluation.

- Load builtin flow: no error, 7 states present, exactly one terminal state
- YAML with missing `name` field: returns structural validation error
- YAML with transition to nonexistent state: returns `TransitionTargetsExist` semantic error
- YAML with unknown tool `crowbar.nonexistent`: returns `AgentToolsKnown` error
- YAML with no terminal state: returns `AtLeastOneTerminal` error
- `Evaluate(flow, "brainstorming", "user_approved")` → `"spec"`
- `Evaluate(flow, "brainstorming", "unknown_event")` → `("", false)`

### `kanban/`

KanbanItem lifecycle via MCP tools.

- Start Task in `implementation` state (force-transition via stub)
- `MCPClient.CreateItem("add login button")` → assert item appears in `ListKanbanItems`
- Assert `kanban_item.created` WebSocket event received
- `MCPClient.UpdateItemStatus(itemID, "implementing")` → assert status updated
- Assert `kanban_item.status_updated` WebSocket event received
- Call `CreateItem` in `brainstorming` state (no `items: true`) → assert `403 Forbidden`

### `threads/`

ReviewThread and ReviewMessage lifecycle.

- Human opens thread via `Client.OpenThread` → assert `phase: human_review, opened_by: human`
- Agent replies via `MCPClient.ReplyThread` → assert message with `role: implementer`
- `MCPClient.ResolveThread(threadID, "👍")` → assert `status: agreed`, emoji stored
- Human opens thread via `MCPClient.OpenThread` in `ai_review` state → assert `phase: ai_review, opened_by: reviewer`
- `Client.ForceApproveThread` → assert `status: force_approved`
- `MCPClient.GetThreads(status: "open", phase: "ai_review")` returns only matching threads

### `mcp/`

MCP authorization and tool routing.

- Unknown token → `401 Unauthorized`
- Token for completed AgentRun → `401 Unauthorized`
- Tool not in state's tool list → `403 Forbidden`
- `crowbar_signal` with unknown event → structured error response
- `crowbar_signal` with valid event → Task advances, `state_transition` frame delivered to chat

### `worktree/`

Git worktree management.

- `CreateTask` with valid branch → assert `task.WorktreePath` is a real directory on disk
- Confirm `git worktree list` shows the worktree on the repo
- `ArchiveTask` → assert worktree directory no longer exists on disk
- `CreateTask` with already-existing branch name → assert error

### `git/`

Git REST endpoints against a known commit history.

- Build `GitRepo` with 3 commits, each modifying known files
- Create Task on that repo; agent commits one more file via the worktree
- `GetGitLog(taskID)` → returns all commits on the task branch in chronological order; asserts correct SHAs and messages
- `GetGitDiff(taskID)` → returns structured JSON; assert correct file name, hunk count, added/removed lines

### `websocket/`

Broadcaster[T] domain event delivery.

- Subscribe to `tasks/:id` → assert only events for that task_id received, not others
- Subscribe to `tasks/:task_id/kanban-items` → assert kanban events received; unsubscribe → assert no more frames
- Create a slow consumer (never drains channel) → assert other connected clients still receive events without blocking

### `crash/`

Server restart with orphaned AgentRun.

`crash/` does not use `IntegrationSuite.SetupTest` — it manages two `Env` instances manually, sharing the same `homeDir := t.TempDir()`. First env simulates the crash; second env simulates the restart against the same SQLite data.

- `env1 := NewEnvWithHome(t, homeDir)` — start Task; `Stub.WaitForRun` confirms AgentRun is `running`
- `env1.Close()` without letting stub fire signal — simulates crash with running AgentRun
- `env2 := NewEnvWithHome(t, homeDir)` — restart with same home dir
- Assert the orphaned AgentRun is now `failed`
- Assert Task status is `paused`
- `env2.Client.RetryTask` → assert new AgentRun created with status `running`

### `guards/`

Invalid inputs and constraint violations.

- `CreateTask` with nonexistent `repoID` → `404 Not Found`
- `CreateTask` with a flow YAML path that has validation errors → `400 Bad Request` with error details
- `ForceTransition` to a state not in the flow → `400 Bad Request`
- `UpdateItemStatus` with a status not in `flow.ItemStatuses` → `400 Bad Request`
- `GET /tasks/nonexistent` → `404 Not Found`

### `concurrency/`

Concurrent operations.

- 10 goroutines each call `CreateTask` on the same Repository simultaneously → all succeed; all worktree paths are distinct directories on disk
- 5 goroutines call `MCPClient.CreateItem` concurrently under the same AgentRun token → all 5 items created; no duplicates; all appear in `ListKanbanItems`

---

## 4. Key Dependencies

- `github.com/stretchr/testify` — suite, assertions (already in ecosystem)
- `github.com/gorilla/websocket` — WebSocket test client (same lib as server)
- Standard `net/http` over Unix socket for REST client (same `hyperlocal`-style dialer as Quiver's test client)
