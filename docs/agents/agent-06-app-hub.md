# Agent 06 — App Hub (WebSocket Hub)

**Working directory:** `/Users/char2cs/Projects/Rabbyte/crowbar/api`
**Module:** `github.com/char2cs/crowbar/api`

## Context

The app hub is the typed domain event broadcaster used by the API layer's WebSocket handlers. It defines the `WebSocketHub` interface (used by the app layer to broadcast domain events) and the `Subscriber` interface (implemented by the API layer's WS handlers). The concrete `Hub` struct lives here and fans out events to registered subscribers.

## Files to read before starting

- `docs/superpowers/specs/2026-05-19-domain-crud-design.md` §4 (WebSocket event envelope)
- `api/ARCHITECTURE.md` §"Scaffold Files to Delete / Rename" (hub.go replacement description)
- Quiver reference: `/Users/char2cs/Projects/Rabbyte/quiver.core/internal/app/hub/` — read all files

## What already exists

Agent 01 replaced `internal/app/hub/hub.go` with a minimal stub defining `WebSocketHub`. Agent 03 defined all domain entity types. This agent fully implements the hub.

## Package layout

```
internal/app/hub/
├── hub.go        // WebSocketHub interface + Hub struct + Subscriber interface
└── broadcaster.go // generic Broadcaster[T] used by Hub internally (optional — see below)
```

## Tasks

### `hub.go`

**`WebSocketHub` interface** — used by the repositories/container to broadcast domain events:

```go
type WebSocketHub interface {
    BroadcastTask(t domain.Task)
    BroadcastAgentRun(r domain.AgentRun)
    BroadcastKanbanItem(i domain.KanbanItem)
    BroadcastReviewThread(t domain.ReviewThread)
}
```

**`Subscriber` interface** — implemented by API-layer WS handlers:

```go
type Subscriber interface {
    PushTask(t domain.Task)
    PushAgentRun(r domain.AgentRun)
    PushKanbanItem(i domain.KanbanItem)
    PushReviewThread(t domain.ReviewThread)
}
```

**`Hub` struct** — implements `WebSocketHub`; fans out to registered `Subscriber` instances:

```go
type Hub struct {
    mu          sync.RWMutex
    subscribers []Subscriber
}

func NewHub() *Hub

func (h *Hub) Register(s Subscriber)
func (h *Hub) Unregister(s Subscriber)

func (h *Hub) BroadcastTask(t domain.Task)
func (h *Hub) BroadcastAgentRun(r domain.AgentRun)
func (h *Hub) BroadcastKanbanItem(i domain.KanbanItem)
func (h *Hub) BroadcastReviewThread(t domain.ReviewThread)
```

Each `BroadcastX` method:
1. Acquires `RLock`
2. Calls `s.PushX(entity)` for each registered subscriber
3. Calls to `PushX` are **non-blocking** — if a subscriber's internal channel is full, skip it (the subscriber is responsible for its own backpressure)

The hub itself does not have a `Run(ctx)` goroutine — broadcast calls are synchronous fan-out under the read lock. This matches Quiver's hub pattern.

## Verification

```bash
cd /Users/char2cs/Projects/Rabbyte/crowbar/api && go build ./internal/app/hub/...
go vet ./internal/app/hub/...
```
