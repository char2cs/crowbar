# Agent 10 — PTY Terminal Handler

**Working directory:** `/Users/char2cs/Projects/Rabbyte/crowbar/api`
**Module:** `github.com/char2cs/crowbar/api`

## Context

The terminal WebSocket handler provides an in-browser PTY terminal pointed at the task's git worktree. It is standalone — no domain events, no chat channel, no Broadcaster.

## Files to read before starting

- `docs/superpowers/specs/2026-05-19-agent-runtime-design.md` §8 (PTY Terminal Handler)
- `api/ARCHITECTURE.md` §"engine/agent/" terminal section

## What already exists

Agents 01–09 complete. Domain `Task` entity defined.

## Package layout

```
internal/api/v0/terminal/
├── terminal.go                  // TerminalHandler interface
└── internal/
    └── handler/
        └── handler.go           // PTY WebSocket implementation
```

## Tasks

### `terminal.go`

```go
package terminal

import (
    "context"
    "github.com/gin-gonic/gin"
)

type TerminalHandler interface {
    Handle(ctx context.Context, c *gin.Context, taskID string)
}

func New(taskRepo repositories.Task) TerminalHandler
```

### `internal/handler/handler.go`

Implements `TerminalHandler.Handle`:

1. Resolve `task.WorktreePath` from task repo; return 404 if task not found
2. Upgrade to WebSocket: `websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}.Upgrade(...)`
3. Determine shell: `os.Getenv("SHELL")` with fallback `"bash"`
4. Start PTY subprocess via `creack/pty`:
   ```go
   cmd := exec.Command(shell)
   cmd.Dir = task.WorktreePath
   ptmx, err := pty.Start(cmd)
   ```
5. Start two goroutines:
   - **PTY → WS:** `io.Copy` from `ptmx` to a loop that calls `conn.WriteMessage(websocket.BinaryMessage, buf)`
   - **WS → PTY:** read WebSocket frames; if JSON with `"type":"resize"` → call `pty.Setsize(ptmx, &pty.Winsize{Cols: cols, Rows: rows})`; otherwise write raw bytes to `ptmx`
6. On WebSocket close or PTY exit: kill cmd, drain both goroutines

Resize frame shape:
```json
{ "type": "resize", "cols": 220, "rows": 50 }
```

Use `sync.WaitGroup` to wait for both goroutines on exit.

## Verification

```bash
cd /Users/char2cs/Projects/Rabbyte/crowbar/api && go build ./internal/api/v0/terminal/...
go vet ./internal/api/v0/terminal/...
```
