# Crowbar — Agent Runtime Spec

**Date:** 2026-05-19
**Status:** Approved
**Sprint:** v0 — Initial Backend

---

## Overview

The Agent Runtime owns everything between "Task entered a new state with an agent" and "AgentRun is marked complete/failed/interrupted". It spawns agent CLI subprocesses via ACP, streams their output to the chat WebSocket in real time, writes artifacts to disk, and drives Task state transitions by evaluating `crowbar_signal()` calls against the Flow Engine.

It also owns the PTY terminal panel — a standalone WebSocket handler with no relation to the domain event system.

**Key libraries:**
- `github.com/coder/acp-go-sdk` — ACP client; spawns subprocess, manages session lifecycle
- `github.com/creack/pty` — PTY for the terminal panel

**Out of scope this sprint:** Docker container pool, Memory injection into agent context, improvement agent triggering.

---

## 1. Package Layout

```
internal/engine/agent/
├── agent.go                        // AgentRuntime interface (exported) + re-exports
└── internal/
    ├── registry/
    │   └── registry.go             // model prefix → AgentEntry
    ├── resolver/
    │   └── resolver.go             // intelligence → model → bin + args
    ├── session/
    │   └── session.go              // ACP session lifecycle
    ├── drain/
    │   └── drain.go                // ACP event drain → events.jsonl + chat channel
    ├── artifacts/
    │   └── artifacts.go            // diff.patch generation at completion
    └── prompt/
        └── prompt.go               // session/prompt assembly

internal/api/v0/chat/
├── chat.go                         // Chat interface (exported) + re-exports
└── internal/
    └── handler/
        └── handler.go              // WebSocket handler: readPump + writePump

internal/api/v0/terminal/
├── terminal.go                     // Terminal interface (exported)
└── internal/
    └── handler/
        └── handler.go              // PTY WebSocket handler
```

`agent.go` exports the `AgentRuntime` interface and re-exports types consumers need from `internal/` (e.g. `type RunResult = session.RunResult`). The unexported `agentRuntime` struct wires the internal subpackages together. Same pattern as Quiver's Wizard and Manifold.

---

## 2. Agent Resolution

### Registry (`internal/registry/registry.go`)

Maps model name prefixes to the CLI binary and ACP flags. Internal to the engine — never exported.

```go
type AgentEntry struct {
    Bin  string
    Args []string // e.g. ["--acp", "--model"] — model name appended at resolve time
}

var entries = map[string]AgentEntry{
    "claude-": {Bin: "claude", Args: []string{"--acp", "--model"}},
    "codex-":  {Bin: "codex",  Args: []string{"acp", "--model"}},
    "gemini-": {Bin: "gemini", Args: []string{"--acp", "--model"}},
}

func Lookup(modelName string) (AgentEntry, bool)
```

`Lookup` iterates entries and returns the first entry whose key is a prefix of `modelName`. Returns `(entry, true)` on match, `("", false)` otherwise.

### Resolver (`internal/resolver/resolver.go`)

```go
func Resolve(
    intelligence flow.IntelligenceLevel,
    cfg          config.Intelligence,
) (bin string, args []string, err error)
```

1. `cfg.ModelFor(intelligence)` → model name string
2. `registry.Lookup(modelName)` → `AgentEntry`
3. Returns `entry.Bin` and `append(entry.Args, modelName)`

Unrecognised model name returns an error — surfaced to the caller at AgentRun creation time, before any subprocess is spawned.

---

## 3. Session Prompt Assembly (`internal/prompt/prompt.go`)

Builds the `session/prompt` payload sent to the ACP session at the start of each AgentRun.

```go
func Build(
    state        flow.StateDefinition,
    priorOutputs []domain.AgentRunOutput,
) string
```

`AgentRunOutput` is a value type `{StateName string, Output string}` derived from completed AgentRuns for this Task in chronological order.

**Prompt structure:**

```
{state.Agent.SystemPrompt}

--- Prior step outputs ---
[brainstorming] {output from brainstorming AgentRun}
[spec]          {output from spec AgentRun}
...
```

Prior outputs are included only if non-empty. No Memory injection this sprint.

---

## 4. ACP Session Lifecycle (`internal/session/session.go`)

### Interface (exported via `agent.go`)

```go
type AgentRuntime interface {
    // Run executes an agent session for the given AgentRun and Task state.
    // The runtime publishes ChatFrames via chatHub and reads user input from it.
    // chatHub is wired at AgentRuntime construction time — not passed per call.
    Run(
        ctx   context.Context,
        run   domain.AgentRun,
        task  domain.Task,
        state flow.StateDefinition,
    ) error
}
```

### Execution steps

1. Open `events.jsonl` writer: `paths.RunsAt(run.ID)/events.jsonl`
2. `resolver.Resolve(state.Agent.Intelligence, cfg)` → `bin`, `args`
3. Spawn ACP subprocess via `acp-go-sdk`:
   - Working directory: `task.WorktreePath`
   - Bin + args from resolver
4. ACP `initialize` → `session/new`
5. `prompt.Build(state, priorOutputs)` → send `session/prompt` with MCP server config attached
6. `input, publish, unregister := chatHub.RegisterSession(task.ID)` — register with hub before drain starts
7. Start drain goroutine: ACP events → `events.jsonl` + `publish` function (see §5)
8. Block until one of:
   - User message arrives on `input` channel → forward to ACP session via `acpConn.Send()`; loop back to blocking
   - `crowbar_signal()` fires via MCP → go to step 9
   - Subprocess exits without signal → `FailAgentRun`; call `unregister()`; return error
   - `ctx` cancelled → `InterruptAgentRun`; terminate subprocess; call `unregister()`; return
9. Stop drain goroutine; flush `events.jsonl`
10. `unregister()` — deregister from hub
11. `artifacts.WriteDiff(task.WorktreePath, task.BaseBranch, task.BranchName, destPath)` → write `diff.patch`
12. Issue `CompleteAgentRun(output)` Asynx command
13. `flow.Evaluate(loadedFlow, task.CurrentState, event)` → next state name
14. Issue `AdvanceState(nextState)` Asynx command on Task aggregate

### Multi-turn chat (chat states)

In states where `state.UI == flow.UIModeChat`, the session does not complete after the first agent response. The ACP session remains open. The session goroutine reads user messages from the `input` channel (provided by `chatHub.RegisterSession`) and forwards them into the ACP session via `acpConn.Send()`. The WebSocket handler routes user messages to that channel via `chatHub.Forward()`. The agent reads them and continues. This loop runs until `crowbar_signal()` fires or the session is interrupted.

In non-chat states (`kanban`, `diff`, `background`), the session runs autonomously to completion — no user input is accepted mid-session.

---

## 5. Event Drain (`internal/drain/drain.go`)

Runs as a dedicated goroutine for the lifetime of the ACP session. Owns two sinks: `events.jsonl` on disk and the `publish` function for the frontend.

```go
func Run(
    ctx     context.Context,
    events  <-chan acpsdk.SessionUpdate,
    dest    io.WriteCloser,
    publish func(ChatFrame),
) error
```

**Per-event handling:**

| ACP event type | events.jsonl | publish call |
|---|---|---|
| Text chunk | Written as JSON line | calls `publish` with `agent_chunk` frame with `delta` |
| Turn end | Written as JSON line | calls `publish` with `agent_turn_end` frame; backend assembles full text → writes `ConversationMessage` to SQLite |
| Tool call | Written as JSON line | calls `publish` with `tool_call` frame |
| Tool result | Written as JSON line | calls `publish` with `tool_result` frame |
| Other (metadata, ping) | Written as JSON line | Not forwarded |

All writes to `events.jsonl` are append-only, newline-delimited JSON. Writes are sequential (single goroutine owns the file) — no locking needed.

Calls to `publish` are non-blocking within the hub — slow or disconnected clients do not stall the drain goroutine. The drain goroutine calls `publish` synchronously; the hub handles fan-out and backpressure internally.

On `ctx` cancel or events channel close: flush remaining writes, close `dest`, return.

---

## 6. Artifacts (`internal/artifacts/artifacts.go`)

Called once at AgentRun completion, after the drain goroutine has been stopped.

```go
func WriteDiff(
    worktreePath string,
    baseBranch   string,
    branchName   string,
    destPath     string,
) error
```

Runs `git diff {baseBranch}...{branchName}` in `worktreePath`, writes stdout to `destPath` (`~/.crowbar/runs/{run_id}/diff.patch`). An empty diff is valid and written as an empty file. Git errors (unknown branch, unborn branch) are returned as errors and cause `FailAgentRun`.

---

## 7. Chat WebSocket Handler (`internal/api/v0/chat/`)

### ChatHub interface

```go
// Hub coordinates bidirectional chat between ACP sessions and WebSocket clients.
// Multiple clients may subscribe to the same task concurrently — all receive the same frames.
// Forward delivers user messages to the running session; if no session is registered, the message is dropped.
type Hub interface {
    // RegisterSession is called by the AgentRuntime before the drain goroutine starts.
    // Returns: input channel the session reads user messages from, a publish function
    // for pushing ChatFrames to all subscribers, and an unregister function to call on exit.
    RegisterSession(taskID string) (input <-chan string, publish func(ChatFrame), unregister func())

    // Subscribe registers a WebSocket client to receive frames for taskID.
    // The returned channel is buffered (cap 64). Call unsubscribe on disconnect.
    Subscribe(taskID string) (frames <-chan ChatFrame, unsubscribe func())

    // Forward delivers a user message to the session registered for taskID.
    // Non-blocking: if no session is registered or the session's input is full, the message is dropped.
    Forward(taskID string, content string)

    // Publish sends a frame directly to all subscribers for taskID.
    // Used by crowbar_signal (MCP server) to push state_transition frames independently of the session.
    // Non-blocking: subscribers with full buffers are skipped.
    Publish(taskID string, frame ChatFrame)
}
```

### Interface

```go
type ChatHandler interface {
    Handle(
        ctx      context.Context,
        c        *gin.Context,
        taskID   string,
    )
}
```

### Handler logic

Registered at `WS /api/v0/tasks/:id/chat`. Not part of the domain Broadcaster system — standalone handler.

**On connect:**
1. Upgrade to WebSocket
2. Look up Task by `taskID`; resolve active AgentRun
3. If no AgentRun is running, send an `agent_turn_end` frame immediately (nothing to stream) and hold the connection open for when one starts
4. `frames, unsubscribe := chatHub.Subscribe(taskID)` — subscribe to receive ChatFrames for this task; any currently-running session's publish calls will reach this subscriber
5. Start `readPump` and `writePump` goroutines

**readPump** — reads incoming WebSocket frames from the client:
1. Unmarshal frame; expect `type: "user_message"`
2. Write `ConversationMessage{Role: user, Type: text, Content: ...}` to SQLite immediately
3. `chatHub.Forward(taskID, content)` — routes the message to the running session's input channel; dropped silently if no session is registered
4. If no ACP session is running (human-only state or paused task), drop the message

**writePump** — reads from the `frames` channel and writes to WebSocket:
1. Receive `ChatFrame` from `frames` channel (the per-subscriber channel returned by `chatHub.Subscribe`)
2. Marshal to JSON envelope
3. `websocket.WriteMessage` to client
4. On `tool_call` or `tool_result` frames: also write `ConversationMessage` to SQLite
5. On `agent_turn_end`: write assembled full text as `ConversationMessage{Role: agent, Type: text}` to SQLite
6. Ping every 30s; drop client on write error. On disconnect or write error: call `unsubscribe()` to deregister from the hub.

### ChatFrame type

Defined in `engine/agent/` (re-exported via `agent.go`) — **not** in the API layer. This keeps the import direction clean: `api/v0/chat` imports from `engine/agent`, never the reverse.

```go
type ChatFrameType string

const (
    ChatFrameTypeUserMessage     ChatFrameType = "user_message"
    ChatFrameTypeAgentChunk      ChatFrameType = "agent_chunk"
    ChatFrameTypeAgentTurnEnd    ChatFrameType = "agent_turn_end"
    ChatFrameTypeToolCall        ChatFrameType = "tool_call"
    ChatFrameTypeToolResult      ChatFrameType = "tool_result"
    ChatFrameTypeStateTransition ChatFrameType = "state_transition"
)

type ChatFrame struct {
    Type      ChatFrameType `json:"type"`
    MessageID string        `json:"message_id,omitempty"`
    Delta     string        `json:"delta,omitempty"`
    Tool      string        `json:"tool,omitempty"`
    Args      any           `json:"args,omitempty"`
    Result    any           `json:"result,omitempty"`
    NewState  string        `json:"new_state,omitempty"`
    Content   string        `json:"content,omitempty"`
}
```

`state_transition` frames are emitted by the MCP server when `crowbar_signal()` fires and a state transition is evaluated — pushed via `chatHub.Publish()` before the session closes, so the frontend can navigate to the new state's view.

---

## 8. PTY Terminal Handler (`internal/api/v0/terminal/`)

### Interface

```go
type TerminalHandler interface {
    Handle(
        ctx    context.Context,
        c      *gin.Context,
        taskID string,
    )
}
```

### Handler logic

Registered at `WS /api/v0/tasks/:id/terminal`. Standalone — no domain events, no chat channel, no Broadcaster.

1. Resolve `task.WorktreePath` from task repository
2. Upgrade to WebSocket
3. Start PTY subprocess: `$SHELL` (fallback `bash`) with `WorkDir = task.WorktreePath` via `creack/pty`
4. Start two goroutines:
   - **PTY → WebSocket:** reads PTY output, sends as binary WebSocket frames
   - **WebSocket → PTY:** reads WebSocket frames; text frames go to PTY stdin; JSON frames with `type: "resize"` call `pty.Setsize(cols, rows)`
5. On WebSocket close or PTY exit: kill PTY process, clean up both goroutines

**Resize frame:**

```json
{ "type": "resize", "cols": 220, "rows": 50 }
```

All other frames from client are treated as raw stdin bytes.

---

## 9. Shutdown & Interrupt Handling

**User-initiated interrupt** (`POST /api/v0/agent-runs/:id/interrupt`):
- Usecase cancels the session's `ctx`
- Session goroutine detects `ctx.Done()`, terminates subprocess, issues `InterruptAgentRun`
- Drain goroutine flushes and exits
- Task status moves to `paused`

**Subprocess crash** (process exits without `crowbar_signal()`):
- ACP events channel closes
- Drain goroutine exits naturally
- Session detects closed channel with no signal received → issues `FailAgentRun`
- Task status moves to `paused`

**Server restart** (crash recovery):
- `app.New()` calls `agentRunRepo.RecoverOrphanedRuns(ctx)` at startup
- Any AgentRun with status `running` and no terminal event is marked `failed`
- Task remains in its last known state; user can Retry or force-transition

**Retry** (`POST /api/v0/tasks/:id/retry`):
- Creates a new AgentRun for the same state
- Prior failed/interrupted run's `output` is not forwarded
- Full session lifecycle starts fresh
