# Agent 07 — GORM Repositories

**Working directory:** `/Users/char2cs/Projects/Rabbyte/crowbar/api`
**Module:** `github.com/char2cs/crowbar/api`

## Context

Three domain entities are stored in GORM SQLite: `Project`, `Repository`, and `ConversationMessage`. This agent implements their repositories.

## Files to read before starting

- `docs/superpowers/specs/2026-05-19-domain-crud-design.md` §2 (entity definitions), §3 (REST routes — infer which repo methods are needed), §7 (HTTP shapes)
- Quiver reference: `/Users/char2cs/Projects/Rabbyte/quiver.core/internal/app/repositories/arrow/` — GORM repo pattern

## What already exists

Agents 01–06 complete. Domain entities defined. Adapter container has `Store *gorm.DB`.

## Package layout

```
internal/app/repositories/
├── interfaces.go          // repository interfaces
├── project/
│   └── project.go
├── repository/
│   └── repository.go
└── conversation/
    └── conversation.go
```

## Tasks

### `interfaces.go`

Define repository interfaces consumed by usecases and handlers:

```go
package repositories

type Project interface {
    Create(ctx context.Context, name string) (domain.Project, error)
    List(ctx context.Context) ([]domain.Project, error)
    Get(ctx context.Context, id string) (domain.Project, error)
    Delete(ctx context.Context, id string) error
}

type Repository interface {
    Create(ctx context.Context, projectID string, name string, path string) (domain.Repository, error)
    List(ctx context.Context, projectID string) ([]domain.Repository, error)
    Get(ctx context.Context, id string) (domain.Repository, error)
    Delete(ctx context.Context, id string) error
}

type Conversation interface {
    Create(ctx context.Context, taskID string, role domain.ConversationMessageRole, msgType domain.ConversationMessageType, content string) (domain.ConversationMessage, error)
    ListByTask(ctx context.Context, taskID string) ([]domain.ConversationMessage, error)
}
```

Errors: return `ErrNotFound` (a sentinel `var ErrNotFound = errors.New("not found")` in this file) for missing records; GORM's `ErrRecordNotFound` maps to this.

### `project/project.go`

Implements `repositories.Project` over GORM. Uses `uuid.New().String()` for IDs (check if `github.com/google/uuid` is available; if not, use `fmt.Sprintf("%d", time.Now().UnixNano())` as a fallback).

`AutoMigrate(&domain.Project{})` in the constructor.

### `repository/repository.go`

Implements `repositories.Repository` over GORM. `AutoMigrate(&domain.Repository{})` in constructor. `List` filters by `projectID`.

### `conversation/conversation.go`

Implements `repositories.Conversation` over GORM. `AutoMigrate(&domain.ConversationMessage{})` in constructor. `ListByTask` orders by `created_at ASC`.

## Verification

```bash
cd /Users/char2cs/Projects/Rabbyte/crowbar/api && go build ./internal/app/repositories/...
go vet ./internal/app/repositories/...
```
