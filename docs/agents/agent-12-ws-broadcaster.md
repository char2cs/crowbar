# Agent 12 — WebSocket Broadcaster

**Working directory:** `/Users/char2cs/Projects/Rabbyte/crowbar/api`
**Module:** `github.com/char2cs/crowbar/api`

## Context

The WebSocket Broadcaster delivers typed domain events to connected frontend clients. It implements the `hub.Subscriber` interface so the app hub can push events to it, then fans them out to connected WebSocket clients filtered by entity ID (e.g. only events for `task_id=X` go to clients subscribed to task X).

## Files to read before starting

- `docs/superpowers/specs/2026-05-19-domain-crud-design.md` §4 (WebSocket event envelope, WS endpoints)
- `api/ARCHITECTURE.md` §"Key Wiring Files" — `api/v0/router.go` dispatch()
- Quiver reference: `/Users/char2cs/Projects/Rabbyte/quiver.core/internal/api/v0/` — WS handler patterns

## What already exists

Agents 01–11 complete. Hub interfaces defined. Domain entities defined.

## Package layout

```
internal/api/v0/ws/
├── broadcaster.go   // generic Broadcaster[T] with predicate filtering
└── handler.go       // WS HTTP handler + Subscriber implementation
```

## Tasks

### `broadcaster.go` — `Broadcaster[T any]`

Generic typed broadcaster. Each instance handles one entity type.

```go
type Broadcaster[T any] struct {
    mu      sync.RWMutex
    clients map[string]*wsClient  // client ID → client
}

type wsClient struct {
    id      string
    ch      chan []byte
    pred    func(T) bool  // filter — true = deliver this event
}

func NewBroadcaster[T any]() *Broadcaster[T]

// Subscribe registers a WebSocket client with an optional predicate.
// Returns the client channel and an unsubscribe function.
func (b *Broadcaster[T]) Subscribe(pred func(T) bool) (<-chan []byte, func())

// Broadcast sends the entity to all subscribers whose predicate returns true.
// Non-blocking: clients with full channels are skipped.
func (b *Broadcaster[T]) Broadcast(entity T, eventType string)
```

`Broadcast` wraps the entity in the WS event envelope:
```json
{ "type": "<eventType>", "data": <entity> }
```

Marshal to JSON, then non-blocking send to each matching client channel (cap 64).

### `handler.go` — WS handler + Subscriber

**`WSHandler` struct** — implements `hub.Subscriber`:

```go
type WSHandler struct {
    tasks        *Broadcaster[domain.Task]
    agentRuns    *Broadcaster[domain.AgentRun]
    kanbanItems  *Broadcaster[domain.KanbanItem]
    threads      *Broadcaster[domain.ReviewThread]
}

func NewWSHandler() *WSHandler

// Subscriber interface implementation
func (h *WSHandler) PushTask(t domain.Task)
func (h *WSHandler) PushAgentRun(r domain.AgentRun)
func (h *WSHandler) PushKanbanItem(i domain.KanbanItem)
func (h *WSHandler) PushReviewThread(t domain.ReviewThread)
```

Each `PushX` calls `broadcaster.Broadcast(entity, "task"/"agent_run"/"kanban_item"/"review_thread")`.

**WS endpoint handlers** — one per subscribable resource:

```go
// WS /api/v0/tasks/:id
func (h *WSHandler) HandleTask(c *gin.Context)
// Subscribes with pred: task.ID == taskID
// On connect: sends initial sync frame (current task state via task repo)

// WS /api/v0/tasks/:id/kanban-items  
func (h *WSHandler) HandleKanbanItems(c *gin.Context)
// Subscribes with pred: item.TaskID == taskID

// WS /api/v0/tasks/:id/threads
func (h *WSHandler) HandleThreads(c *gin.Context)
// Subscribes with pred: thread.TaskID == taskID
```

Each handler:
1. Upgrade to WebSocket
2. `ch, unsub := broadcaster.Subscribe(pred)`
3. Start write goroutine: reads from `ch`, calls `conn.WriteMessage(websocket.TextMessage, data)`
4. Ping every 30s
5. On connection close: `unsub()`

### `dispatch()` helper

```go
// Dispatch routes to restHandler or wsHandler based on Upgrade header.
func Dispatch(restHandler gin.HandlerFunc, wsHandler gin.HandlerFunc) gin.HandlerFunc {
    return func(c *gin.Context) {
        if c.GetHeader("Upgrade") == "websocket" {
            wsHandler(c)
        } else {
            restHandler(c)
        }
    }
}
```

Export this from the `ws` package so `router.go` can use it.

## Verification

```bash
cd /Users/char2cs/Projects/Rabbyte/crowbar/api && go build ./internal/api/v0/ws/...
go vet ./internal/api/v0/ws/...
```
