# Agent 03 — Domain Entities

**Working directory:** `/Users/char2cs/Projects/Rabbyte/crowbar/api`
**Module:** `github.com/char2cs/crowbar/api`

## Context

The domain layer defines all entities, status enums, and value types. It has zero imports from any other internal package. Later agents build repositories and handlers on top of these types.

## Files to read before starting

- `docs/superpowers/specs/2026-05-19-domain-crud-design.md` — complete; entity definitions are in §2
- `api/ARCHITECTURE.md` §"Storage Tiers" — which entities use GORM tags vs Asynx tags
- Quiver reference: `/Users/char2cs/Projects/Rabbyte/quiver.core/internal/domain/` — tagging patterns

## What already exists

Agent 01 deleted `internal/domain/workspace.go`. Other scaffold domain files may exist — read them first and replace/extend as needed.

## Tasks

Write one file per entity in `internal/domain/`. All files are in `package domain`.

### `project.go`

```go
type Project struct {
    ID        string    `gorm:"primaryKey" json:"id"`
    Name      string    `json:"name"`
    CreatedAt time.Time `json:"created_at"`
}
```

### `repository.go`

```go
type Repository struct {
    ID        string    `gorm:"primaryKey" json:"id"`
    ProjectID string    `gorm:"index" json:"project_id"`
    Name      string    `json:"name"`
    Path      string    `json:"path"`      // absolute path on disk
    CreatedAt time.Time `json:"created_at"`
}
```

### `task.go`

Task is event-sourced via Asynx. No GORM tags.

```go
type TaskStatus string

const (
    TaskStatusPending   TaskStatus = "pending"
    TaskStatusRunning   TaskStatus = "running"
    TaskStatusPaused    TaskStatus = "paused"
    TaskStatusComplete  TaskStatus = "complete"
    TaskStatusArchived  TaskStatus = "archived"
)

type Task struct {
    ID           string     `json:"id"`
    RepositoryID string     `json:"repository_id"`
    Title        string     `json:"title"`
    Status       TaskStatus `json:"status"`
    CurrentState string     `json:"current_state"`  // flow state name
    FlowPath     string     `json:"flow_path"`       // "" = builtin
    BranchName   string     `json:"branch_name"`
    BaseBranch   string     `json:"base_branch"`
    WorktreePath string     `json:"worktree_path"`
    CreatedAt    time.Time  `json:"created_at"`
    UpdatedAt    time.Time  `json:"updated_at"`
}
```

### `agentrun.go`

AgentRun is event-sourced via Asynx.

```go
type AgentRunStatus string

const (
    AgentRunStatusRunning     AgentRunStatus = "running"
    AgentRunStatusCompleted   AgentRunStatus = "completed"
    AgentRunStatusFailed      AgentRunStatus = "failed"
    AgentRunStatusInterrupted AgentRunStatus = "interrupted"
)

type AgentRunOutput struct {
    StateName string `json:"state_name"`
    Output    string `json:"output"`
}

type AgentRun struct {
    ID         string         `json:"id"`
    TaskID     string         `json:"task_id"`
    StateName  string         `json:"state_name"`
    Status     AgentRunStatus `json:"status"`
    Token      string         `gorm:"-" json:"-"` // MCP auth token; never serialised
    Outputs    []AgentRunOutput `json:"outputs"`
    CreatedAt  time.Time      `json:"created_at"`
    UpdatedAt  time.Time      `json:"updated_at"`
}
```

### `kanban_item.go`

KanbanItem is event-sourced via Asynx.

```go
type KanbanItem struct {
    ID          string    `json:"id"`
    TaskID      string    `json:"task_id"`
    Title       string    `json:"title"`
    Description string    `json:"description"`
    Status      string    `json:"status"`   // open string type — validated against flow.ItemStatuses at runtime
    AgentRunID  string    `json:"agent_run_id"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}
```

### `review_thread.go`

ReviewThread and ReviewMessage are event-sourced via Asynx.

```go
type ReviewThreadStatus string
type ReviewPhase string

const (
    ReviewThreadStatusOpen          ReviewThreadStatus = "open"
    ReviewThreadStatusAgreed        ReviewThreadStatus = "agreed"
    ReviewThreadStatusForceApproved ReviewThreadStatus = "force_approved"

    ReviewPhaseAIReview    ReviewPhase = "ai_review"
    ReviewPhaseHumanReview ReviewPhase = "human_review"
)

type ReviewMessage struct {
    ID        string    `json:"id"`
    ThreadID  string    `json:"thread_id"`
    Role      string    `json:"role"`    // "reviewer" | "implementer" | "human"
    Content   string    `json:"content"`
    CreatedAt time.Time `json:"created_at"`
}

type ReviewThread struct {
    ID         string             `json:"id"`
    TaskID     string             `json:"task_id"`
    AgentRunID *string            `json:"agent_run_id,omitempty"`
    File       string             `json:"file"`
    Line       int                `json:"line"`
    Phase      ReviewPhase        `json:"phase"`
    OpenedBy   string             `json:"opened_by"` // "human" | "reviewer"
    Status     ReviewThreadStatus `json:"status"`
    Emoji      string             `json:"emoji,omitempty"`
    Messages   []ReviewMessage    `json:"messages"`
    CreatedAt  time.Time          `json:"created_at"`
    UpdatedAt  time.Time          `json:"updated_at"`
}
```

### `conversation_message.go`

ConversationMessage is stored in GORM SQLite. Uses GORM tags.

```go
type ConversationMessageRole string
type ConversationMessageType string

const (
    ConversationMessageRoleUser  ConversationMessageRole = "user"
    ConversationMessageRoleAgent ConversationMessageRole = "agent"

    ConversationMessageTypeText       ConversationMessageType = "text"
    ConversationMessageTypeToolCall   ConversationMessageType = "tool_call"
    ConversationMessageTypeToolResult ConversationMessageType = "tool_result"
)

type ConversationMessage struct {
    ID        string                  `gorm:"primaryKey" json:"id"`
    TaskID    string                  `gorm:"index" json:"task_id"`
    Role      ConversationMessageRole `json:"role"`
    Type      ConversationMessageType `json:"type"`
    Content   string                  `json:"content"`
    CreatedAt time.Time               `json:"created_at"`
}
```

## Verification

```bash
cd /Users/char2cs/Projects/Rabbyte/crowbar/api && go build ./internal/domain/...
go vet ./internal/domain/...
```
