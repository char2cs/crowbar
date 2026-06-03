# Agent 01 — Scaffold Cleanup

**Working directory:** `/Users/char2cs/Projects/Rabbyte/crowbar/api`
**Module:** `github.com/char2cs/crowbar/api`

## Context

The scaffold was generated with an incorrect module path (`rabbytesoftware/crowbar/api`). The `go.mod` has already been corrected to `github.com/char2cs/crowbar/api`, but internal Go files still import the old path. Several scaffold files also need to be deleted or replaced per the architecture spec.

## Files to read before starting

- `api/ARCHITECTURE.md` — §"Scaffold Files to Delete / Rename", §"go.mod Changes Needed"
- `api/go.mod` — confirm current module path

## Tasks

### 1. Fix all import paths

In every `.go` file under `api/`, replace all occurrences of `"github.com/rabbytesoftware/crowbar/api/` with `"github.com/char2cs/crowbar/api/`.

Files known to contain the old path (verify by grepping):
- `internal/internal.go`
- `internal/api/container.go`
- `internal/api/v0/events.go`
- `internal/api/v0/health.go`
- `internal/api/v0/health_test.go`
- `internal/api/v0/router.go`
- `internal/app/container.go`
- `internal/app/hub/hub.go`
- `internal/app/hub/hub_test.go`
- `internal/core/gateway/gateway.go`
- `internal/core/gateway/gateway_test.go`
- `internal/engine/container.go`
- `internal/engine/engine_test.go`
- `internal/domain/domain_test.go`

After replacing, grep for any remaining `rabbytesoftware` occurrences and fix them.

### 2. Update go.mod Go version

Change `go 1.25.0` to `go 1.26.2` in `api/go.mod`.

### 3. Delete scaffold files

Delete the following files — they are replaced by proper implementations in later agents:
- `internal/domain/workspace.go` — entity is `Repository`, not `Workspace`
- `internal/api/v0/events.go` — SSE replaced by WebSocket Broadcaster pattern

### 4. Replace app/hub/hub.go with a typed stub

The scaffold's generic event hub is wrong. Replace `internal/app/hub/hub.go` with a minimal stub that defines the correct interface shape so later agents can import it. Write only the package declaration and an empty `WebSocketHub` interface — agents 6 and 12 will implement it fully.

```go
package hub

import "github.com/char2cs/crowbar/api/internal/domain"

// WebSocketHub is implemented by agents 06 and 12.
type WebSocketHub interface {
    BroadcastTask(t domain.Task)
    BroadcastAgentRun(r domain.AgentRun)
    BroadcastKanbanItem(i domain.KanbanItem)
    BroadcastReviewThread(t domain.ReviewThread)
}
```

Delete `internal/app/hub/hub_test.go` if it references the old hub shape and cannot compile.

### 5. Add required go.mod dependencies

Add the following to `go.mod` (use `go get` or edit directly):
```
github.com/char2cs/asynx
github.com/gorilla/websocket
github.com/mark3labs/mcp-go
github.com/creack/pty
github.com/stretchr/testify
```

Do **not** add `github.com/coder/acp-go-sdk` — it is not yet available.

Run `go mod tidy` after editing.

## Verification

```bash
cd /Users/char2cs/Projects/Rabbyte/crowbar/api && grep -r "rabbytesoftware" --include="*.go" . && echo "FAIL: old paths remain" || echo "OK: no old paths"
go build ./...
```

The build will have compile errors from incomplete scaffold stubs — that is expected. The goal is zero `rabbytesoftware` import paths and a valid `go.mod`.
