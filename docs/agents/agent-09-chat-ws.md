# Agent 09 — Chat WebSocket

**Working directory:** `/Users/char2cs/Projects/Rabbyte/crowbar/api`
**Module:** `github.com/char2cs/crowbar/api`

## Context

The chat WebSocket handler enables bidirectional real-time communication between the frontend and running ACP agent sessions. This agent also defines `ChatFrame` and `AgentRuntime` in `engine/agent/agent.go` — those types are needed by both this package and the MCP server (Agent 13).

## Files to read before starting

- `docs/superpowers/specs/2026-05-19-agent-runtime-design.md` §7 (Hub interface, ChatFrame, ChatHandler), §4 (AgentRuntime interface)
- `api/ARCHITECTURE.md` §"ACP SDK Status" — AgentRuntime interface only; no implementation

## What already exists

Agents 01–08 complete. Domain entities defined. No `engine/agent/` package exists yet.

## Package layout

```
internal/engine/agent/
└── agent.go                // AgentRuntime interface + ChatFrame + ChatFrameType

internal/api/v0/chat/
├── chat.go                 // Hub interface (exported)
└── internal/
    └── handler/
        └── handler.go      // ChatHandler implementation
```

## Tasks

### `internal/engine/agent/agent.go`

Define only the interface and types — no implementation (ACP SDK not available).

```go
package agent

import (
    "context"
    "github.com/char2cs/crowbar/api/internal/domain"
    "github.com/char2cs/crowbar/api/internal/engine/flow"
)

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

type AgentRuntime interface {
    Run(
        ctx   context.Context,
        run   domain.AgentRun,
        task  domain.Task,
        state flow.StateDefinition,
    ) error
}
```

### `internal/api/v0/chat/chat.go`

```go
package chat

import "github.com/char2cs/crowbar/api/internal/engine/agent"

// Re-export ChatFrame so consumers import from chat, not engine/agent
type ChatFrame = agent.ChatFrame
type ChatFrameType = agent.ChatFrameType

// Hub coordinates bidirectional chat between ACP sessions and WebSocket clients.
type Hub interface {
    RegisterSession(taskID string) (input <-chan string, publish func(agent.ChatFrame), unregister func())
    Subscribe(taskID string) (frames <-chan agent.ChatFrame, unsubscribe func())
    Forward(taskID string, content string)
    Publish(taskID string, frame agent.ChatFrame)
}

type ChatHandler interface {
    Handle(ctx context.Context, c *gin.Context, taskID string)
}
```

### Hub implementation

Implement a concrete `hub` struct (unexported) implementing `Hub`:

```go
type hub struct {
    mu       sync.RWMutex
    sessions map[string]*session      // taskID → session
    subs     map[string][]*subscriber // taskID → subscribers
}

type session struct {
    input   chan string
    publish func(ChatFrame)
}

type subscriber struct {
    ch chan ChatFrame
}
```

`RegisterSession`: creates a buffered `input` channel (cap 32). The `publish` func fans out to all `subs[taskID]`. Drops frames for subscribers whose channel is full (non-blocking send). Returns `unregister` that removes the session from `sessions`.

`Subscribe`: creates a buffered channel (cap 64). Returns `unsubscribe` that removes it from `subs[taskID]`.

`Forward`: sends to `sessions[taskID].input` non-blocking; dropped silently if no session or channel full.

`Publish`: fans out frame to all `subs[taskID]` non-blocking.

Expose `func NewHub() Hub`.

### `internal/api/v0/chat/internal/handler/handler.go`

Implements `ChatHandler.Handle`. See spec §7 handler logic:

**On connect:**
1. Upgrade connection: `websocket.Upgrader{}.Upgrade(c.Writer, c.Request, nil)`
2. `frames, unsubscribe := hub.Subscribe(taskID)`
3. Start `readPump` and `writePump` goroutines (use `errgroup` or `sync.WaitGroup`)
4. Wait for either pump to exit; call `unsubscribe()`

**readPump:**
1. `conn.ReadMessage()` → unmarshal `{"type":"user_message","content":"..."}`
2. Write `ConversationMessage{Role: user}` to SQLite via conversation repo
3. `hub.Forward(taskID, content)`

**writePump:**
1. Read from `frames` channel
2. Marshal frame to JSON → `conn.WriteMessage(websocket.TextMessage, ...)`
3. On `agent_turn_end`: write assembled full text as `ConversationMessage{Role: agent, Type: text}` to SQLite
4. Ping every 30s via `conn.WriteMessage(websocket.PingMessage, nil)`
5. On write error: return (triggers unsubscribe in parent)

The handler needs the conversation repo as a dependency — accept it in the constructor.

## Verification

```bash
cd /Users/char2cs/Projects/Rabbyte/crowbar/api && go build ./internal/engine/agent/... ./internal/api/v0/chat/...
go vet ./internal/engine/agent/... ./internal/api/v0/chat/...
```
