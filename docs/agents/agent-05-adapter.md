# Agent 05 — Adapter Layer

**Working directory:** `/Users/char2cs/Projects/Rabbyte/crowbar/api`
**Module:** `github.com/char2cs/crowbar/api`

## Context

The adapter layer opens the SQLite GORM database and the four Asynx event store files. It owns no domain logic — it only constructs and exposes storage backends to the app layer.

## Files to read before starting

- `api/ARCHITECTURE.md` §"Storage Tiers", §"Asynx API Reference"
- `docs/superpowers/specs/2026-05-19-domain-crud-design.md` §1 (storage tier description)
- Quiver reference: `/Users/char2cs/Projects/Rabbyte/quiver.core/internal/adapter/container.go`
- Quiver reference: `/Users/char2cs/Projects/Rabbyte/quiver.core/internal/adapter/eventstore/sqlite/` — event store implementation

## What already exists

Agents 01–04 are complete. Core paths package exists.

## Package layout

```
internal/adapter/
├── container.go
└── store/
    └── sqlite/
        └── sqlite.go      // GORM SQLite helpers
```

## Tasks

### `internal/adapter/store/sqlite/sqlite.go`

```go
package sqlite

import (
    "gorm.io/driver/sqlite"
    "gorm.io/gorm"
)

// OpenDB opens a SQLite GORM database at the given path.
// Sets max open connections to 1 to serialise writes.
func OpenDB(path string) (*gorm.DB, error)
```

### `internal/adapter/container.go`

```go
package adapter

import (
    asynxModels "github.com/char2cs/asynx/models"
    asynxSqlite "github.com/char2cs/asynx/store/sqlite"
    gormdb "gorm.io/gorm"
    "github.com/char2cs/crowbar/api/internal/core/paths"
    adapterSqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
)

type Container struct {
    Store        *gormdb.DB
    TaskES       asynxModels.Store
    AgentRunES   asynxModels.Store
    KanbanItemES asynxModels.Store
    ThreadES     asynxModels.Store
    close        []func() error
}

type adapterOpts struct{ homeDir string }
type Option func(*adapterOpts)

func WithHomeDir(dir string) Option {
    return func(o *adapterOpts) { o.homeDir = dir }
}

func New(opts ...Option) (*Container, error)
func (c *Container) Close() error
```

`New`:
1. Resolve `storePath` and `eventsPath` using `paths.StoreAt`/`paths.EventsAt` (or non-At variants if no homeDir override)
2. Open GORM DB at `filepath.Join(storePath, "crowbar.db")`
3. Open four event stores via `asynxSqlite.NewEventStore`:
   - `filepath.Join(eventsPath, "tasks.db")`
   - `filepath.Join(eventsPath, "agent_runs.db")`
   - `filepath.Join(eventsPath, "kanban_items.db")`
   - `filepath.Join(eventsPath, "review_threads.db")`
4. Store close functions; `Close()` calls all of them

Check the Quiver reference for the exact `asynxSqlite` import path — it may be `github.com/char2cs/asynx/store/sqlite` or similar.

## Verification

```bash
cd /Users/char2cs/Projects/Rabbyte/crowbar/api && go build ./internal/adapter/...
go vet ./internal/adapter/...
```
