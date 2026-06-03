# Agent 08 — Asynx Repositories

**Working directory:** `/Users/char2cs/Projects/Rabbyte/crowbar/api`
**Module:** `github.com/char2cs/crowbar/api`

## Context

Four domain aggregates are event-sourced via Asynx: `Task`, `AgentRun`, `KanbanItem`, `ReviewThread`. Each gets its own repository package that wraps an `asynx.Asynx[T]` instance.

## Files to read before starting

- `docs/superpowers/specs/2026-05-19-domain-crud-design.md` §2 and §3 — all CRUD operations
- `api/ARCHITECTURE.md` §"Asynx API Reference" — full builder and command API
- Quiver reference: `/Users/char2cs/Projects/Rabbyte/quiver.core/internal/app/repositories/runtime/` — Asynx repo pattern and crash recovery
- Quiver reference: `/Users/char2cs/Projects/Rabbyte/quiver.core/internal/app/repositories/arrow/arrow.go` — Subscribe callback pattern

## What already exists

Agents 01–07 complete. `repositories/interfaces.go` has the interface definitions from Agent 07. Adapter container has all four event stores.

## Package layout

```
internal/app/repositories/
├── task/
│   └── task.go
├── agentrun/
│   └── agentrun.go
├── kanbanitem/
│   └── kanbanitem.go
└── reviewthread/
    └── reviewthread.go
```

Add these interfaces to `interfaces.go` (extend Agent 07's file):

```go
type Task interface {
    Create(ctx context.Context, repositoryID string, title string, flowPath string, branchName string, baseBranch string, worktreePath string) (domain.Task, error)
    Get(ctx context.Context, id string) (domain.Task, error)
    Archive(ctx context.Context, id string) error
    Pause(ctx context.Context, id string) error
    Resume(ctx context.Context, id string) error
    Retry(ctx context.Context, id string) error
    AdvanceState(ctx context.Context, id string, nextState string, event string) error
    ForceTransition(ctx context.Context, id string, toState string) error
    Subscribe(topic string, fn func(ctx context.Context, evt asynxModels.Event[domain.Task])) (string, error)
}

type AgentRun interface {
    Create(ctx context.Context, taskID string, stateName string, token string) (domain.AgentRun, error)
    Get(ctx context.Context, id string) (domain.AgentRun, error)
    GetByToken(ctx context.Context, token string) (domain.AgentRun, error)
    ListByTask(ctx context.Context, taskID string) ([]domain.AgentRun, error)
    CompleteAgentRun(ctx context.Context, id string, output string) error
    FailAgentRun(ctx context.Context, id string) error
    InterruptAgentRun(ctx context.Context, id string) error
    RecoverOrphanedRuns(ctx context.Context) error
    Subscribe(topic string, fn func(ctx context.Context, evt asynxModels.Event[domain.AgentRun])) (string, error)
}

type KanbanItem interface {
    Create(ctx context.Context, taskID string, title string, agentRunID string) (domain.KanbanItem, error)
    Get(ctx context.Context, id string) (domain.KanbanItem, error)
    ListByTask(ctx context.Context, taskID string) ([]domain.KanbanItem, error)
    UpdateStatus(ctx context.Context, id string, status string, agentRunID string) error
    Subscribe(topic string, fn func(ctx context.Context, evt asynxModels.Event[domain.KanbanItem])) (string, error)
}

type ReviewThread interface {
    Open(ctx context.Context, taskID string, agentRunID *string, file string, line int, phase domain.ReviewPhase, openedBy string, content string) (domain.ReviewThread, error)
    Get(ctx context.Context, id string) (domain.ReviewThread, error)
    ListByTask(ctx context.Context, taskID string, status string, phase string) ([]domain.ReviewThread, error)
    PostMessage(ctx context.Context, threadID string, role string, content string) error
    ForceApprove(ctx context.Context, threadID string) error
    ResolveThread(ctx context.Context, threadID string, emoji string) error
    Subscribe(topic string, fn func(ctx context.Context, evt asynxModels.Event[domain.ReviewThread])) (string, error)
}
```

## Tasks

### Asynx command pattern

Each mutation is an Asynx command. A command is a struct implementing `asynxModels.Command[T]`. Example for Task:

```go
type createTask struct {
    id           string
    repositoryID string
    title        string
    // ...
}

func (c createTask) ID() string { return c.id }

func (c createTask) Handle(t domain.Task) (domain.Task, error) {
    t.ID = c.id
    t.RepositoryID = c.repositoryID
    t.Title = c.title
    t.Status = domain.TaskStatusPending
    t.CurrentState = "brainstorming"  // first state
    t.CreatedAt = time.Now()
    t.UpdatedAt = time.Now()
    return t, nil
}
```

Use `ax.Send(ctx, cmd)` for fire-and-forget; `ax.SendWait(ctx, cmd)` for mutations where the caller needs the updated aggregate.

### `task/task.go`

Commands: `createTask`, `archiveTask`, `pauseTask`, `resumeTask`, `advanceState`, `forceTransition`.

`Create` uses `ax.SendWait` and returns the aggregate. `Retry` creates a new AgentRun for the current state — it does NOT reset `CurrentState`; it only updates `Status = running`.

**Important:** `Create` must also create a git worktree via `exec.Command("git", "worktree", "add", worktreePath, "-b", branchName)` in the repository path. Fail the command if the branch already exists.

### `agentrun/agentrun.go`

Commands: `createAgentRun`, `completeAgentRun`, `failAgentRun`, `interruptAgentRun`.

`GetByToken`: Asynx does not support querying by non-ID field. Implement a secondary index: maintain a `sync.Map` mapping `token → id` that is populated on `Create` and consulted by `GetByToken`. This map is in-memory and rebuilt on startup via `RecoverOrphanedRuns`.

`RecoverOrphanedRuns`: scan all AgentRun aggregates known to Asynx (use `ax.Preload` + `ax.Get` on a list of IDs stored in a local slice), find those with `Status == running`, and issue `failAgentRun` for each. See ARCHITECTURE.md §"Crash recovery pattern".

### `kanbanitem/kanbanitem.go`

Commands: `createKanbanItem`, `updateKanbanItemStatus`.

### `reviewthread/reviewthread.go`

Commands: `openThread`, `postMessage`, `forceApproveThread`, `resolveThread`.

`openThread` creates the thread aggregate and appends the first message. `postMessage` appends a `ReviewMessage` to `thread.Messages`. `resolveThread` sets `Status = agreed` and stores the emoji.

## Verification

```bash
cd /Users/char2cs/Projects/Rabbyte/crowbar/api && go build ./internal/app/repositories/...
go vet ./internal/app/repositories/...
```
