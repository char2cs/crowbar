# Terminal Session Persistence Design

**Date:** 2026-06-27  
**Status:** Approved for implementation  
**Branch:** hardening/production-readiness

## Problem

Terminal sessions in Crowbar die whenever the user switches workspaces. The frontend explicitly calls `DELETE /terminals/:id` on workspace unmount, killing the PTY process dead. This is a showstopper: running processes are lost, history is gone, and the user has to re-orient every time they context-switch.

The goal: terminal sessions survive workspace switches, Crowbar crashes, and machine restarts — without any external dependency like tmux.

## Solution Overview

Implement session persistence entirely in the Go daemon. The PTY is decoupled from the WebSocket via an in-memory ring buffer. Sessions live through three tiers based on usage, with metadata and scrollback persisted to disk alongside each workspace's existing storage. The frontend stops killing sessions on workspace switch and reconnects to existing PTYs on re-entry.

## Session Lifecycle

Sessions move through three states:

```
Active ──switch──► Detached ──idle + limit──► Suspended
  ▲                   │                           │
  └───────────────────┘                           │
       re-enter                re-enter ──────────┘
                               (fresh shell + scrollback replay)
```

**Active** — workspace is open, user is looking at the terminal. WebSocket connected, xterm mounted, PTY running, ring buffer in memory.

**Detached** — user switched to another workspace. WebSocket disconnected, xterm unmounted. PTY keeps running. Output continues writing to the in-memory ring buffer, which flushes to disk periodically. Sessions with a running foreground process stay Detached indefinitely — they are never suspended.

**Suspended** — idle shell (no running process) that has been inactive long enough, or the session count exceeded the soft limit. The shell PTY is killed, the full scrollback is persisted to disk, and the CWD is saved. On re-entry, a fresh shell is spawned in the saved CWD and the scrollback is replayed. From the user's perspective it looks identical to where they left off.

**Machine restart** — all PTY processes die (unavoidable — the OS kills them). The daemon starts automatically via a launchd service. On first attach after restart, suspended sessions restore normally. Detached sessions (that had running processes) become suspended: the process is dead, but scrollback and CWD are preserved on disk.

## Architecture

### Ring Buffer

Each session owns a fixed-size circular byte buffer (default: 2MB, configurable). The PTY goroutine always writes to this buffer. `Attach()` subscribes to the buffer and replays history on connect. When no client is attached, output accumulates silently.

```
PTY goroutine → ring buffer → subscriber (WebSocket) when attached
                            → disk flush (periodic + on detach)
```

The ring buffer is a sliding window over PTY output. Old bytes are overwritten when full. On attach, the full buffer contents are sent first, then live output streams. This is transparent to xterm — it just receives a burst of bytes on connect followed by the live stream.

### Storage Layout

Terminal session data lives alongside each workspace's existing storage:

```
~/.crowbar/
└── projects/<projectId>/<repoId>/workspaces/<wsId>/storages/
    ├── event_stream.db          (existing)
    ├── view.db                  (existing)
    └── terminal_sessions/
        ├── <sessionId>.buf      scrollback ring buffer (binary)
        └── <sessionId>.meta     session metadata (JSON)
```

For home workspaces, the same pattern under the home workspace's storage path.

**`<sessionId>.meta`** (JSON):
```json
{
  "sessionId": "...",
  "workspaceId": "...",
  "cwd": "/path/to/dir",
  "shell": "/bin/zsh",
  "profileId": "...",
  "state": "detached",
  "createdAt": "...",
  "lastActiveAt": "..."
}
```

**`<sessionId>.buf`** — raw PTY output bytes, circular. Written on detach and periodically while detached (every 30s). Read on suspend and restore.

Session metadata is also tracked in `state/view.db` as a GORM model (like `terminal_profiles`) for global queries — e.g., "find all sessions for workspace X" without scanning the filesystem.

### Idle Detection

A session is considered idle when its foreground process group ID equals the shell's PID — i.e., no child process is running in the foreground. This is checked via the PTY's controlling terminal using `tcgetpgrp`. Polling interval: 10 seconds.

Idle sessions are eligible for suspension. Running sessions are never touched automatically.

### Session Limits

**Soft limit: 10 simultaneous Detached sessions per workspace.** When exceeded, the oldest idle Detached session is suspended automatically. Running sessions do not count against this limit — they stay alive regardless.

**No hard limit on PTY processes.** The OS applies backpressure naturally. Memory overhead of an idle detached shell: ~7–10MB (5–8MB process + up to 2MB ring buffer).

### Daemon Auto-Start (launchd)

The Crowbar daemon registers as a launchd service on first launch. On machine restart, launchd starts the daemon before the user opens the app. The daemon loads session metadata from disk and makes sessions available in Suspended state. When the user opens Crowbar and enters a workspace, their terminals reconnect immediately.

## Backend Changes

### `api/internal/engine/terminal/`

**New: `internal/buffer/ring.go`**  
Circular byte buffer. Thread-safe. Methods: `Write([]byte)`, `Snapshot() []byte`, `Flush(io.Writer) error`, `Load(io.Reader) error`.

**Modified: `internal/session/session.go`**  
- Add `RingBuffer` field  
- PTY read goroutine writes to ring buffer instead of directly to a subscriber channel  
- Add `Attach(conn WSConn)` / `Detach()` methods that subscribe/unsubscribe a WebSocket to the buffer  
- Add `IsIdle() bool` via `tcgetpgrp`  
- Add `State` field: `active | detached | suspended`

**Modified: `terminal.go` (engine)**  
- Add `Detach(ctx, sessionID)` — disconnects WebSocket but keeps PTY alive  
- Add `Suspend(ctx, sessionID)` — kills idle PTY, flushes buffer to disk  
- Add `Restore(ctx, sessionID) error` — spawns fresh shell in saved CWD, loads scrollback  
- Add auto-suspension goroutine: polls idle sessions every 60s, suspends when over soft limit  
- Add `LoadPersistedSessions(storagePath)` — called on daemon start, reads `.meta` files  

**New: `internal/persistence/`**  
- `Store` — reads/writes `.meta` and `.buf` files for a given workspace storage path  
- `FlushBuffer(sessionID, buf *ring.Buffer) error`  
- `LoadBuffer(sessionID) (*ring.Buffer, error)`  
- `SaveMeta(SessionMeta) error`  
- `LoadMeta(sessionID) (SessionMeta, error)`  
- `DeleteSession(sessionID) error`  

**Modified: `handlers.go` (interface)**  
Add `Detach(ctx, sessionID) error` to the `TerminalEngine` interface.

**Modified: WS handlers** (`home/handlers/terminal.go`, `terminal/handlers/ws.go`)  
On WebSocket close: call `Detach()` instead of nothing (currently Attach just returns — the PTY is untouched but no explicit detach is recorded).

### `state/view.db` — GORM Model

```go
type TerminalSession struct {
    ID          string `gorm:"primaryKey"`
    WorkspaceID string `gorm:"index"`
    CWD         string
    Shell       string
    ProfileID   string
    State       string // active | detached | suspended
    CreatedAt   time.Time
    LastActiveAt time.Time
}
```

Used for global queries. Kept in sync with the per-session `.meta` files.

## Frontend Changes

### `workspace-store-registry.ts`

Remove the `killTerminalSession` calls from `destroyWorkspaceStore`. On workspace destroy, terminal sessions are left alive on the daemon. The frontend simply forgets them.

### Global Terminal Session Registry

The existing `useTerminalStore` already tracks sessions by `sessionId`. The gap is that session IDs are currently stored only in the per-workspace `buffers` array, which is lost on workspace destroy. 

Add a separate persisted index in IndexedDB (`terminal-sessions` object store):
```ts
{ workspaceId: string, sessionIds: string[], updatedAt: number }
```

This survives workspace unmount. On workspace re-entry, the frontend reads this index to find existing sessions before creating new ones.

### Reconnect on Workspace Re-Entry

In `useTerminalConnection`, if a session already has a known `sessionId` on mount, set `reuseExistingConnection: true`. The `reuseExistingConnection` path already exists — it just needs to be wired to the persisted session IDs on re-entry.

The ring buffer replay arrives as a normal byte burst over the WebSocket. xterm handles it transparently.

### Session State UI

Add a small indicator on terminal tabs:
- **Live** (default) — no indicator
- **Suspended** — subtle grey dot, tooltip "Session suspended — will restore on open"
- **Dead** (process exited while away) — exit code indicator, same as current behavior

## Error Handling

- **Restore fails** (e.g., CWD no longer exists): spawn shell in the workspace root, show a one-line notice in the terminal output.
- **Buffer read error**: start fresh with empty scrollback, log the error.
- **Ring buffer full** (2MB): oldest bytes are overwritten silently. This is expected and intentional.
- **Disk full on flush**: log and continue — the in-memory buffer is still valid for the current session.

## What This Does NOT Solve

**Running processes dying on machine restart.** A `npm run dev` that was running will be dead after a reboot. This is a hard OS constraint — no local technology can freeze and restore a running process on macOS. The user will see the scrollback showing it was running, and will need to restart it manually. This is the only honest gap.

## Implementation Order

1. Ring buffer + decouple PTY from WebSocket (backend core)
2. Detach/suspend/restore lifecycle (backend engine)
3. Disk persistence + daemon restore on start (backend)
4. launchd service registration (Tauri/daemon)
5. Frontend: stop killing on workspace switch + IndexedDB session index
6. Frontend: reconnect on re-entry
7. Frontend: session state UI indicators
