# Agent 11 — REST Handlers

**Working directory:** `/Users/char2cs/Projects/Rabbyte/crowbar/api`
**Module:** `github.com/char2cs/crowbar/api`

## Context

This agent implements all REST HTTP handlers. Handlers receive parsed requests, call repository methods, and return raw domain structs. No DTOs — the domain entity is the HTTP response body.

## Files to read before starting

- `docs/superpowers/specs/2026-05-19-domain-crud-design.md` §3 (routes), §7 (HTTP shapes, request bodies, status codes, error envelope)
- Quiver reference: `/Users/char2cs/Projects/Rabbyte/quiver.core/internal/api/v0/` — handler patterns

## What already exists

Agents 01–10 complete. All repositories, domain types, and the flow engine are implemented.

## Package layout

```
internal/api/v0/
├── handlers/
│   ├── project.go
│   ├── repository.go
│   ├── task.go
│   ├── agentrun.go
│   ├── kanbanitem.go
│   ├── thread.go
│   ├── git.go
│   └── conversation.go
└── middleware/
    └── errors.go    // error helper
```

## Tasks

### Error conventions

All handlers use this pattern:
- Success with body: `c.JSON(200, entity)` or `c.JSON(201, entity)`
- Success no body: `c.Status(204)`
- Not found: `c.JSON(404, gin.H{"error": "not found"})`
- Bad request: `c.JSON(400, gin.H{"error": "...", "details": [...]})`
- Internal error: `c.JSON(500, gin.H{"error": err.Error()})`

Map `repositories.ErrNotFound` → 404.

### `handlers/project.go`

```go
type ProjectHandler struct{ repo repositories.Project }

func (h *ProjectHandler) Create(c *gin.Context)  // POST /projects → 201
func (h *ProjectHandler) List(c *gin.Context)    // GET /projects → 200
func (h *ProjectHandler) Get(c *gin.Context)     // GET /projects/:id → 200
func (h *ProjectHandler) Delete(c *gin.Context)  // DELETE /projects/:id → 204
```

Request body for Create: `{"name": "string"}`.

### `handlers/repository.go`

```go
type RepositoryHandler struct{ repo repositories.Repository }

func (h *RepositoryHandler) Create(c *gin.Context)  // POST /projects/:id/repositories → 201
func (h *RepositoryHandler) List(c *gin.Context)    // GET /projects/:id/repositories → 200
func (h *RepositoryHandler) Get(c *gin.Context)     // GET /repositories/:id → 200
func (h *RepositoryHandler) Delete(c *gin.Context)  // DELETE /repositories/:id → 204
```

Request body for Create: `{"name": "string", "path": "string"}`.

### `handlers/task.go`

```go
type TaskHandler struct {
    taskRepo repositories.Task
    repoRepo repositories.Repository
    flowLoader flow.Loader
}

func (h *TaskHandler) Create(c *gin.Context)          // POST /repositories/:id/tasks → 201
func (h *TaskHandler) Get(c *gin.Context)             // GET /tasks/:id → 200
func (h *TaskHandler) Archive(c *gin.Context)         // POST /tasks/:id/archive → 200
func (h *TaskHandler) Pause(c *gin.Context)           // POST /tasks/:id/pause → 200
func (h *TaskHandler) Resume(c *gin.Context)          // POST /tasks/:id/resume → 200
func (h *TaskHandler) Retry(c *gin.Context)           // POST /tasks/:id/retry → 200
func (h *TaskHandler) ForceTransition(c *gin.Context) // POST /tasks/:id/force-transition → 200
```

Request body for Create: `{"title": "string", "branch_name": "string", "base_branch": "string", "flow_path": "string"}`. `flow_path` is optional.

Request body for ForceTransition: `{"to_state": "string"}`. Validate that `to_state` is a valid state name in the task's flow before calling the repo.

### `handlers/agentrun.go`

```go
type AgentRunHandler struct{ repo repositories.AgentRun }

func (h *AgentRunHandler) List(c *gin.Context)      // GET /tasks/:id/agent-runs → 200
func (h *AgentRunHandler) Interrupt(c *gin.Context) // POST /agent-runs/:id/interrupt → 200
```

### `handlers/kanbanitem.go`

```go
type KanbanItemHandler struct{ repo repositories.KanbanItem }

func (h *KanbanItemHandler) List(c *gin.Context) // GET /tasks/:id/kanban-items → 200
```

### `handlers/thread.go`

```go
type ThreadHandler struct{ repo repositories.ReviewThread }

func (h *ThreadHandler) List(c *gin.Context)         // GET /tasks/:id/threads?status=&phase= → 200
func (h *ThreadHandler) Open(c *gin.Context)         // POST /tasks/:id/threads → 201
func (h *ThreadHandler) PostMessage(c *gin.Context)  // POST /threads/:id/messages → 200
func (h *ThreadHandler) ForceApprove(c *gin.Context) // POST /threads/:id/force-approve → 200
```

Request body for Open: `{"file": "string", "line": int, "content": "string"}`. `opened_by` is always `"human"` for REST-created threads.

Request body for PostMessage: `{"content": "string"}`.

### `handlers/git.go`

Git REST endpoints run shell commands in the task's worktree path.

```go
type GitHandler struct{ taskRepo repositories.Task }

func (h *GitHandler) Log(c *gin.Context)       // GET /tasks/:id/git/log → 200
func (h *GitHandler) Diff(c *gin.Context)      // GET /tasks/:id/git/diff → 200
func (h *GitHandler) ListFiles(c *gin.Context) // GET /tasks/:id/files?path= → 200
```

`Log`: runs `git log --format="%H|%s|%ai" HEAD` in `task.WorktreePath`. Parse output into:
```go
type GitCommit struct {
    SHA     string `json:"sha"`
    Message string `json:"message"`
    Date    string `json:"date"`
}
```

`Diff`: runs `git diff HEAD --unified=3 --name-only` then `git diff HEAD --unified=3` in `task.WorktreePath`. Parse into:
```go
type GitDiff struct {
    Files []DiffFile `json:"files"`
}
type DiffFile struct {
    Name   string `json:"name"`
    Hunks  int    `json:"hunks"`
    Added  int    `json:"added"`
    Removed int   `json:"removed"`
}
```

`ListFiles`: runs `git ls-files -- <path>` where path is from query param (default `.`). Returns `[]string`.

Use `exec.CommandContext(c.Request.Context(), "git", ...)` with `cmd.Dir = task.WorktreePath`.

### `handlers/conversation.go`

```go
type ConversationHandler struct{ repo repositories.Conversation }

func (h *ConversationHandler) List(c *gin.Context) // GET /tasks/:id/messages → 200
```

## Verification

```bash
cd /Users/char2cs/Projects/Rabbyte/crowbar/api && go build ./internal/api/v0/handlers/...
go vet ./internal/api/v0/handlers/...
```
