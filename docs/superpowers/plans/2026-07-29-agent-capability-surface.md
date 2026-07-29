# Agent Capability Surface Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the agent running inside a Crowbar chat reach Crowbar itself — read and answer the user's code-review threads, see the workspaces and chats it is allowed to see, and title its own chat — through an MCP server hosted by the daemon and injected into every managed vendor CLI session.

**Architecture:** The MCP protocol handler lives **in the daemon**, beside the usecases. `crowbar mcp` is a thin stdio relay: it reads a JSON-RPC message from stdin, POSTs it to a runner-keyed daemon route, and writes the response to stdout. Three layers — `engine/mcp` (pure protocol, zero Crowbar knowledge), `app/usecases/agenttools` (tool definitions, scope resolution, calls into existing usecases), `api/.../endpoints/agent/handlers/mcp.go` (HTTP seam). Injection into both vendor CLIs is pure `config_injection` YAML in the existing descriptors.

**Tech Stack:** Go 1.x, gin, cobra, testify. MCP protocol revision `2025-11-25`, hand-rolled (no SDK).

**Spec:** `docs/superpowers/specs/2026-07-29-agent-capability-surface-design.md`

## Global Constraints

- **MCP protocol revision is exactly `2025-11-25`.** Both claude 2.1.220 and codex 0.139.0 pin this. Do not implement `2026-07-28` semantics (stateless core, MRTR, `server/discover`, `resultType`).
- **Never write to a provider's home directory.** Not `~/.codex`, not `~/.claude`, not additively. All Crowbar state goes under the workspace's chats dir.
- **No skills of any kind are produced by this plan.** No `SKILL.md`, no `--plugin-dir`, no `.agents/`, no `write_skills` inject verb.
- **No tool accepts a workspace, project, repo, chat-scope or runner argument.** Authority is derived server-side from the caller's runner. `get_chat_log` takes a chat id, which is validated against the caller's visible set.
- **No new descriptor field and no new inject verb.** Injection uses the existing `pass_arg` verb inside `config_injection`.
- **Tool count ceiling is 8.** codex does not defer tool schemas; every tool costs context on every codex turn.
- **Tool results are MCP text content. Never declare `outputSchema`.** Under `2025-11-25` declaring it makes the server send the payload twice.
- **Go style:** follow the `go-style` skill and surrounding code. Errors wrap with `fmt.Errorf("<pkg>: <action>: %w", err)`. Comments explain *why*, not *what*.
- **Test location:** Go tests live beside the code (`foo_test.go`); black-box regression tests for fixed bugs go in `api/tests`; integration tests needing real vendor CLIs go in `api/tests/integration/agent/` behind `//go:build integration`.
- **No timing in tests.** No sleeps, no polling, no `Eventually`. Block on real signals.
- **Commit after every task.** Do not push. Do not open a PR.

## Key existing code this plan binds to

| what | where |
|---|---|
| Runner aggregate (`ID` == crowbarSegmentID) | `api/internal/domain/agent_runner.go:19` |
| Runner lookup by id | `agentrunner.EventStore.Get` — `api/internal/app/repositories/agentrunner/event_store.go:117` |
| segment → runner → *current* chat | `(*Usecase).RenameByRunner` — `api/internal/app/usecases/agent/agent.go:258` |
| Spawn seam (mints runnerID, builds TemplateCtx) | `(*Usecase).spawnRunner` — `api/internal/app/usecases/agent/agent.go:671` |
| Template vars + `Expand` | `api/internal/engine/agent/template.go` |
| Inject verbs (`set_env`, `write_file`, `pass_arg`) | `api/internal/engine/agent/inject.go:55` |
| Agent routes (workspace mount) | `api/internal/api/v0/endpoints/agent/routes.go:36` |
| Agent routes (project-home mount) | `api/internal/api/v0/endpoints/home/routes.go:96-108` |
| `AgentUsecase` port | `api/internal/api/v0/endpoints/agent/handlers/handlers.go:15` |
| Thread store port | `api/internal/api/v0/endpoints/threads/handlers/handlers.go:20` |
| Thread aggregate | `api/internal/domain/review_thread.go:7` |
| `reviewthread.OpenInput` | `api/internal/app/repositories/reviewthread/reviewthread.go:39` |
| Branch review usecase | `api/internal/app/usecases/branchreview/branch_review.go:37` |
| Unexported base-ref resolution | `branch_review.go:159` (`resolveDiffRef`) |
| Ledger | `api/internal/app/ledger/ledger.go` |
| Workspace aggregate (`ParentID`, `IsDefault`, `Kind`) | `api/internal/domain/workspace.go:20` |
| CLI scope helpers | `api/cmd/crowbar/scope.go` |
| ipc client | `api/internal/core/ipc/client.go` |
| Prompts config | `api/internal/core/config/default.yaml` |
| PTY-drive test helper | `api/tests/integration/agent/barriers_test.go` |

## File structure

**Created**

| file | responsibility |
|---|---|
| `api/internal/engine/mcp/jsonrpc.go` | JSON-RPC 2.0 envelope types + error codes. No MCP, no Crowbar. |
| `api/internal/engine/mcp/protocol.go` | MCP `2025-11-25` types: `Tool`, `InitializeResult`, `CallToolResult`, `TextContent`. |
| `api/internal/engine/mcp/server.go` | `Server` — dispatches one JSON-RPC message against a `ToolSet`. Zero Crowbar knowledge. |
| `api/internal/app/usecases/agenttools/tokens.go` | Per-boot HMAC runner token: mint + verify. |
| `api/internal/app/usecases/agenttools/scope.go` | segment+token → runner → chat → workspace → visible workspace set. |
| `api/internal/app/usecases/agenttools/toolset.go` | `ToolSet` implementation: registry, dispatch, arg decoding. |
| `api/internal/app/usecases/agenttools/tools_chat.go` | `set_chat_title`. |
| `api/internal/app/usecases/agenttools/tools_review.go` | The five review-thread tools. |
| `api/internal/app/usecases/agenttools/tools_context.go` | `list_workspaces`, `get_chat_log`. |
| `api/internal/app/usecases/agenttools/render.go` | Line-oriented text rendering of tool results. |
| `api/internal/app/usecases/agenttools/metrics.go` | Per-tool call counters. |
| `api/internal/api/v0/endpoints/agent/handlers/mcp.go` | `POST .../agent/runners/:segid/mcp` HTTP seam. |
| `api/cmd/crowbar/mcp.go` | The stdio relay. |

**Modified**

| file | change |
|---|---|
| `api/internal/engine/agent/template.go` | Add `RunnerToken` field + `{runner_token}` to `Expand`. |
| `api/internal/app/usecases/agent/agent.go` | Mint token in `spawnRunner`, set on `TemplateCtx`. |
| `api/internal/engine/agent/descriptors/claude.yaml` | `--mcp-config` injection. |
| `api/internal/engine/agent/descriptors/codex.yaml` | `-c mcp_servers.crowbar.*` injection. |
| `api/internal/core/config/default.yaml` + `config.go` | `capabilities_instruction` prompt. |
| `api/internal/api/v0/endpoints/agent/routes.go` | Mount the MCP route. |
| `api/internal/api/v0/endpoints/home/routes.go` | Mount the same route under `/home`. |
| `api/internal/api/v0/route_audit_test.go` | Declare the route. |
| `api/internal/api/v0/endpoints/agent/handlers/handlers.go` | Extend `AgentUsecase` with `DispatchMCP`. |
| `api/internal/app/usecases/branchreview/branch_review.go` | Export `GetBase`. |
| `api/internal/api/v0/endpoints/review/handlers/handlers.go` | Add `GetBase` to the `ReviewUsecase` port. |
| `api/cmd/crowbar/main.go` | Register `newMCPCmd()`. |

---

# Phase 0 — Skeleton and the control experiment

The deliverable of Phase 0 is a real claude and a real codex both registering the `crowbar` MCP server and successfully calling **one** tool, `set_chat_title`. Nothing else. Its failure rate is already known, so it is the measurement that de-risks the rest.

---

### Task 1: JSON-RPC envelope types

**Files:**
- Create: `api/internal/engine/mcp/jsonrpc.go`
- Test: `api/internal/engine/mcp/jsonrpc_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `mcp.Request`, `mcp.Response`, `mcp.RPCError`, `mcp.NewError(id, code, message) Response`, `mcp.NewResult(id, any) (Response, error)`, and the constants `mcp.CodeParseError = -32700`, `CodeInvalidRequest = -32600`, `CodeMethodNotFound = -32601`, `CodeInvalidParams = -32602`, `CodeInternalError = -32603`.

- [ ] **Step 1: Write the failing test**

```go
package mcp_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/mcp"
)

func TestRequest_DecodesIDAndParams(t *testing.T) {
	var req mcp.Request
	require.NoError(t, json.Unmarshal([]byte(`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"x"}}`), &req))
	require.Equal(t, "2.0", req.JSONRPC)
	require.Equal(t, "tools/call", req.Method)
	require.JSONEq(t, `{"name":"x"}`, string(req.Params))
	require.False(t, req.IsNotification())
}

func TestRequest_NotificationHasNoID(t *testing.T) {
	var req mcp.Request
	require.NoError(t, json.Unmarshal([]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`), &req))
	require.True(t, req.IsNotification())
}

func TestNewError_ShapesJSONRPCError(t *testing.T) {
	resp := mcp.NewError(json.RawMessage(`3`), mcp.CodeMethodNotFound, "no such method")
	b, err := json.Marshal(resp)
	require.NoError(t, err)
	require.JSONEq(t,
		`{"jsonrpc":"2.0","id":3,"error":{"code":-32601,"message":"no such method"}}`,
		string(b))
}

func TestNewResult_OmitsErrorField(t *testing.T) {
	resp, err := mcp.NewResult(json.RawMessage(`1`), map[string]string{"ok": "yes"})
	require.NoError(t, err)
	b, err := json.Marshal(resp)
	require.NoError(t, err)
	require.JSONEq(t, `{"jsonrpc":"2.0","id":1,"result":{"ok":"yes"}}`, string(b))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./internal/engine/mcp/... -run 'TestRequest|TestNewError|TestNewResult' -v`
Expected: FAIL — package `mcp` does not exist.

- [ ] **Step 3: Write the implementation**

```go
// Package mcp implements the Model Context Protocol revision 2025-11-25 —
// the revision both vendor CLIs Crowbar drives actually speak (claude 2.1.220
// and codex 0.139.0 both pin it). The 2026-07-28 revision is deliberately NOT
// implemented here: it removes the initialize handshake, replaces
// server-initiated requests with Multi Round-Trip Requests and requires
// server/discover, none of which any client Crowbar targets can use yet.
//
// This package knows nothing about Crowbar. It speaks protocol and delegates
// every tool decision to a ToolSet.
package mcp

import "encoding/json"

// JSON-RPC 2.0 error codes (the -32000..-32099 server range is left alone).
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

// Request is one inbound JSON-RPC message. ID is kept as RawMessage because
// JSON-RPC permits a string, a number or null, and a response MUST echo the id
// back with the same type it arrived as — decoding into any Go scalar would
// silently rewrite it.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// IsNotification reports whether the message carries no id. A notification gets
// no reply at all — not even an error — per JSON-RPC 2.0.
func (r Request) IsNotification() bool {
	return len(r.ID) == 0 || string(r.ID) == "null"
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type Response struct {
	JSONRPC string `json:"jsonrpc"`
	// No omitempty: JSON-RPC 2.0 requires id to be PRESENT and null when the
	// request's id could not be determined (a parse error). With omitempty a nil
	// RawMessage drops the member entirely, which is a protocol violation. A nil
	// RawMessage marshals to `null` on its own, so dropping the tag is the fix.
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *RPCError       `json:"error,omitempty"`
}

func NewError(id json.RawMessage, code int, message string) Response {
	return Response{JSONRPC: "2.0", ID: id, Error: &RPCError{Code: code, Message: message}}
}

func NewResult(id json.RawMessage, result any) (Response, error) {
	raw, err := json.Marshal(result)
	if err != nil {
		return Response{}, fmt.Errorf("mcp: marshal result: %w", err)
	}
	return Response{JSONRPC: "2.0", ID: id, Result: raw}, nil
}
```

(imports: `encoding/json`, `fmt`)

- [ ] **Step 4: Run test to verify it passes**

Run: `cd api && go test ./internal/engine/mcp/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/engine/mcp/
git commit -m "feat(mcp): JSON-RPC 2.0 envelope types for the agent tool surface"
```

---

### Task 2: MCP protocol types and server dispatch

**Files:**
- Create: `api/internal/engine/mcp/protocol.go`, `api/internal/engine/mcp/server.go`
- Test: `api/internal/engine/mcp/server_test.go`

**Interfaces:**
- Consumes: Task 1's `Request`/`Response`/`New*`/code constants.
- Produces:
  - `type Tool struct { Name, Description string; InputSchema json.RawMessage }`
  - `type ToolSet interface { Tools() []Tool; Call(ctx context.Context, name string, args json.RawMessage) (string, error) }`
  - `func NewServer(name, version string, tools ToolSet) *Server`
  - `func (s *Server) Handle(ctx context.Context, raw []byte) ([]byte, bool)` — returns the response bytes and `false` when the message was a notification (nothing to send).

- [ ] **Step 1: Write the failing test**

```go
package mcp_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/mcp"
)

type fakeTools struct {
	called     string
	calledArgs string
	err        error
}

func (f *fakeTools) Tools() []mcp.Tool {
	return []mcp.Tool{{
		Name:        "set_chat_title",
		Description: "Set this chat's title.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"title":{"type":"string"}},"required":["title"]}`),
	}}
}

func (f *fakeTools) Call(_ context.Context, name string, args json.RawMessage) (string, error) {
	f.called, f.calledArgs = name, string(args)
	if f.err != nil {
		return "", f.err
	}
	return "titled", nil
}

func srv(f *fakeTools) *mcp.Server { return mcp.NewServer("crowbar", "test", f) }

func TestServer_InitializePinsProtocolRevision(t *testing.T) {
	out, send := srv(&fakeTools{}).Handle(context.Background(),
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	require.True(t, send)

	var resp struct {
		Result struct {
			ProtocolVersion string `json:"protocolVersion"`
			Capabilities    struct {
				Tools map[string]any `json:"tools"`
			} `json:"capabilities"`
			ServerInfo struct {
				Name string `json:"name"`
			} `json:"serverInfo"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out, &resp))
	require.Equal(t, "2025-11-25", resp.Result.ProtocolVersion)
	require.NotNil(t, resp.Result.Capabilities.Tools)
	require.Equal(t, "crowbar", resp.Result.ServerInfo.Name)
}

func TestServer_NotificationGetsNoReply(t *testing.T) {
	out, send := srv(&fakeTools{}).Handle(context.Background(),
		[]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	require.False(t, send)
	require.Nil(t, out)
}

func TestServer_ToolsListDoesNotDeclareOutputSchema(t *testing.T) {
	out, _ := srv(&fakeTools{}).Handle(context.Background(),
		[]byte(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`))
	// Declaring outputSchema obliges the server to send structuredContent AND a
	// serialized copy in content — the payload twice. Never declare it.
	require.NotContains(t, string(out), "outputSchema")
	require.Contains(t, string(out), "set_chat_title")
}

func TestServer_ToolsCallReturnsTextContent(t *testing.T) {
	f := &fakeTools{}
	out, _ := srv(f).Handle(context.Background(),
		[]byte(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"set_chat_title","arguments":{"title":"Hi"}}}`))

	require.Equal(t, "set_chat_title", f.called)
	require.JSONEq(t, `{"title":"Hi"}`, f.calledArgs)

	var resp struct {
		Result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out, &resp))
	require.False(t, resp.Result.IsError)
	require.Len(t, resp.Result.Content, 1)
	require.Equal(t, "text", resp.Result.Content[0].Type)
	require.Equal(t, "titled", resp.Result.Content[0].Text)
}

// A failing tool is a TOOL error (isError:true in a successful result), not a
// JSON-RPC error — that is what lets the model read the failure and retry
// instead of the client tearing the connection down.
func TestServer_ToolFailureIsToolErrorNotRPCError(t *testing.T) {
	out, _ := srv(&fakeTools{err: errors.New("thread not visible")}).Handle(context.Background(),
		[]byte(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"set_chat_title","arguments":{}}}`))

	var resp struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *mcp.RPCError `json:"error"`
	}
	require.NoError(t, json.Unmarshal(out, &resp))
	require.Nil(t, resp.Error)
	require.True(t, resp.Result.IsError)
	require.Contains(t, resp.Result.Content[0].Text, "thread not visible")
}

func TestServer_UnknownMethodIsRPCError(t *testing.T) {
	out, _ := srv(&fakeTools{}).Handle(context.Background(),
		[]byte(`{"jsonrpc":"2.0","id":5,"method":"resources/list"}`))
	var resp mcp.Response
	require.NoError(t, json.Unmarshal(out, &resp))
	require.NotNil(t, resp.Error)
	require.Equal(t, mcp.CodeMethodNotFound, resp.Error.Code)
}

func TestServer_MalformedJSONIsParseError(t *testing.T) {
	out, send := srv(&fakeTools{}).Handle(context.Background(), []byte(`{not json`))
	require.True(t, send)
	var resp mcp.Response
	require.NoError(t, json.Unmarshal(out, &resp))
	require.Equal(t, mcp.CodeParseError, resp.Error.Code)
	// JSON-RPC 2.0: when the request's id cannot be determined, id MUST be
	// present and null. Asserting on the raw bytes because an absent member and
	// an explicit null both decode to a nil RawMessage.
	require.Contains(t, string(out), `"id":null`)
}

// tools/list must never emit `"tools":null` — a ToolSet with no tools yet is an
// empty array. A null there is a wire shape some clients reject outright.
func TestServer_ToolsListEmitsAnArrayWhenThereAreNoTools(t *testing.T) {
	out, _ := srv(&fakeTools{empty: true}).Handle(context.Background(),
		[]byte(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`))
	require.Contains(t, string(out), `"tools":[]`)
}

func TestServer_PingReturnsEmptyResult(t *testing.T) {
	out, _ := srv(&fakeTools{}).Handle(context.Background(),
		[]byte(`{"jsonrpc":"2.0","id":6,"method":"ping"}`))
	require.JSONEq(t, `{"jsonrpc":"2.0","id":6,"result":{}}`, string(out))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./internal/engine/mcp/... -run TestServer -v`
Expected: FAIL — `mcp.NewServer` undefined.

- [ ] **Step 3: Write `protocol.go`**

```go
package mcp

import (
	"context"
	"encoding/json"
)

// ProtocolVersion is the revision this server speaks. See the package doc for
// why it is not 2026-07-28.
const ProtocolVersion = "2025-11-25"

// Tool is one advertised tool. InputSchema is a raw JSON Schema object.
//
// There is deliberately no OutputSchema field: under 2025-11-25 a tool that
// declares one must return structuredContent AND should also serialize the same
// data into content, which sends the whole payload twice. Results here are text.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// ToolSet is everything the protocol layer needs to know about Crowbar: nothing,
// beyond a list of tools and a way to call one. Call returns the text a model
// should read; an error is rendered as a TOOL error, not a transport error.
type ToolSet interface {
	Tools() []Tool
	Call(ctx context.Context, name string, args json.RawMessage) (string, error)
}

type initializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ServerInfo      serverInfo     `json:"serverInfo"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type toolsListResult struct {
	Tools []Tool `json:"tools"`
}

type textContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type callToolResult struct {
	Content []textContent `json:"content"`
	IsError bool          `json:"isError"`
}

type callToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}
```

- [ ] **Step 4: Write `server.go`**

```go
package mcp

import (
	"context"
	"encoding/json"
)

// Server dispatches a single JSON-RPC message against a ToolSet. It holds no
// per-connection state: every message is self-contained, which is what lets the
// transport be one HTTP POST per message (see cmd/crowbar's mcp relay).
type Server struct {
	name    string
	version string
	tools   ToolSet
}

func NewServer(name, version string, tools ToolSet) *Server {
	return &Server{name: name, version: version, tools: tools}
}

// Handle processes one inbound message. The bool reports whether anything should
// be sent back: a JSON-RPC notification is answered with silence, never with an
// error, so a client that sends notifications/initialized is not confused by a
// stray reply.
func (s *Server) Handle(ctx context.Context, raw []byte) ([]byte, bool) {
	var req Request
	if err := json.Unmarshal(raw, &req); err != nil {
		return marshal(NewError(nil, CodeParseError, "invalid JSON")), true
	}
	if req.IsNotification() {
		return nil, false
	}

	switch req.Method {
	case "initialize":
		return marshal(s.result(req.ID, initializeResult{
			ProtocolVersion: ProtocolVersion,
			// An empty object, not nil: the tools capability must be PRESENT for a
			// client to call tools/list, and `"tools":null` does not count.
			Capabilities: map[string]any{"tools": map[string]any{}},
			ServerInfo:   serverInfo{Name: s.name, Version: s.version},
		})), true

	case "ping":
		return marshal(s.result(req.ID, struct{}{})), true

	case "tools/list":
		return marshal(s.result(req.ID, toolsListResult{Tools: s.tools.Tools()})), true

	case "tools/call":
		var p callToolParams
		if err := json.Unmarshal(req.Params, &p); err != nil || p.Name == "" {
			return marshal(NewError(req.ID, CodeInvalidParams, "params must carry a tool name")), true
		}
		text, err := s.tools.Call(ctx, p.Name, p.Arguments)
		if err != nil {
			// A tool failure is data the model should read and act on, so it rides
			// back as a successful result carrying isError — not as a JSON-RPC
			// error, which clients treat as a transport fault.
			return marshal(s.result(req.ID, callToolResult{
				Content: []textContent{{Type: "text", Text: err.Error()}},
				IsError: true,
			})), true
		}
		return marshal(s.result(req.ID, callToolResult{
			Content: []textContent{{Type: "text", Text: text}},
		})), true

	default:
		return marshal(NewError(req.ID, CodeMethodNotFound, "unsupported method "+req.Method)), true
	}
}

func (s *Server) result(id json.RawMessage, v any) Response {
	resp, err := NewResult(id, v)
	if err != nil {
		return NewError(id, CodeInternalError, "encode result: "+err.Error())
	}
	return resp
}

func marshal(resp Response) []byte {
	b, err := json.Marshal(resp)
	if err != nil {
		// The only way this fails is a non-marshalable result, which result()
		// already converted to an error response — this is belt and braces.
		return []byte(`{"jsonrpc":"2.0","error":{"code":-32603,"message":"encode response"}}`)
	}
	return b
}
```

- [ ] **Step 5: Run tests**

Run: `cd api && go test ./internal/engine/mcp/... -v`
Expected: PASS, all tests.

- [ ] **Step 6: Commit**

```bash
git add api/internal/engine/mcp/
git commit -m "feat(mcp): protocol 2025-11-25 dispatch over a ToolSet"
```

---

### Task 3: Per-boot runner token

**Files:**
- Create: `api/internal/app/usecases/agenttools/tokens.go`
- Test: `api/internal/app/usecases/agenttools/tokens_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `func NewTokenMinter() (*TokenMinter, error)`, `func (m *TokenMinter) Mint(runnerID string) string`, `func (m *TokenMinter) Verify(runnerID, token string) bool`.

**Why this shape:** a runner's PTY is a child of the daemon process, so a daemon restart kills every runner. A secret minted per boot therefore never outlives the runners it authenticates — revocation is free and no schema change to the event-sourced `AgentRunner` is needed.

- [ ] **Step 1: Write the failing test**

```go
package agenttools_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/agenttools"
)

func TestTokenMinter_VerifiesItsOwnToken(t *testing.T) {
	m, err := agenttools.NewTokenMinter()
	require.NoError(t, err)

	tok := m.Mint("runner-a")
	require.NotEmpty(t, tok)
	require.True(t, m.Verify("runner-a", tok))
}

func TestTokenMinter_RejectsTokenForAnotherRunner(t *testing.T) {
	m, err := agenttools.NewTokenMinter()
	require.NoError(t, err)

	// The whole point: holding runner A's token must not grant runner B's scope.
	require.False(t, m.Verify("runner-b", m.Mint("runner-a")))
}

func TestTokenMinter_RejectsGarbage(t *testing.T) {
	m, err := agenttools.NewTokenMinter()
	require.NoError(t, err)

	require.False(t, m.Verify("runner-a", ""))
	require.False(t, m.Verify("runner-a", "not-base64-$$$"))
	require.False(t, m.Verify("", m.Mint("")))
}

// Two minters model two daemon boots. Runners never survive a boot, so a token
// from the previous one must be dead on arrival.
func TestTokenMinter_TokensDoNotSurviveAReboot(t *testing.T) {
	first, err := agenttools.NewTokenMinter()
	require.NoError(t, err)
	second, err := agenttools.NewTokenMinter()
	require.NoError(t, err)

	require.False(t, second.Verify("runner-a", first.Mint("runner-a")))
}

// The token travels in argv and in a JSON/TOML config value, so it must be safe
// to embed in both without escaping.
func TestTokenMinter_TokenIsURLSafeBase64(t *testing.T) {
	m, err := agenttools.NewTokenMinter()
	require.NoError(t, err)

	tok := m.Mint("runner-a")
	require.NotContains(t, tok, "+")
	require.NotContains(t, tok, "/")
	require.NotContains(t, tok, "=")
	require.NotContains(t, tok, `"`)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./internal/app/usecases/agenttools/... -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Write the implementation**

```go
// Package agenttools is the agent-facing capability surface: the tools an agent
// running inside a Crowbar chat may call, the authority model that decides what
// each caller can see, and the rendering of results back to a model.
//
// Everything here is reached through one seam — DispatchMCP — so authorization
// happens in exactly one place and the transport (an MCP relay process) never
// self-authorizes.
package agenttools

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// TokenMinter issues and verifies the per-runner token that authenticates an MCP
// call back into the daemon.
//
// The segment (runner) id alone is not an authenticator: the agent controls the
// process that holds it and can read its own argv, so an agent that learned a
// sibling's id could otherwise assume that sibling's scope. The token binds a
// caller to the runner it was minted for.
//
// The secret is per-DAEMON-BOOT and never persisted. That is sound because a
// runner's PTY is a child of the daemon: when the daemon dies every runner dies
// with it, so there is no live runner whose token could outlive the secret.
// Revocation is therefore automatic and no runner state has to be migrated.
type TokenMinter struct {
	secret []byte
}

func NewTokenMinter() (*TokenMinter, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("agenttools: mint token secret: %w", err)
	}
	return &TokenMinter{secret: secret}, nil
}

func (m *TokenMinter) Mint(runnerID string) string {
	return base64.RawURLEncoding.EncodeToString(m.sign(runnerID))
}

// Verify is constant-time in the comparison so a caller cannot probe the token
// byte by byte.
func (m *TokenMinter) Verify(runnerID, token string) bool {
	// An empty runner id names no runner, so it must never authenticate — without
	// this guard Mint("") and Verify("", …) agree with each other and an unset
	// --segment flag would silently pass the check.
	if runnerID == "" || token == "" {
		return false
	}
	got, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return false
	}
	return hmac.Equal(got, m.sign(runnerID))
}

func (m *TokenMinter) sign(runnerID string) []byte {
	mac := hmac.New(sha256.New, m.secret)
	mac.Write([]byte(runnerID))
	return mac.Sum(nil)
}
```

- [ ] **Step 4: Run tests**

Run: `cd api && go test ./internal/app/usecases/agenttools/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/usecases/agenttools/
git commit -m "feat(agenttools): per-boot HMAC runner token"
```

---

### Task 4: Scope resolver

**Files:**
- Create: `api/internal/app/usecases/agenttools/scope.go`
- Test: `api/internal/app/usecases/agenttools/scope_test.go`

**Interfaces:**
- Consumes: Task 3's `*TokenMinter`.
- Produces:
  - `type RunnerReader interface { Get(ctx context.Context, runnerID string) (domain.AgentRunner, error) }`
  - `type ChatReader interface { Get(ctx context.Context, chatID string) (domain.AgentChat, error); ListByWorkspace(ctx context.Context, wsID string) ([]domain.AgentChat, error) }`
  - `type WorkspaceLister interface { Get(ctx context.Context, wsID string) (domain.Workspace, error); List(ctx context.Context) ([]domain.Workspace, error) }`
  - `type Caller struct { RunnerID, ChatID string; Workspace domain.Workspace; Visible []domain.Workspace }`
  - `func NewResolver(minter *TokenMinter, runners RunnerReader, chats ChatReader, workspaces WorkspaceLister) *Resolver`
  - `func (r *Resolver) Resolve(ctx context.Context, runnerID, token string) (Caller, error)`
  - `func (c Caller) CanSee(wsID string) bool`
  - Sentinels `ErrUnauthorized`, `ErrOutOfScope`.

**Visibility rule (from the spec):** a git workspace sees itself and its `ParentID` descendants; the repo's `IsDefault` workspace sees every workspace in that repo; a `Kind == home` workspace sees every workspace in that project. Downward only, never upward.

- [ ] **Step 1: Write the failing test**

```go
package agenttools_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/agenttools"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type stubRunners struct{ r domain.AgentRunner; err error }

func (s stubRunners) Get(context.Context, string) (domain.AgentRunner, error) { return s.r, s.err }

type stubChats struct{ c domain.AgentChat; list []domain.AgentChat }

func (s stubChats) Get(context.Context, string) (domain.AgentChat, error) { return s.c, nil }
func (s stubChats) ListByWorkspace(context.Context, string) ([]domain.AgentChat, error) {
	return s.list, nil
}

type stubWorkspaces struct{ all []domain.Workspace }

func (s stubWorkspaces) Get(_ context.Context, id string) (domain.Workspace, error) {
	for _, w := range s.all {
		if w.ID == id {
			return w, nil
		}
	}
	return domain.Workspace{}, apperrNotFound()
}
func (s stubWorkspaces) List(context.Context) ([]domain.Workspace, error) { return s.all, nil }

// The tree used by every case below.
//   proj home (home)
//     repo-default (git, IsDefault)
//       ws-a
//         ws-a1
//       ws-b
//   other-repo-ws (a different repo, same project)
func tree() []domain.Workspace {
	return []domain.Workspace{
		{ID: "home", ProjectID: "P", Kind: domain.WorkspaceKindHome},
		{ID: "repo-default", ProjectID: "P", RepoID: "R", Kind: domain.WorkspaceKindGit, IsDefault: true},
		{ID: "ws-a", ProjectID: "P", RepoID: "R", Kind: domain.WorkspaceKindGit, ParentID: "repo-default"},
		{ID: "ws-a1", ProjectID: "P", RepoID: "R", Kind: domain.WorkspaceKindGit, ParentID: "ws-a"},
		{ID: "ws-b", ProjectID: "P", RepoID: "R", Kind: domain.WorkspaceKindGit, ParentID: "repo-default"},
		{ID: "other-repo-ws", ProjectID: "P", RepoID: "R2", Kind: domain.WorkspaceKindGit},
	}
}

func resolverOn(t *testing.T, callerWs string) (*agenttools.Resolver, *agenttools.TokenMinter) {
	t.Helper()
	m, err := agenttools.NewTokenMinter()
	require.NoError(t, err)
	return agenttools.NewResolver(m,
		stubRunners{r: domain.AgentRunner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: callerWs}},
		stubChats{c: domain.AgentChat{ID: "CHAT", WorkspaceID: callerWs}},
		stubWorkspaces{all: tree()},
	), m
}

func visibleIDs(c agenttools.Caller) []string {
	out := make([]string, 0, len(c.Visible))
	for _, w := range c.Visible {
		out = append(out, w.ID)
	}
	return out
}

func TestResolve_GitWorkspaceSeesItselfAndDescendants(t *testing.T) {
	r, m := resolverOn(t, "ws-a")
	c, err := r.Resolve(context.Background(), "RUN", m.Mint("RUN"))
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"ws-a", "ws-a1"}, visibleIDs(c))
}

func TestResolve_NeverSeesUpwards(t *testing.T) {
	r, m := resolverOn(t, "ws-a1")
	c, err := r.Resolve(context.Background(), "RUN", m.Mint("RUN"))
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"ws-a1"}, visibleIDs(c))
	require.False(t, c.CanSee("ws-a"))
	require.False(t, c.CanSee("repo-default"))
}

func TestResolve_RepoDefaultSeesWholeRepoButNotOtherRepos(t *testing.T) {
	r, m := resolverOn(t, "repo-default")
	c, err := r.Resolve(context.Background(), "RUN", m.Mint("RUN"))
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"repo-default", "ws-a", "ws-a1", "ws-b"}, visibleIDs(c))
	require.False(t, c.CanSee("other-repo-ws"))
	require.False(t, c.CanSee("home"))
}

func TestResolve_HomeWorkspaceSeesWholeProject(t *testing.T) {
	r, m := resolverOn(t, "home")
	c, err := r.Resolve(context.Background(), "RUN", m.Mint("RUN"))
	require.NoError(t, err)
	require.ElementsMatch(t,
		[]string{"home", "repo-default", "ws-a", "ws-a1", "ws-b", "other-repo-ws"},
		visibleIDs(c))
}

func TestResolve_RejectsBadToken(t *testing.T) {
	r, m := resolverOn(t, "ws-a")
	_, err := r.Resolve(context.Background(), "RUN", m.Mint("SOMEONE-ELSE"))
	require.ErrorIs(t, err, agenttools.ErrUnauthorized)
}

// A displaced runner has no current chat: it must not resolve to the chat it
// used to be on.
func TestResolve_RejectsDisplacedRunner(t *testing.T) {
	m, err := agenttools.NewTokenMinter()
	require.NoError(t, err)
	r := agenttools.NewResolver(m,
		stubRunners{r: domain.AgentRunner{ID: "RUN", CurrentChatID: "", WorkspaceID: "ws-a"}},
		stubChats{}, stubWorkspaces{all: tree()})

	_, err = r.Resolve(context.Background(), "RUN", m.Mint("RUN"))
	require.ErrorIs(t, err, agenttools.ErrUnauthorized)
}

// A cycle in ParentID must not hang the walk.
func TestResolve_ToleratesParentCycle(t *testing.T) {
	m, err := agenttools.NewTokenMinter()
	require.NoError(t, err)
	cyc := []domain.Workspace{
		{ID: "x", ProjectID: "P", RepoID: "R", Kind: domain.WorkspaceKindGit, ParentID: "y"},
		{ID: "y", ProjectID: "P", RepoID: "R", Kind: domain.WorkspaceKindGit, ParentID: "x"},
	}
	r := agenttools.NewResolver(m,
		stubRunners{r: domain.AgentRunner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "x"}},
		stubChats{c: domain.AgentChat{ID: "CHAT", WorkspaceID: "x"}},
		stubWorkspaces{all: cyc})

	c, err := r.Resolve(context.Background(), "RUN", m.Mint("RUN"))
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"x", "y"}, visibleIDs(c))
}
```

Add the helper the stub needs at the bottom of the test file:

```go
func apperrNotFound() error { return errNotFoundForTest }

var errNotFoundForTest = errors.New("not found")
```

(import `"errors"`).

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./internal/app/usecases/agenttools/... -run TestResolve -v`
Expected: FAIL — `agenttools.NewResolver` undefined.

- [ ] **Step 3: Write the implementation**

```go
package agenttools

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
)

var (
	// ErrUnauthorized means the caller could not be established at all: a bad
	// token, a dead runner, or a runner with no current chat.
	ErrUnauthorized = errors.New("agenttools: unauthorized")
	// ErrOutOfScope means the caller is real but asked about something outside
	// what its position in the workspace tree permits.
	ErrOutOfScope = errors.New("agenttools: out of scope")
)

type RunnerReader interface {
	Get(ctx context.Context, runnerID string) (domain.AgentRunner, error)
}

type ChatReader interface {
	Get(ctx context.Context, chatID string) (domain.AgentChat, error)
	ListByWorkspace(ctx context.Context, wsID string) ([]domain.AgentChat, error)
}

type WorkspaceLister interface {
	Get(ctx context.Context, wsID string) (domain.Workspace, error)
	List(ctx context.Context) ([]domain.Workspace, error)
}

// Caller is an authenticated agent and everything it is allowed to reach.
// Visible always contains the caller's own workspace.
type Caller struct {
	RunnerID  string
	ChatID    string
	Workspace domain.Workspace
	Visible   []domain.Workspace
}

func (c Caller) CanSee(wsID string) bool {
	for _, w := range c.Visible {
		if w.ID == wsID {
			return true
		}
	}
	return false
}

type Resolver struct {
	minter     *TokenMinter
	runners    RunnerReader
	chats      ChatReader
	workspaces WorkspaceLister
}

func NewResolver(
	minter *TokenMinter,
	runners RunnerReader,
	chats ChatReader,
	workspaces WorkspaceLister,
) *Resolver {
	return &Resolver{minter: minter, runners: runners, chats: chats, workspaces: workspaces}
}

// Resolve authenticates (runnerID, token) and computes what that caller may see.
//
// It resolves through the runner's CURRENT chat rather than a chat id baked in
// at spawn: an agent that clears its conversation moves to a different chat while
// the runner id stays stable, so anything keyed on a baked chat id would act on
// the chat the agent used to be on. This is the same property RenameByRunner
// relies on.
func (r *Resolver) Resolve(ctx context.Context, runnerID, token string) (Caller, error) {
	if runnerID == "" || !r.minter.Verify(runnerID, token) {
		return Caller{}, ErrUnauthorized
	}
	runner, err := r.runners.Get(ctx, runnerID)
	if err != nil {
		// A runner the store cannot produce is a dead runner: its PTY is gone, so
		// there is nothing to authorize.
		return Caller{}, fmt.Errorf("%w: runner: %w", ErrUnauthorized, err)
	}
	if runner.CurrentChatID == "" {
		return Caller{}, fmt.Errorf("%w: runner has been displaced", ErrUnauthorized)
	}
	ws, err := r.workspaces.Get(ctx, runner.WorkspaceID)
	if err != nil {
		return Caller{}, fmt.Errorf("%w: workspace: %w", ErrUnauthorized, err)
	}
	all, err := r.workspaces.List(ctx)
	if err != nil {
		return Caller{}, fmt.Errorf("agenttools: resolve: list workspaces: %w", err)
	}
	return Caller{
		RunnerID:  runnerID,
		ChatID:    runner.CurrentChatID,
		Workspace: ws,
		Visible:   visibleFrom(ws, all),
	}, nil
}

// visibleFrom applies the three-tier rule. It is downward only by construction:
// nothing here ever consults a workspace's ancestors.
func visibleFrom(caller domain.Workspace, all []domain.Workspace) []domain.Workspace {
	switch {
	case caller.Kind == domain.WorkspaceKindHome:
		return filter(all, func(w domain.Workspace) bool { return w.ProjectID == caller.ProjectID })
	case caller.IsDefault:
		return filter(all, func(w domain.Workspace) bool {
			return w.ProjectID == caller.ProjectID && w.RepoID == caller.RepoID
		})
	default:
		return descendants(caller, all)
	}
}

// descendants walks the ParentID tree downward from caller. seen guards against
// a cycle in the parent chain, which would otherwise spin forever.
func descendants(caller domain.Workspace, all []domain.Workspace) []domain.Workspace {
	byParent := map[string][]domain.Workspace{}
	for _, w := range all {
		byParent[w.ParentID] = append(byParent[w.ParentID], w)
	}
	seen := map[string]bool{caller.ID: true}
	out := []domain.Workspace{caller}
	queue := []domain.Workspace{caller}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, child := range byParent[cur.ID] {
			if seen[child.ID] {
				continue
			}
			seen[child.ID] = true
			out = append(out, child)
			queue = append(queue, child)
		}
	}
	return out
}

func filter(all []domain.Workspace, keep func(domain.Workspace) bool) []domain.Workspace {
	out := make([]domain.Workspace, 0, len(all))
	for _, w := range all {
		if keep(w) {
			out = append(out, w)
		}
	}
	return out
}
```

- [ ] **Step 4: Run tests**

Run: `cd api && go test ./internal/app/usecases/agenttools/... -v`
Expected: PASS. If `TestResolve_ToleratesParentCycle` returns only `{"x"}`, the walk is not seeding from the caller's children correctly — fix `descendants`, do not change the test.

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/usecases/agenttools/
git commit -m "feat(agenttools): downward-only scope resolver over the workspace tree"
```

---

### Task 5: ToolSet with `set_chat_title`

**Files:**
- Create: `api/internal/app/usecases/agenttools/toolset.go`, `api/internal/app/usecases/agenttools/tools_chat.go`
- Test: `api/internal/app/usecases/agenttools/toolset_test.go`

**Interfaces:**
- Consumes: Task 2's `mcp.Tool`/`mcp.ToolSet`, Task 4's `*Resolver`/`Caller`.
- Produces:
  - `type ChatRenamer interface { RenameByRunner(ctx context.Context, runnerID, title, source string) error }`
  - `type Deps struct { Resolver *Resolver; Chats ChatRenamer }`
  - `type ToolSet struct{ ... }` implementing `mcp.ToolSet`, built by `func NewToolSet(deps Deps, runnerID, token string) *ToolSet`
  - `type toolDef struct { name, description string; schema json.RawMessage; run func(ctx context.Context, c Caller, args json.RawMessage) (string, error) }`

**Note:** the ToolSet is constructed **per request** and closes over `(runnerID, token)`, so no tool handler can ever be reached without a successful `Resolve`.

- [ ] **Step 1: Write the failing test**

```go
package agenttools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/agenttools"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type spyRenamer struct {
	runnerID, title, source string
	calls                   int
}

func (s *spyRenamer) RenameByRunner(_ context.Context, runnerID, title, source string) error {
	s.calls++
	s.runnerID, s.title, s.source = runnerID, title, source
	return nil
}

func toolsetOn(t *testing.T, renamer agenttools.ChatRenamer) (*agenttools.ToolSet, string) {
	t.Helper()
	m, err := agenttools.NewTokenMinter()
	require.NoError(t, err)
	res := agenttools.NewResolver(m,
		stubRunners{r: domain.AgentRunner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
		stubChats{c: domain.AgentChat{ID: "CHAT", WorkspaceID: "ws-a"}},
		stubWorkspaces{all: tree()})
	tok := m.Mint("RUN")
	return agenttools.NewToolSet(agenttools.Deps{Resolver: res, Chats: renamer}, "RUN", tok), tok
}

func TestToolSet_AdvertisesSetChatTitle(t *testing.T) {
	ts, _ := toolsetOn(t, &spyRenamer{})
	names := []string{}
	for _, tool := range ts.Tools() {
		names = append(names, tool.Name)
		require.NotEmpty(t, tool.Description, "%s has no description — it is the whole trigger budget", tool.Name)
		require.NotEmpty(t, tool.InputSchema)
	}
	require.Contains(t, names, "set_chat_title")
}

// Global constraint: codex does not defer tool schemas, so every tool costs
// context on every codex turn.
func TestToolSet_RespectsToolCeiling(t *testing.T) {
	ts, _ := toolsetOn(t, &spyRenamer{})
	require.LessOrEqual(t, len(ts.Tools()), 8)
}

// No tool may take a scope argument — authority comes from the runner, never
// from something the model can type.
func TestToolSet_NoToolAcceptsAScopeArgument(t *testing.T) {
	ts, _ := toolsetOn(t, &spyRenamer{})
	forbidden := []string{"workspaceId", "workspace_id", "projectId", "project_id", "repoId", "repo_id", "runnerId", "segment"}
	for _, tool := range ts.Tools() {
		for _, f := range forbidden {
			require.NotContains(t, string(tool.InputSchema), f,
				"tool %s exposes %s; scope must never be an argument", tool.Name, f)
		}
	}
}

func TestToolSet_SetChatTitleRenamesTheCallersRunner(t *testing.T) {
	spy := &spyRenamer{}
	ts, _ := toolsetOn(t, spy)

	out, err := ts.Call(context.Background(), "set_chat_title", json.RawMessage(`{"title":"Refactor auth"}`))
	require.NoError(t, err)
	require.Contains(t, out, "Refactor auth")

	require.Equal(t, 1, spy.calls)
	require.Equal(t, "RUN", spy.runnerID)
	require.Equal(t, "Refactor auth", spy.title)
	// source=agent so a user-locked title is never clobbered.
	require.Equal(t, "agent", spy.source)
}

func TestToolSet_SetChatTitleRejectsEmptyTitle(t *testing.T) {
	ts, _ := toolsetOn(t, &spyRenamer{})
	_, err := ts.Call(context.Background(), "set_chat_title", json.RawMessage(`{"title":"   "}`))
	require.Error(t, err)
}

func TestToolSet_UnknownToolErrors(t *testing.T) {
	ts, _ := toolsetOn(t, &spyRenamer{})
	_, err := ts.Call(context.Background(), "rm_rf", json.RawMessage(`{}`))
	require.Error(t, err)
}

func TestToolSet_BadTokenCannotReachAnyTool(t *testing.T) {
	spy := &spyRenamer{}
	m, err := agenttools.NewTokenMinter()
	require.NoError(t, err)
	res := agenttools.NewResolver(m,
		stubRunners{r: domain.AgentRunner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
		stubChats{}, stubWorkspaces{all: tree()})
	ts := agenttools.NewToolSet(agenttools.Deps{Resolver: res, Chats: spy}, "RUN", "forged")

	_, err = ts.Call(context.Background(), "set_chat_title", json.RawMessage(`{"title":"x"}`))
	require.ErrorIs(t, err, agenttools.ErrUnauthorized)
	require.Zero(t, spy.calls, "an unauthorized call must never reach a tool handler")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./internal/app/usecases/agenttools/... -run TestToolSet -v`
Expected: FAIL — `agenttools.NewToolSet` undefined.

- [ ] **Step 3: Write `toolset.go`**

```go
package agenttools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/engine/mcp"
)

// Deps is everything the tool surface needs from the rest of the app. Later
// phases extend this struct; a nil dependency simply means the tools that need
// it are not registered.
type Deps struct {
	Resolver *Resolver
	Chats    ChatRenamer
}

type toolDef struct {
	name        string
	description string
	schema      json.RawMessage
	run         func(ctx context.Context, c Caller, args json.RawMessage) (string, error)
}

// ToolSet is built PER REQUEST around one caller's credentials, which is what
// makes it impossible to reach a tool handler without a successful Resolve —
// there is no code path that calls run() with an unauthenticated Caller.
type ToolSet struct {
	deps     Deps
	runnerID string
	token    string
	defs     []toolDef
}

func NewToolSet(deps Deps, runnerID, token string) *ToolSet {
	ts := &ToolSet{deps: deps, runnerID: runnerID, token: token}
	ts.defs = append(ts.defs, chatTools(deps)...)
	return ts
}

func (t *ToolSet) Tools() []mcp.Tool {
	out := make([]mcp.Tool, 0, len(t.defs))
	for _, d := range t.defs {
		out = append(out, mcp.Tool{Name: d.name, Description: d.description, InputSchema: d.schema})
	}
	return out
}

func (t *ToolSet) Call(ctx context.Context, name string, args json.RawMessage) (string, error) {
	caller, err := t.deps.Resolver.Resolve(ctx, t.runnerID, t.token)
	if err != nil {
		return "", err
	}
	for _, d := range t.defs {
		if d.name != name {
			continue
		}
		if len(args) == 0 {
			args = json.RawMessage(`{}`)
		}
		return d.run(ctx, caller, args)
	}
	return "", fmt.Errorf("agenttools: unknown tool %q", name)
}

// decode unmarshals a tool's arguments, turning a decode failure into a message
// the model can act on rather than a bare syntax error.
func decode(args json.RawMessage, into any) error {
	if err := json.Unmarshal(args, into); err != nil {
		return fmt.Errorf("agenttools: arguments are not valid for this tool: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Write `tools_chat.go`**

```go
package agenttools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type ChatRenamer interface {
	RenameByRunner(ctx context.Context, runnerID, title, source string) error
}

func chatTools(deps Deps) []toolDef {
	if deps.Chats == nil {
		return nil
	}
	return []toolDef{{
		name:        "set_chat_title",
		description: "Set this chat's title. Call once, early, with a concise 2-5 word Title-Case summary of the task.",
		schema: json.RawMessage(`{
			"type":"object",
			"properties":{"title":{"type":"string","description":"Concise 2-5 word Title-Case title."}},
			"required":["title"],
			"additionalProperties":false
		}`),
		run: func(ctx context.Context, c Caller, args json.RawMessage) (string, error) {
			var in struct {
				Title string `json:"title"`
			}
			if err := decode(args, &in); err != nil {
				return "", err
			}
			title := strings.TrimSpace(in.Title)
			if title == "" {
				return "", fmt.Errorf("agenttools: set_chat_title: title must not be empty")
			}
			// Renaming by RUNNER, not by chat id: the runner resolves to whatever
			// chat it is on right now, so an agent that cleared its conversation
			// titles the chat it is actually in.
			//
			// source="agent" gives agent precedence: it upgrades a derived title
			// and never clobbers one the user locked.
			if err := deps.Chats.RenameByRunner(ctx, c.RunnerID, title, "agent"); err != nil {
				return "", fmt.Errorf("agenttools: set_chat_title: %w", err)
			}
			return "Chat titled: " + title, nil
		},
	}}
}
```

- [ ] **Step 5: Run tests**

Run: `cd api && go test ./internal/app/usecases/agenttools/... -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add api/internal/app/usecases/agenttools/
git commit -m "feat(agenttools): tool registry and set_chat_title"
```

---

### Task 6: `DispatchMCP` usecase method and HTTP route

**Files:**
- Create: `api/internal/api/v0/endpoints/agent/handlers/mcp.go`
- Modify: `api/internal/app/usecases/agent/agent.go`, `api/internal/api/v0/endpoints/agent/handlers/handlers.go`, `api/internal/api/v0/endpoints/agent/routes.go`, `api/internal/api/v0/endpoints/home/routes.go`, `api/internal/api/v0/route_audit_test.go`
- Modify (test stubs): `api/internal/api/v0/endpoints/agent/routes_test.go:117`, `api/internal/api/v0/endpoints/agent/handlers/chats_test.go:213`, `api/internal/api/v0/endpoints/agent/handlers/hooks_test.go:280`
- Test: `api/internal/api/v0/endpoints/agent/handlers/mcp_test.go`

**Interfaces:**
- Consumes: Task 2 `mcp.NewServer`, Task 5 `agenttools.NewToolSet`.
- Produces: `DispatchMCP(ctx context.Context, runnerID, token string, message []byte) ([]byte, bool, error)` on the agent usecase and on the `AgentUsecase` port. Route: `POST .../agent/runners/:segid/mcp`, body `{"token": string, "rpc": <raw JSON-RPC>}`, response `{"success":true,"data":{"rpc": <raw>}}` or `204 No Content` for a notification.

**The 6-point route checklist** (all enforced by existing tests — skipping any one fails the build):
1. `endpoints/agent/routes.go` — mount on `wsScoped`.
2. `endpoints/home/routes.go` — mount the identical route with `h.RequireHomeWorkspace` first (`TestHomeMountsEveryAgentRoute`).
3. `route_audit_test.go` — declare the route string in the agent block.
4. Handler in `endpoints/agent/handlers/`. Runner-keyed handlers do **not** call `requireChatInWorkspace`.
5. Method on the `AgentUsecase` interface.
6. Update the three test stubs listed above.

- [ ] **Step 1: Write the failing handler test**

```go
package handlers_test

// Add to the existing handlers_test package. The stub AgentUsecase in this
// package must gain DispatchMCP; see Step 5.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMCP_ReturnsRPCResponseInEnvelope(t *testing.T) {
	// stubUsecase.dispatchMCPOut is returned verbatim by DispatchMCP.
	r, stub := newAgentRouter(t)
	stub.dispatchMCPOut = []byte(`{"jsonrpc":"2.0","id":1,"result":{}}`)
	stub.dispatchMCPSend = true

	body := `{"token":"TOK","rpc":{"jsonrpc":"2.0","id":1,"method":"ping"}}`
	req := httptest.NewRequest(http.MethodPost, "/v0/projects/P/repos/R/workspaces/W/agent/runners/SEG/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "SEG", stub.dispatchMCPRunner)
	require.Equal(t, "TOK", stub.dispatchMCPToken)
	require.JSONEq(t, `{"jsonrpc":"2.0","id":1,"method":"ping"}`, string(stub.dispatchMCPMessage))

	var env struct {
		Success bool `json:"success"`
		Data    struct {
			RPC json.RawMessage `json:"rpc"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	require.True(t, env.Success)
	require.JSONEq(t, `{"jsonrpc":"2.0","id":1,"result":{}}`, string(env.Data.RPC))
}

func TestMCP_NotificationGets204(t *testing.T) {
	r, stub := newAgentRouter(t)
	stub.dispatchMCPSend = false

	body := `{"token":"TOK","rpc":{"jsonrpc":"2.0","method":"notifications/initialized"}}`
	req := httptest.NewRequest(http.MethodPost, "/v0/projects/P/repos/R/workspaces/W/agent/runners/SEG/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Empty(t, rec.Body.String())
}

func TestMCP_MissingRPCIs400(t *testing.T) {
	r, _ := newAgentRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/v0/projects/P/repos/R/workspaces/W/agent/runners/SEG/mcp", strings.NewReader(`{"token":"TOK"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}
```

If `newAgentRouter` does not already exist in this package, mirror the router construction used by the neighbouring `chats_test.go` and return the stub alongside it.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./internal/api/v0/endpoints/agent/... -run TestMCP -v`
Expected: FAIL — route not registered / stub field undefined.

- [ ] **Step 3: Add `DispatchMCP` to the agent usecase**

In `api/internal/app/usecases/agent/agent.go`, add the field to `Usecase` and wire it in `New`:

```go
// In the Usecase struct, after `turns *turnWaits`:
	// tools is the agent-facing capability surface. It is nil until the container
	// wires it, so a daemon built without it simply advertises no tools rather
	// than failing to start.
	tools *agenttools.Deps
	minter *agenttools.TokenMinter
```

Add the method (place it near `RenameByRunner`, which it mirrors):

```go
// DispatchMCP runs one MCP message on behalf of the runner named by runnerID.
//
// It is the ONLY entry point to the agent tool surface, which is what keeps
// authorization in one place: the relay process that carries these bytes never
// decides anything, and the ToolSet is constructed around this caller's
// credentials so no tool handler is reachable without a successful Resolve.
//
// The bool reports whether a reply should be sent: a JSON-RPC notification is
// answered with silence.
func (u *Usecase) DispatchMCP(
	ctx context.Context,
	runnerID, token string,
	message []byte,
) ([]byte, bool, error) {
	if u.tools == nil || u.minter == nil {
		return nil, false, fmt.Errorf("agent: dispatch mcp: tool surface not configured")
	}
	ts := agenttools.NewToolSet(*u.tools, runnerID, token)
	srv := enginemcp.NewServer("crowbar", metadata.GetVersion(), ts)
	out, send := srv.Handle(ctx, message)
	return out, send, nil
}
```

Imports to add: `agenttools "github.com/char2cs/crowbar/api/internal/app/usecases/agenttools"`, `enginemcp "github.com/char2cs/crowbar/api/internal/engine/mcp"`, `"github.com/char2cs/crowbar/api/internal/core/metadata"`.

Extend `New(...)` with two trailing parameters `minter *agenttools.TokenMinter, tools *agenttools.Deps` and assign them. Update every call site of `agent.New` (find with `rg 'agentusecase\.New\(|agent\.New\('` under `api/`) to pass the new arguments; in the app container construct the minter once with `agenttools.NewTokenMinter()` and fail daemon start if it errors.

- [ ] **Step 4: Write the handler**

Create `api/internal/api/v0/endpoints/agent/handlers/mcp.go`:

```go
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/libs"
)

// MCP carries one JSON-RPC message from a vendor CLI's MCP client to the
// daemon's tool surface and the reply back.
//
// It is runner-keyed like RenameByRunner and so does NOT call
// requireChatInWorkspace: the caller is authenticated by (segid, token) and its
// scope is derived from the runner, never from the URL. The path ids exist only
// because every in-PTY callback builds a workspace-nested agent URL.
func (h *Handlers) MCP(ctx *gin.Context) {
	var body struct {
		Token string          `json:"token"`
		RPC   json.RawMessage `json:"rpc"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil || len(body.RPC) == 0 {
		libs.WriteBadRequest(ctx, "body must carry an rpc message")
		return
	}

	out, send, err := h.usecase.DispatchMCP(ctx.Request.Context(),
		ctx.Param("segid"), body.Token, body.RPC)
	if err != nil {
		libs.WriteInternalError(ctx, err)
		return
	}
	if !send {
		ctx.Status(http.StatusNoContent)
		return
	}
	libs.WriteQueryOK(ctx, gin.H{"rpc": json.RawMessage(out)})
}
```

Use whatever the neighbouring handlers actually call for 400/500 responses — check `chats.go` and match it exactly rather than inventing `libs.WriteBadRequest`/`WriteInternalError` if those names differ.

- [ ] **Step 5: Complete the 6-point checklist**

1. `endpoints/agent/routes.go`, beside line 53:
   ```go
   wsScoped.POST("/agent/runners/:segid/mcp", h.MCP)
   ```
2. `endpoints/home/routes.go`, beside line 105:
   ```go
   home.POST("/agent/runners/:segid/mcp", h.RequireHomeWorkspace, ah.MCP)
   ```
3. `route_audit_test.go`, in the agent block near line 228:
   ```go
   "POST " + ws + "/agent/runners/:segid/mcp",
   ```
   and the matching home entry wherever the home routes are declared in that file.
4. Handler — done in Step 4.
5. `endpoints/agent/handlers/handlers.go`, add to `AgentUsecase`:
   ```go
   // DispatchMCP runs one MCP JSON-RPC message for the runner named by runnerID,
   // authenticated by token. The bool reports whether a reply should be sent.
   DispatchMCP(ctx context.Context, runnerID string, token string, message []byte) ([]byte, bool, error)
   ```
6. Add the method to the three stubs at `endpoints/agent/routes_test.go:117`, `handlers/chats_test.go:213`, `handlers/hooks_test.go:280`:
   ```go
   func (s *stubUsecase) DispatchMCP(_ context.Context, runnerID, token string, message []byte) ([]byte, bool, error) {
   	s.dispatchMCPRunner, s.dispatchMCPToken, s.dispatchMCPMessage = runnerID, token, message
   	return s.dispatchMCPOut, s.dispatchMCPSend, s.dispatchMCPErr
   }
   ```
   with the corresponding fields on each stub struct (only the stub used by `mcp_test.go` needs the recording fields; the others may return zero values).

- [ ] **Step 6: Run the full API test suite**

Run: `cd api && go test ./internal/api/... ./internal/app/... ./internal/engine/mcp/... 2>&1 | tail -30`
Expected: PASS, including `TestHomeMountsEveryAgentRoute` and the route audit.

- [ ] **Step 7: Commit**

```bash
git add api/
git commit -m "feat(agent): DispatchMCP usecase seam and runner-keyed MCP route"
```

---

### Task 7: The `crowbar mcp` stdio relay

**Files:**
- Create: `api/cmd/crowbar/mcp.go`
- Modify: `api/cmd/crowbar/main.go:23` (add `newMCPCmd()` to `root.AddCommand`)
- Test: `api/cmd/crowbar/mcp_test.go`

**Interfaces:**
- Consumes: `scopedAgentPath`, `bindScopeFlags` (`api/cmd/crowbar/scope.go`), `ipc.NewClient` (`api/internal/core/ipc/client.go:22`).
- Produces: `func newMCPCmd() *cobra.Command` and `func runMCPRelay(in io.Reader, out io.Writer, post func(path string, body any) ([]byte, error), segment, project, repo, workspace, token string) error`.

**Why a relay:** the protocol handler lives in the daemon so there is only one API surface over the usecases and no way for a stale `crowbar` binary to disagree with the daemon about tool behaviour. This process shovels bytes and has nothing to be stale about.

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type postCall struct {
	path string
	body map[string]any
}

func recorder(responses ...string) (*[]postCall, func(string, any) ([]byte, error)) {
	calls := &[]postCall{}
	i := 0
	return calls, func(path string, body any) ([]byte, error) {
		raw, _ := json.Marshal(body)
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		*calls = append(*calls, postCall{path: path, body: m})
		if i >= len(responses) {
			return []byte(`{"success":true,"data":{}}`), nil
		}
		r := responses[i]
		i++
		return []byte(r), nil
	}
}

func TestRelay_ForwardsEachLineAndWritesTheReply(t *testing.T) {
	calls, post := recorder(
		`{"success":true,"data":{"rpc":{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-11-25"}}}}`,
	)
	var out bytes.Buffer
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n")

	require.NoError(t, runMCPRelay(in, &out, post, "SEG", "P", "R", "W", "TOK"))

	require.Len(t, *calls, 1)
	require.Equal(t, "/v0/projects/P/repos/R/workspaces/W/agent/runners/SEG/mcp", (*calls)[0].path)
	require.Equal(t, "TOK", (*calls)[0].body["token"])

	// Exactly one line out, and it is the unwrapped rpc object.
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	require.Len(t, lines, 1)
	require.JSONEq(t, `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-11-25"}}`, lines[0])
}

// A 204 comes back as an envelope with no rpc field: the relay must stay silent,
// because replying to a notification confuses the client.
func TestRelay_WritesNothingForANotification(t *testing.T) {
	_, post := recorder(``)
	var out bytes.Buffer
	in := strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n")

	require.NoError(t, runMCPRelay(in, &out, post, "SEG", "P", "R", "W", "TOK"))
	require.Empty(t, out.String())
}

func TestRelay_SkipsBlankLines(t *testing.T) {
	calls, post := recorder()
	var out bytes.Buffer
	in := strings.NewReader("\n   \n" + `{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n")

	require.NoError(t, runMCPRelay(in, &out, post, "SEG", "P", "R", "W", "TOK"))
	require.Len(t, *calls, 1)
}

// A project-home workspace has no repo id; the path must fall to the home mount
// or every call 404s.
func TestRelay_UsesHomePathWhenRepoIsEmpty(t *testing.T) {
	calls, post := recorder()
	var out bytes.Buffer
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n")

	require.NoError(t, runMCPRelay(in, &out, post, "SEG", "P", "", "W", "TOK"))
	require.Equal(t, "/v0/projects/P/home/agent/runners/SEG/mcp", (*calls)[0].path)
}

// A daemon that is down must not kill the CLI session: the relay reports the
// failure as a JSON-RPC error and keeps going.
func TestRelay_TransportFailureBecomesAnRPCError(t *testing.T) {
	var out bytes.Buffer
	post := func(string, any) ([]byte, error) { return nil, errBoom }
	in := strings.NewReader(`{"jsonrpc":"2.0","id":9,"method":"tools/list"}` + "\n")

	require.NoError(t, runMCPRelay(in, &out, post, "SEG", "P", "R", "W", "TOK"))

	var resp struct {
		ID    int `json:"id"`
		Error struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out.String())), &resp))
	require.Equal(t, 9, resp.ID)
	require.Equal(t, -32603, resp.Error.Code)
}

var errBoom = errors.New("daemon down")
```

(import `"errors"`).

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./cmd/crowbar/... -run TestRelay -v`
Expected: FAIL — `runMCPRelay` undefined.

- [ ] **Step 3: Write the implementation**

```go
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/char2cs/crowbar/api/internal/core/ipc"
)

func newMCPCmd() *cobra.Command {
	var project, repo, workspace, segment, token string
	cmd := &cobra.Command{
		Use:    "mcp",
		Short:  "Relay MCP stdio traffic to the Crowbar daemon",
		Hidden: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			client, err := ipc.NewClient("unix://")
			if err != nil {
				return err
			}
			post := func(path string, body any) (int, []byte, error) {
				return client.PostJSON(context.Background(), path, body)
			}
			return runMCPRelay(os.Stdin, os.Stdout, post, segment, project, repo, workspace, token)
		},
	}
	cmd.Flags().StringVar(&segment, "segment", "", "Crowbar segment id")
	cmd.Flags().StringVar(&token, "token", "", "runner token minted at spawn")
	bindScopeFlags(cmd, &project, &repo, &workspace)
	return cmd
}

// runMCPRelay is the whole relay: read one JSON-RPC message per line, hand it to
// the daemon, write back whatever the daemon says to write.
//
// It deliberately understands NOTHING about MCP. Every protocol decision — which
// methods exist, what a tool does, whether a message deserves a reply — is made
// in the daemon, so a stale crowbar binary cannot disagree with the daemon it is
// talking to.
func runMCPRelay(
	in io.Reader,
	out io.Writer,
	post func(path string, body any) ([]byte, error),
	segment, project, repo, workspace, token string,
) error {
	path := scopedAgentPath(project, repo, workspace, "/runners/"+segment+"/mcp")

	scanner := bufio.NewScanner(in)
	// MCP messages carry tool results and file patches; the 64 KiB default would
	// truncate them into parse errors.
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	writer := bufio.NewWriter(out)
	defer writer.Flush()

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		status, raw, err := post(path, map[string]any{"token": token, "rpc": json.RawMessage(line)})
		if err != nil {
			// The session must survive a daemon blip: report it as a JSON-RPC error
			// against the id the client sent, and keep serving.
			writeLine(writer, transportError(line, err))
			continue
		}
		// A non-2xx is an APPLICATION error (bad request, tool surface not
		// configured, a stale binary hitting a route that 404s). PostJSON returns
		// err == nil for those, and the body carries no rpc — which is
		// indistinguishable from the 204 silence below. Answering it with silence
		// hangs the client forever on a request it is entitled to a reply for, so
		// it must become a JSON-RPC error like any other failure.
		if status < 200 || status > 299 {
			writeLine(writer, transportError(line, fmt.Errorf("daemon returned HTTP %d: %s", status, raw)))
			continue
		}

		var env struct {
			Data struct {
				RPC json.RawMessage `json:"rpc"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &env); err != nil || len(env.Data.RPC) == 0 {
			// A 2xx with no rpc is the daemon's 204: it was a notification, and
			// JSON-RPC says we stay silent.
			continue
		}
		writeLine(writer, env.Data.RPC)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("crowbar mcp: read stdin: %w", err)
	}
	return nil
}

func writeLine(w *bufio.Writer, payload []byte) {
	w.Write(payload)
	w.WriteByte('\n')
	// Flushed per message: an MCP client blocks waiting for this reply, so a
	// buffered response is a hang.
	w.Flush()
}

// transportError echoes the request's id so the client can match the failure to
// its call. A message we cannot even parse gets a null id, which is legal.
func transportError(line string, cause error) []byte {
	var req struct {
		ID json.RawMessage `json:"id"`
	}
	_ = json.Unmarshal([]byte(line), &req)
	id := req.ID
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	return []byte(fmt.Sprintf(
		`{"jsonrpc":"2.0","id":%s,"error":{"code":-32603,"message":%q}}`,
		id, "crowbar daemon unreachable: "+cause.Error()))
}
```

- [ ] **Step 4: Register the command**

In `api/cmd/crowbar/main.go:23`, change:

```go
root.AddCommand(newServeCmd(), newVersionCmd(), newHookCmd(), newHandoffCmd(), newChatCmd(), newMCPCmd())
```

- [ ] **Step 5: Run tests**

Run: `cd api && go test ./cmd/crowbar/... -v`
Expected: PASS, including the existing `scope_roundtrip_test.go`.

- [ ] **Step 6: Commit**

```bash
git add api/cmd/crowbar/
git commit -m "feat(cli): crowbar mcp stdio relay"
```

---

### Task 8: Descriptor injection and the capability preamble

**Files:**
- Modify: `api/internal/engine/agent/template.go`, `api/internal/app/usecases/agent/agent.go:733-761`, `api/internal/engine/agent/descriptors/claude.yaml`, `api/internal/engine/agent/descriptors/codex.yaml`, `api/internal/core/config/default.yaml`, `api/internal/core/config/config.go`
- Test: `api/internal/engine/agent/template_test.go`, `api/internal/engine/agent/descriptor_test.go`

**Interfaces:**
- Consumes: Task 3's `*TokenMinter` (already on the usecase from Task 6).
- Produces: `TemplateCtx.RunnerToken` field and the `{runner_token}` expansion token; `config.GetPrompts().CapabilitiesInstruction`.

- [ ] **Step 1: Write the failing tests**

Add to `api/internal/engine/agent/template_test.go`:

```go
func TestExpand_RunnerToken(t *testing.T) {
	got := agent.Expand("tok={runner_token}", agent.TemplateCtx{RunnerToken: "abc123"})
	require.Equal(t, "tok=abc123", got)
}
```

Add to `api/internal/engine/agent/descriptor_test.go`:

```go
// Both descriptors must register the crowbar MCP server, and must do it through
// a channel that writes nothing into the user's home or repo.
func TestDescriptors_RegisterTheCrowbarMCPServer(t *testing.T) {
	for _, id := range []string{"claude", "codex"} {
		d := loadEmbeddedDescriptor(t, id)

		plan, err := agent.BuildSpawnPlan(d, agent.TemplateCtx{
			Tmp: t.TempDir(), Cwd: t.TempDir(),
			CrowbarHook: "/usr/local/bin/crowbar",
			Segid:       "SEG", RunnerToken: "TOK", Provider: id,
			ProjectID: "P", RepoID: "R", WorkspaceID: "W",
		}, nil, nil)
		require.NoError(t, err)

		joined := strings.Join(plan.Argv, " ")
		require.Contains(t, joined, "mcp", "%s does not register an MCP server", id)
		require.Contains(t, joined, "SEG")
		require.Contains(t, joined, "TOK")
	}
}

// The empty-repo case is why scope travels as discrete array elements rather
// than through {scope_flags}: a project-home workspace has no repo id.
func TestDescriptors_MCPArgsSurviveAnEmptyRepoID(t *testing.T) {
	for _, id := range []string{"claude", "codex"} {
		d := loadEmbeddedDescriptor(t, id)
		plan, err := agent.BuildSpawnPlan(d, agent.TemplateCtx{
			Tmp: t.TempDir(), Cwd: t.TempDir(),
			CrowbarHook: "/usr/local/bin/crowbar",
			Segid:       "SEG", RunnerToken: "TOK", Provider: id,
			ProjectID: "P", RepoID: "", WorkspaceID: "W",
		}, nil, nil)
		require.NoError(t, err)
		require.NotContains(t, strings.Join(plan.Argv, " "), "{repo_id}")
	}
}
```

Use whatever helper the existing descriptor tests use to load an embedded descriptor; if none exists, read it via the embed FS the way `descriptors_embed.go` exposes it.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd api && go test ./internal/engine/agent/... -run 'TestExpand_RunnerToken|TestDescriptors_' -v`
Expected: FAIL — unknown field `RunnerToken`.

- [ ] **Step 3: Add the template variable**

In `api/internal/engine/agent/template.go`, add to `TemplateCtx` after `Segid`:

```go
	// RunnerToken authenticates this runner's MCP calls back into the daemon. The
	// segment id alone cannot: the agent controls the process holding it and can
	// read its own argv, so an agent that learned a sibling's segment could
	// otherwise assume that sibling's scope. Minted per daemon boot; runners never
	// outlive a boot, so it needs no persistence and revokes itself.
	RunnerToken string
```

and to `Expand`'s replacer:

```go
		"{runner_token}", ctx.RunnerToken,
```

- [ ] **Step 4: Mint the token at spawn**

In `api/internal/app/usecases/agent/agent.go`, inside `spawnRunner`, in the `TemplateCtx` literal at line ~733, add:

```go
		RunnerToken: u.minter.Mint(runnerID),
```

- [ ] **Step 5: Add the injection to both descriptors**

Append to `config_injection` in `descriptors/claude.yaml`:

```yaml
  # The Crowbar tool surface. --mcp-config takes a JSON STRING, so nothing is
  # written to disk and the user's own MCP servers are untouched (verified
  # against claude 2.1.220: the tools arrive as mcp__crowbar__*, and claude
  # DEFERS their schemas until first use).
  #
  # Scope travels as discrete array elements, never {scope_flags}: each element
  # reaches the process as its own argument, so a project-home workspace's empty
  # repo id arrives as an empty string instead of swallowing the next token the
  # way a flat shell string would (see TemplateCtx.ScopeFlags).
  - pass_arg:
      arg: "--mcp-config"
      value: '{"mcpServers":{"crowbar":{"command":"{crowbar}","args":["mcp","--segment","{segid}","--token","{runner_token}","--project","{project_id}","--workspace","{workspace_id}","--repo","{repo_id}"]}}}'
```

Append to `config_injection` in `descriptors/codex.yaml`:

```yaml
  # The Crowbar tool surface, injected through the same session-layer -c channel
  # the hooks above use: nothing is written to disk and ~/.codex is untouched,
  # which is the law this descriptor is built on. Verified against codex 0.139.0
  # via `codex mcp list -c mcp_servers.…` — the server registers enabled.
  #
  # codex does NOT defer MCP tool schemas, so every tool costs context on every
  # turn here. Keep the surface small.
  - pass_arg: { arg: "-c", value: 'mcp_servers.crowbar.command="{crowbar}"' }
  - pass_arg:
      arg: "-c"
      value: 'mcp_servers.crowbar.args=["mcp","--segment","{segid}","--token","{runner_token}","--project","{project_id}","--workspace","{workspace_id}","--repo","{repo_id}"]'
```

- [ ] **Step 6: Add the capability preamble**

In `api/internal/core/config/default.yaml`, add under `prompts:`:

```yaml
    capabilities_instruction: |
      You are running inside a Crowbar workspace. Crowbar's own tools are available in your tool list; prefer them over shell equivalents for anything they cover, and call them rather than describing what you would do.
```

**The preamble must never name a capability that is not registered yet.** A directive that points at an absent tool family — and forbids the fallback the model would otherwise reach for — is worse than no directive at all. At this phase the only registered tool is `set_chat_title`, so the text stays generic. Task 13 extends it once the review tools exist, and adds the test that keeps the two honest.

In `api/internal/core/config/config.go`, add to the prompts struct beside `TitleInstruction`:

```go
	// CapabilitiesInstruction tells a model that Crowbar's tools exist and, more
	// importantly, WHEN to prefer them. Tool descriptions alone do not override a
	// model's prior — asked to "review this branch" it reaches for gh or writes
	// prose — so this is a directive, not a capability list.
	CapabilitiesInstruction string `yaml:"capabilities_instruction"`
```

In `spawnRunner`, compose it into `{context}` ahead of the title instruction:

```go
	var parts []string
	if capabilities := config.GetPrompts().CapabilitiesInstruction; capabilities != "" {
		parts = append(parts, engineagent.Expand(capabilities, tctx))
	}
	if injectTitle {
		parts = append(parts, engineagent.Expand(config.GetPrompts().TitleInstruction, tctx))
	}
```

- [ ] **Step 7: Run the full backend suite**

Run: `cd api && go test ./... 2>&1 | grep -v "^ok" | head -30`
Expected: no failures.

- [ ] **Step 8: Commit**

```bash
git add api/
git commit -m "feat(agent): inject the crowbar MCP server into claude and codex"
```

---

### Task 9: Phase 0 integration test against real vendor CLIs

**Files:**
- Create: `api/tests/integration/agent/agent_mcp_test.go`
- Test helpers reused: `mustSpawnChat` (`agent_title_test.go:38`), the PTY `drive` helper (`barriers_test.go:380`)

**Interfaces:**
- Consumes: everything from Tasks 1–8.
- Produces: nothing consumed downstream.

- [ ] **Step 1: Write the integration test**

```go
//go:build integration

// Phase 0's gate: a REAL vendor CLI must register the crowbar MCP server from
// the descriptor's injected config and successfully call a tool through the
// relay, the daemon route, the token check and the scope resolver. Nothing below
// this level proves the descriptor injection actually works against the real
// binaries — the unit tests only prove the argv is shaped as intended.
package agent_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMCP_ClaudeTitlesItsChatThroughTheToolSurface(t *testing.T) {
	requireProvider(t, "claude")
	h := newHarness(t)
	_, _, _, chatID, _ := mustSpawnChat(t, h, "claude")

	drive(t, h, chatID, "Use your crowbar tool to set this chat's title to exactly: Widget Refactor. Do nothing else.")
	waitForTitle(t, h, chatID, "Widget Refactor")

	chat, err := h.app.Usecases.Agent.GetChat(context.Background(), chatID)
	require.NoError(t, err)
	require.Equal(t, "Widget Refactor", chat.Title)
}

func TestMCP_CodexTitlesItsChatThroughTheToolSurface(t *testing.T) {
	requireProvider(t, "codex")
	h := newHarness(t)
	_, _, _, chatID, _ := mustSpawnChat(t, h, "codex")

	drive(t, h, chatID, "Use your crowbar tool to set this chat's title to exactly: Widget Refactor. Do nothing else.")
	waitForTitle(t, h, chatID, "Widget Refactor")
}

// A forged token must reach no tool. This drives the daemon route directly
// rather than a CLI, because a real CLI has no way to send a bad token.
func TestMCP_ForgedTokenIsRejected(t *testing.T) {
	h := newHarness(t)
	_, _, _, _, runnerID := mustSpawnChat(t, h, "claude")

	out, send, err := h.app.Usecases.Agent.DispatchMCP(context.Background(), runnerID, "forged",
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"set_chat_title","arguments":{"title":"Nope"}}}`))
	require.NoError(t, err)
	require.True(t, send)
	require.Contains(t, string(out), "unauthorized")
}
```

`waitForTitle` must block on a real signal, never a timer. Implement it against the chat broadcast the daemon already emits on rename — mirror how `barriers_test.go` waits for turn completion. If no such signal is reachable from the harness, block on the runner's `turn_stop` hook and then assert, rather than polling.

`requireProvider` should `t.Skip` when the vendor CLI is not on PATH, mirroring any existing skip helper in the package.

- [ ] **Step 2: Run the integration test**

Run: `cd api && go test -tags integration ./tests/integration/agent/... -run TestMCP -v 2>&1 | tail -40`
Expected: PASS for both providers.

If a provider fails to register the server, debug by inspecting the rendered argv — add a `t.Logf` of `plan.Argv` in the harness — and by running the equivalent probe by hand:
- claude: `claude -p --strict-mcp-config --mcp-config '<the rendered json>'` then ask it to list MCP tools
- codex: `codex mcp list -c 'mcp_servers.crowbar.command=…' -c 'mcp_servers.crowbar.args=[…]'`

- [ ] **Step 3: Commit**

```bash
git add api/tests/
git commit -m "test(mcp): real claude and codex title their chat through the tool surface"
```

- [ ] **Step 4: Report the Phase 0 measurement**

Record, in the commit message or a note appended to the spec, whether the agents called the tool reliably. This is the control experiment: `title_instruction` is known to be ignored, so a materially better rate here is the evidence that justifies Phases 1–2. If the rate is no better, stop and re-plan rather than building on it.

---

# Phase 1 — Review threads

---

### Task 10: Export the review base ref

**Files:**
- Modify: `api/internal/app/usecases/branchreview/branch_review.go` (interface at `:37`, impl), `api/internal/api/v0/endpoints/review/handlers/handlers.go` (`ReviewUsecase` port at `:17`)
- Test: `api/internal/app/usecases/branchreview/branch_review_test.go`

**Interfaces:**
- Consumes: the unexported `resolveDiffRef` (`branch_review.go:159`).
- Produces: `GetBase(ctx context.Context, wsID string) (string, error)` on `branchreview.Usecase`, returning the ref the review diffs against.

**Why:** `get_review_scope` must tell the agent what range the review covers. Without it the agent reviews `HEAD~1` or `main` — whatever it guesses — and every finding anchors against the wrong diff.

- [ ] **Step 1: Write the failing test**

```go
func TestGetBase_ReturnsTheRefTheReviewDiffsAgainst(t *testing.T) {
	// Build the same fixture the existing GetFiles tests use: a workspace whose
	// branch has diverged from its parent. Assert GetBase returns the same ref
	// the diff is actually taken against — i.e. the merge base, not the frozen
	// ForkPointSha, and not the bare branch name.
	uc, ws, wantRef := newDivergedWorkspaceFixture(t)

	got, err := uc.GetBase(context.Background(), ws.ID)
	require.NoError(t, err)
	require.Equal(t, wantRef, got)
}

func TestGetBase_UnknownWorkspaceIsNotFound(t *testing.T) {
	uc, _, _ := newDivergedWorkspaceFixture(t)
	_, err := uc.GetBase(context.Background(), "nope")
	require.Error(t, err)
}
```

Build `newDivergedWorkspaceFixture` by copying the setup used by the nearest existing test in `branch_review_files_test.go` (or whichever file exercises `GetFiles` against a real temp repo) so the ref assertion is against real git output, not a stub.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./internal/app/usecases/branchreview/... -run TestGetBase -v`
Expected: FAIL — `GetBase` undefined.

- [ ] **Step 3: Implement**

Add to the `Usecase` interface in `branch_review.go`:

```go
	// GetBase returns the ref this workspace's review diffs against — the merge
	// base of the parent (or default) branch and HEAD, which is what every
	// /review surface actually uses. It is exported because the agent tool
	// surface must be able to tell a model what range it is reviewing; a model
	// left to guess reviews HEAD~1 or main and anchors every finding wrongly.
	GetBase(ctx context.Context, wsID string) (string, error)
```

Add the implementation beside `GetFiles`:

```go
func (u *branchReviewUsecase) GetBase(
	ctx context.Context,
	wsID string,
) (string, error) {
	ws, err := u.workspaces.Get(ctx, wsID)
	if err != nil {
		return "", fmt.Errorf("branchreview: get base: workspace: %w", err)
	}
	ref, err := u.resolveDiffRef(ctx, ws)
	if err != nil {
		return "", fmt.Errorf("branchreview: get base: resolve ref: %w", err)
	}
	return ref, nil
}
```

Add `GetBase` to the `ReviewUsecase` port in `endpoints/review/handlers/handlers.go` and to any test stub implementing it (find with `rg 'SetMergeStrategy\(ctx context.Context' api/`).

- [ ] **Step 4: Run tests**

Run: `cd api && go test ./internal/app/usecases/branchreview/... ./internal/api/v0/endpoints/review/... -v 2>&1 | tail -20`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/
git commit -m "feat(branchreview): export GetBase so the agent can know what it is reviewing"
```

---

### Task 11: `get_review_scope` and `list_review_threads`

**Files:**
- Create: `api/internal/app/usecases/agenttools/tools_review.go`, `api/internal/app/usecases/agenttools/render.go`
- Modify: `api/internal/app/usecases/agenttools/toolset.go` (register the review tools)
- Test: `api/internal/app/usecases/agenttools/tools_review_test.go`, `api/internal/app/usecases/agenttools/render_test.go`

**Interfaces:**
- Consumes: Task 4 `Caller`, Task 10 `GetBase`.
- Produces:
  - `type ReviewReader interface { GetBase(ctx context.Context, wsID string) (string, error); GetFiles(ctx context.Context, wsID string, commit string) ([]gitdomain.ReviewFileSummary, error); GetOutline(ctx context.Context, wsID string, commit string) ([]gitdomain.FileOutline, error) }`
  - `type ThreadReader interface { ListByWorkspace(ctx context.Context, wsID string) ([]domain.ReviewThread, error); Get(ctx context.Context, id string) (domain.ReviewThread, error) }`
  - `Deps.Review ReviewReader`, `Deps.Threads ThreadWriter` (the write half arrives in Task 12; define the full interface now)
  - `func renderThreads(threads []domain.ReviewThread) string`, `func renderScope(base string, files []gitdomain.ReviewFileSummary) string`

- [ ] **Step 1: Write the failing render test**

```go
func TestRenderThreads_IsLineOrientedWithProseOnItsOwnLine(t *testing.T) {
	out := agenttools.RenderThreadsForTest([]domain.ReviewThread{{
		ID: "t1", FilePath: "src/auth.go", StartLine: 41, EndLine: 47,
		Side: domain.ReviewSideRight, Status: domain.ReviewThreadStatusOpen,
		Messages: []domain.ReviewMessage{
			{ID: "m1", Author: "mateo", Body: "This retry loop can spin forever: token revoked."},
			{ID: "m2", Author: "claude", IsAgent: true, Body: "Agreed, bounding it."},
		},
	}})

	require.Contains(t, out, "t1")
	require.Contains(t, out, "src/auth.go:41-47")
	require.Contains(t, out, "right")
	require.Contains(t, out, "unresolved")

	// The body contains a colon and must not be able to corrupt the row.
	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Equal(t, 1, countLinesContaining(lines, "src/auth.go:41-47"),
		"the anchor row must appear exactly once")
	require.True(t, strings.Contains(out, "This retry loop can spin forever: token revoked."))
}

func TestRenderThreads_EmptyIsExplicit(t *testing.T) {
	require.Contains(t, agenttools.RenderThreadsForTest(nil), "No review threads")
}
```

Export a small test seam (`RenderThreadsForTest = renderThreads`) in an `export_test.go` file rather than exporting the renderer itself.

- [ ] **Step 2: Write the failing tool test**

```go
func TestListReviewThreads_DefaultsToUnresolvedOnly(t *testing.T) {
	// Fixture: two threads on the caller's workspace, one resolved.
	// Assert the default call omits the resolved one and that
	// {"includeResolved":true} includes it.
}

func TestGetReviewScope_ReportsBaseAndChangedFiles(t *testing.T) {
	// Assert the rendered text contains the base ref returned by the stub
	// ReviewReader and every changed file path with its +/- counts.
}

func TestReviewTools_OnlyReadTheCallersOwnWorkspace(t *testing.T) {
	// The stub ThreadReader records which wsID it was asked for; assert it is
	// the caller's own workspace and that no tool exposes a way to name another.
}
```

Fill these in with the same concrete style as the earlier tests — construct the stub, call `ts.Call`, assert on the returned string and on what the stub recorded.

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd api && go test ./internal/app/usecases/agenttools/... -run 'TestRender|TestListReview|TestGetReviewScope|TestReviewTools' -v`
Expected: FAIL.

- [ ] **Step 4: Implement `render.go`**

```go
package agenttools

import (
	"fmt"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// renderThreads emits one anchor row per thread with every message on its own
// indented line.
//
// Keys appear once (in the header), not per row, which is what makes this
// cheaper than JSON or YAML for the same data. Prose is never inlined into a
// row: review bodies are user-authored markdown full of colons, dashes and code
// fences, and inlining them would let a comment corrupt the structure.
func renderThreads(threads []domain.ReviewThread) string {
	if len(threads) == 0 {
		return "No review threads."
	}
	var b strings.Builder
	b.WriteString("id  file:lines  side  state  messages\n")
	for _, t := range threads {
		state := "unresolved"
		if t.IsResolved() {
			state = "resolved"
		}
		fmt.Fprintf(&b, "%s  %s:%d-%d  %s  %s  %d\n",
			t.ID, t.FilePath, t.StartLine, t.EndLine, t.Side, state, len(t.Messages))
		for _, m := range t.Messages {
			author := m.Author
			if m.IsAgent {
				author += " (agent)"
			}
			fmt.Fprintf(&b, "    %s: %s\n", author, m.Body)
		}
	}
	return b.String()
}

func renderScope(base string, files []gitdomain.ReviewFileSummary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "This review covers everything on this branch since %s.\n", base)
	if len(files) == 0 {
		b.WriteString("No changed files.\n")
		return b.String()
	}
	b.WriteString("status  +adds  -dels  path\n")
	for _, f := range files {
		fmt.Fprintf(&b, "%s  +%d  -%d  %s\n", f.Status, f.Additions, f.Deletions, f.Path)
	}
	return b.String()
}
```

- [ ] **Step 5: Implement the two read tools in `tools_review.go`**

Register them from `NewToolSet` via a `reviewTools(deps)` function following the `chatTools` pattern. `list_review_threads` takes `{"includeResolved": bool}` (default false) and calls `deps.Threads.ListByWorkspace(ctx, c.Workspace.ID)`. `get_review_scope` takes no arguments and calls `deps.Review.GetBase` then `deps.Review.GetFiles(ctx, c.Workspace.ID, "")`.

Descriptions, verbatim:

```
list_review_threads: "List the code-review threads the user left on this branch. Call when asked to address, answer, or check review comments."
get_review_scope:    "What this branch review covers: base ref and changed files. Call before reviewing so findings target the right diff."
```

- [ ] **Step 6: Run tests**

Run: `cd api && go test ./internal/app/usecases/agenttools/... -v`
Expected: PASS, including `TestToolSet_RespectsToolCeiling` (now 3 tools) and `TestToolSet_NoToolAcceptsAScopeArgument`.

- [ ] **Step 7: Commit**

```bash
git add api/internal/app/usecases/agenttools/
git commit -m "feat(agenttools): get_review_scope and list_review_threads"
```

---

### Task 12: `post_review_comment` with anchor validation and idempotency

**Files:**
- Modify: `api/internal/app/usecases/agenttools/tools_review.go`
- Test: `api/internal/app/usecases/agenttools/tools_review_test.go`

**Interfaces:**
- Consumes: Task 11's `ReviewReader.GetOutline`, `Deps`.
- Produces: `type ThreadWriter interface { Open(ctx context.Context, in reviewthread.OpenInput, now time.Time) (domain.ReviewThread, error); Reply(ctx context.Context, id, messageID, author string, isAgent bool, body string, now time.Time) (domain.ReviewThread, error); Resolve(ctx context.Context, id string) (domain.ReviewThread, error) }` on `Deps.ThreadWrites`.

- [ ] **Step 1: Write the failing tests**

```go
func TestPostReviewComment_AnchorsAndMarksItselfAsAgent(t *testing.T) {
	// Assert reviewthread.OpenInput carries WsID = caller's workspace,
	// FilePath/StartLine/EndLine/Side from the args, IsAgent = true, and a
	// non-empty Author.
}

// The whole correctness risk: an anchor outside any hunk floats off the diff.
func TestPostReviewComment_RejectsAnAnchorOutsideAnyHunk(t *testing.T) {
	// Stub GetOutline with a file whose only hunk is NewStart 40, NewLines 10.
	// Posting at line 200 must error and must NOT call Open.
}

func TestPostReviewComment_RejectsAnUnknownFile(t *testing.T) {
	// A path absent from the outline must error and must NOT call Open.
}

func TestPostReviewComment_LeftSideAnchorsAgainstOldLineNumbers(t *testing.T) {
	// side="left" validates against OldStart/OldLines, not NewStart/NewLines.
}

func TestPostReviewComment_IdempotencyKeyCollapsesARetry(t *testing.T) {
	// Two calls with the same idempotencyKey open exactly one thread and return
	// the same thread id both times.
}

func TestPostReviewComment_DifferentKeysOpenDifferentThreads(t *testing.T) {}
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd api && go test ./internal/app/usecases/agenttools/... -run TestPostReviewComment -v`
Expected: FAIL.

- [ ] **Step 3: Implement**

Schema:

```json
{
  "type":"object",
  "properties":{
    "filePath":{"type":"string","description":"Path as it appears in the review's changed-file list."},
    "startLine":{"type":"integer","description":"First line of the anchor, in the numbering of the chosen side."},
    "endLine":{"type":"integer","description":"Last line of the anchor. Same as startLine for a single line."},
    "side":{"type":"string","enum":["left","right"],"description":"left = the base revision, right = this branch. Use right unless commenting on removed code."},
    "body":{"type":"string","description":"The finding, in markdown."},
    "idempotencyKey":{"type":"string","description":"Stable key for this finding; a retry with the same key will not duplicate the comment."}
  },
  "required":["filePath","startLine","endLine","side","body"],
  "additionalProperties":false
}
```

Description, verbatim:

```
"Post a review finding as a thread anchored to a file and line range, visible in Crowbar's review UI. Use this instead of writing findings in chat when reviewing a branch."
```

Validation, before any write:

```go
// An anchor that does not land in a hunk of the CURRENT review is rejected
// rather than stored: a floating comment is worse than no comment, because the
// user sees a finding with no code next to it and cannot tell what it refers to.
func validateAnchor(outline []gitdomain.FileOutline, path string, start, end int, side domain.ReviewSide) error {
	for _, f := range outline {
		if f.Path != path && f.OldPath != path {
			continue
		}
		for _, h := range f.Hunks {
			lo, span := h.NewStart, h.NewLines
			if side == domain.ReviewSideLeft {
				lo, span = h.OldStart, h.OldLines
			}
			if start >= lo && end <= lo+span-1 {
				return nil
			}
		}
		return fmt.Errorf("agenttools: lines %d-%d of %s are not in any changed hunk on the %s side; call get_review_scope and anchor inside a changed range", start, end, path, side)
	}
	return fmt.Errorf("agenttools: %s is not part of this review; call get_review_scope for the changed files", path)
}
```

Idempotency: keep a per-`ToolSet`-owner map from `(workspaceID, idempotencyKey)` to the created thread id. Because a `ToolSet` is per request, this map must live on `Deps` (guarded by a mutex), not on the `ToolSet`. Add it as a small `idempotency` struct in `toolset.go` constructed once by the container.

Author: use the caller's provider id from the runner so the UI can attribute the comment; `IsAgent: true` always.

- [ ] **Step 4: Run tests**

Run: `cd api && go test ./internal/app/usecases/agenttools/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/usecases/agenttools/
git commit -m "feat(agenttools): post_review_comment with hunk-anchored validation"
```

---

### Task 13: `reply_to_review_thread` and `resolve_review_thread`

**Files:**
- Modify: `api/internal/app/usecases/agenttools/tools_review.go`
- Test: `api/internal/app/usecases/agenttools/tools_review_test.go`

**Interfaces:**
- Consumes: Task 12's `ThreadWriter`.
- Produces: two more `toolDef`s. Tool count reaches 6 — still under the ceiling of 8.

Descriptions, verbatim:

```
reply_to_review_thread:  "Reply to an existing review thread. Get thread ids from list_review_threads."
resolve_review_thread:   "Mark a review thread resolved. Only resolve a thread whose finding you have actually addressed."
```

- [ ] **Step 1: Write the failing tests**

```go
// spyThreads records every write so a test can assert a rejected call never
// reached the store. thread is what Get returns.
type spyThreads struct {
	thread   domain.ReviewThread
	opened   []reviewthread.OpenInput
	replied  []string
	resolved []string
}

func (s *spyThreads) Get(context.Context, string) (domain.ReviewThread, error) {
	return s.thread, nil
}
func (s *spyThreads) ListByWorkspace(context.Context, string) ([]domain.ReviewThread, error) {
	return []domain.ReviewThread{s.thread}, nil
}
func (s *spyThreads) Open(_ context.Context, in reviewthread.OpenInput, _ time.Time) (domain.ReviewThread, error) {
	s.opened = append(s.opened, in)
	return domain.ReviewThread{ID: "new-thread", WsID: in.WsID}, nil
}
func (s *spyThreads) Reply(_ context.Context, id, _, _ string, _ bool, _ string, _ time.Time) (domain.ReviewThread, error) {
	s.replied = append(s.replied, id)
	return s.thread, nil
}
func (s *spyThreads) Resolve(_ context.Context, id string) (domain.ReviewThread, error) {
	s.resolved = append(s.resolved, id)
	return s.thread, nil
}

// reviewToolsOn builds a ToolSet whose caller sits on ws-a (so it sees ws-a and
// ws-a1, and NOT repo-default, ws-b or other-repo-ws).
func reviewToolsOn(t *testing.T, threads *spyThreads) *agenttools.ToolSet {
	t.Helper()
	m, err := agenttools.NewTokenMinter()
	require.NoError(t, err)
	res := agenttools.NewResolver(m,
		stubRunners{r: domain.AgentRunner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
		stubChats{c: domain.AgentChat{ID: "CHAT", WorkspaceID: "ws-a"}},
		stubWorkspaces{all: tree()})
	return agenttools.NewToolSet(agenttools.Deps{
		Resolver: res, Threads: threads, ThreadWrites: threads,
	}, "RUN", m.Mint("RUN"))
}

func TestReplyToReviewThread_AppendsAsAgent(t *testing.T) {
	spy := &spyThreads{thread: domain.ReviewThread{ID: "t1", WsID: "ws-a"}}
	ts := reviewToolsOn(t, spy)

	out, err := ts.Call(context.Background(), "reply_to_review_thread",
		json.RawMessage(`{"threadId":"t1","body":"Bounded the loop."}`))
	require.NoError(t, err)
	require.Contains(t, out, "t1")
	require.Equal(t, []string{"t1"}, spy.replied)
}

// The scope hole to close: a thread id names a thread in SOME workspace, so the
// id itself is not an authorization. ws-b is a sibling the caller cannot see.
func TestReplyToReviewThread_RejectsAThreadOutsideTheCallersScope(t *testing.T) {
	spy := &spyThreads{thread: domain.ReviewThread{ID: "t9", WsID: "ws-b"}}
	ts := reviewToolsOn(t, spy)

	_, err := ts.Call(context.Background(), "reply_to_review_thread",
		json.RawMessage(`{"threadId":"t9","body":"should not land"}`))
	require.ErrorIs(t, err, agenttools.ErrOutOfScope)
	require.Empty(t, spy.replied, "an out-of-scope reply must never reach the store")
}

// An ANCESTOR is out of scope too — visibility is downward only.
func TestReplyToReviewThread_RejectsAThreadOnAnAncestorWorkspace(t *testing.T) {
	spy := &spyThreads{thread: domain.ReviewThread{ID: "t0", WsID: "repo-default"}}
	ts := reviewToolsOn(t, spy)

	_, err := ts.Call(context.Background(), "reply_to_review_thread",
		json.RawMessage(`{"threadId":"t0","body":"upward is forbidden"}`))
	require.ErrorIs(t, err, agenttools.ErrOutOfScope)
	require.Empty(t, spy.replied)
}

// A DESCENDANT is in scope.
func TestReplyToReviewThread_AllowsAThreadOnADescendantWorkspace(t *testing.T) {
	spy := &spyThreads{thread: domain.ReviewThread{ID: "t2", WsID: "ws-a1"}}
	ts := reviewToolsOn(t, spy)

	_, err := ts.Call(context.Background(), "reply_to_review_thread",
		json.RawMessage(`{"threadId":"t2","body":"ok"}`))
	require.NoError(t, err)
	require.Equal(t, []string{"t2"}, spy.replied)
}

func TestResolveReviewThread_Resolves(t *testing.T) {
	spy := &spyThreads{thread: domain.ReviewThread{ID: "t1", WsID: "ws-a"}}
	ts := reviewToolsOn(t, spy)

	_, err := ts.Call(context.Background(), "resolve_review_thread",
		json.RawMessage(`{"threadId":"t1"}`))
	require.NoError(t, err)
	require.Equal(t, []string{"t1"}, spy.resolved)
}

func TestResolveReviewThread_RejectsAThreadOutsideTheCallersScope(t *testing.T) {
	spy := &spyThreads{thread: domain.ReviewThread{ID: "t9", WsID: "other-repo-ws"}}
	ts := reviewToolsOn(t, spy)

	_, err := ts.Call(context.Background(), "resolve_review_thread",
		json.RawMessage(`{"threadId":"t9"}`))
	require.ErrorIs(t, err, agenttools.ErrOutOfScope)
	require.Empty(t, spy.resolved, "an out-of-scope resolve must never reach the store")
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd api && go test ./internal/app/usecases/agenttools/... -run 'TestReplyToReviewThread|TestResolveReviewThread' -v`
Expected: FAIL.

- [ ] **Step 3: Implement**

Both tools must first `Get` the thread and check `c.CanSee(thread.WsID)`, returning `ErrOutOfScope` otherwise — a thread id is not itself an authorization.

- [ ] **Step 3b: Extend the capability preamble, now that the review tools exist**

The review surface is complete as of this task, so `capabilities_instruction` in `api/internal/core/config/default.yaml` can finally carry the directive that was held back in Task 8:

```yaml
    capabilities_instruction: |
      You are running inside a Crowbar workspace. Crowbar's own tools are available in your tool list; prefer them over shell equivalents for anything they cover, and call them rather than describing what you would do.
      Code review happens in Crowbar, not on GitHub: when you review a branch or are asked about review comments, use the crowbar review tools. Post findings as anchored review threads rather than as prose in this chat, and do not use `gh pr review`.
```

Then add the test that keeps the prompt and the registry honest — a preamble naming an absent tool family, while forbidding the fallback, is worse than no preamble:

```go
// The preamble is a DIRECTIVE, so it must never name a capability the agent does
// not actually have. Every `x_y`-shaped token in it has to be a registered tool.
func TestCapabilitiesPreamble_OnlyNamesRegisteredTools(t *testing.T) {
	ts := reviewToolsOn(t, &spyThreads{})
	registered := map[string]bool{}
	for _, tool := range ts.Tools() {
		registered[tool.Name] = true
	}

	preamble := config.GetPrompts().CapabilitiesInstruction
	require.NotEmpty(t, preamble)
	for _, word := range regexp.MustCompile(`\b[a-z]+(?:_[a-z]+)+\b`).FindAllString(preamble, -1) {
		require.True(t, registered[word],
			"the preamble names %q, which is not a registered tool", word)
	}
}
```

- [ ] **Step 4: Run tests**

Run: `cd api && go test ./internal/app/usecases/agenttools/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/usecases/agenttools/
git commit -m "feat(agenttools): reply_to_review_thread and resolve_review_thread"
```

---

### Task 14: Phase 1 integration test

**Files:**
- Modify: `api/tests/integration/agent/agent_mcp_test.go`

- [ ] **Step 1: Write the test**

```go
//go:build integration

// A real claude, given a real diff and a real review comment, must answer that
// comment as a THREAD REPLY rather than as chat prose. This is the product
// thesis end to end.
func TestMCP_ClaudeAnswersAReviewThread(t *testing.T) {
	requireProvider(t, "claude")
	h := newHarness(t)
	// Build a workspace with a committed change, open a user thread on it via
	// the thread store, then spawn the chat.
	// drive(): "There is a review comment on this branch. Read it and reply to it."
	// Assert: the thread now has 2 messages and the second has IsAgent == true.
}

func TestMCP_ClaudePostsAFindingAsAnAnchoredThread(t *testing.T) {
	// drive(): "Review this branch and post any finding as a review comment."
	// Assert: at least one thread exists on the workspace with IsAgent == true
	// and a FilePath that appears in the review's changed-file list.
}
```

- [ ] **Step 2: Run**

Run: `cd api && go test -tags integration ./tests/integration/agent/... -run TestMCP -v 2>&1 | tail -40`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add api/tests/
git commit -m "test(mcp): real claude answers and posts review threads"
```

---

# Phase 2 — Chat context

---

### Task 15: `list_workspaces`

**Files:**
- Create: `api/internal/app/usecases/agenttools/tools_context.go`
- Modify: `api/internal/app/usecases/agenttools/render.go`, `toolset.go`
- Test: `api/internal/app/usecases/agenttools/tools_context_test.go`

**Interfaces:**
- Consumes: Task 4's `Caller.Visible`, `ChatReader.ListByWorkspace`.
- Produces: one `toolDef` (tool count 7) and `func renderWorkspaces(caller domain.Workspace, visible []domain.Workspace, chats map[string][]domain.AgentChat) string`.

Description, verbatim:

```
"List the workspaces this chat can see — itself and its children, or the whole repo or project depending on where it runs — each with its chats."
```

- [ ] **Step 1: Write the failing tests**

```go
func TestListWorkspaces_ListsOnlyTheVisibleSetAndMarksSelf(t *testing.T) {}
func TestListWorkspaces_IncludesEachWorkspacesChats(t *testing.T) {}
func TestListWorkspaces_NeverListsAnAncestor(t *testing.T) {}
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd api && go test ./internal/app/usecases/agenttools/... -run TestListWorkspaces -v`
Expected: FAIL.

- [ ] **Step 3: Implement**

Take no arguments. Iterate `c.Visible`, call `deps.Chats2.ListByWorkspace` per workspace, render line-oriented with a `*` marking the caller's own workspace. Chats are folded in here rather than given their own tool to stay under the ceiling.

- [ ] **Step 4: Run tests, then commit**

```bash
cd api && go test ./internal/app/usecases/agenttools/... -v
git add api/internal/app/usecases/agenttools/
git commit -m "feat(agenttools): list_workspaces over the visible set"
```

---

### Task 16: `get_chat_log`

**Files:**
- Modify: `api/internal/app/usecases/agenttools/tools_context.go`
- Test: `api/internal/app/usecases/agenttools/tools_context_test.go`

**Interfaces:**
- Consumes: `ledger.Ledger.RenderConversation` (`api/internal/app/ledger/ledger.go:82`), reached through a new `Deps.ChatLogs ChatLogReader` with `ReadChatLog(ctx context.Context, chatID string) (string, error)`. Implement `ReadChatLog` on the agent usecase by reusing `openLedger` (`agent.go:1888`).
- Produces: one `toolDef` (tool count 8 — at the ceiling; adding a ninth requires merging two first).

Description, verbatim:

```
"Read the conversation of another chat you can see. Get chat ids from list_workspaces."
```

- [ ] **Step 1: Write the failing tests**

```go
type stubChatLogs struct {
	log  string
	read []string
}

func (s *stubChatLogs) ReadChatLog(_ context.Context, chatID string) (string, error) {
	s.read = append(s.read, chatID)
	return s.log, nil
}

// chatOn builds a ToolSet on ws-a whose ChatReader resolves the named chat into
// the given workspace.
func chatLogToolsOn(t *testing.T, target domain.AgentChat, logs *stubChatLogs) *agenttools.ToolSet {
	t.Helper()
	m, err := agenttools.NewTokenMinter()
	require.NoError(t, err)
	chats := stubChats{c: target}
	res := agenttools.NewResolver(m,
		stubRunners{r: domain.AgentRunner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
		chats, stubWorkspaces{all: tree()})
	return agenttools.NewToolSet(agenttools.Deps{
		Resolver: res, Chats2: chats, ChatLogs: logs,
	}, "RUN", m.Mint("RUN"))
}

func TestGetChatLog_ReturnsTheLedgerRendering(t *testing.T) {
	logs := &stubChatLogs{log: "user: hello\n\nassistant (claude): hi\n"}
	ts := chatLogToolsOn(t, domain.AgentChat{ID: "other", WorkspaceID: "ws-a1"}, logs)

	out, err := ts.Call(context.Background(), "get_chat_log", json.RawMessage(`{"chatId":"other"}`))
	require.NoError(t, err)
	require.Contains(t, out, "assistant (claude): hi")
	require.Equal(t, []string{"other"}, logs.read)
}

// A chat id is not an authorization: the chat's workspace must be visible.
// ws-b is a sibling, so it is not.
func TestGetChatLog_RejectsAChatOutsideTheCallersScope(t *testing.T) {
	logs := &stubChatLogs{log: "secret"}
	ts := chatLogToolsOn(t, domain.AgentChat{ID: "other", WorkspaceID: "ws-b"}, logs)

	_, err := ts.Call(context.Background(), "get_chat_log", json.RawMessage(`{"chatId":"other"}`))
	require.ErrorIs(t, err, agenttools.ErrOutOfScope)
	require.Empty(t, logs.read, "an out-of-scope chat log must never be read from disk")
}

func TestGetChatLog_RejectsAChatOnAnAncestorWorkspace(t *testing.T) {
	logs := &stubChatLogs{log: "secret"}
	ts := chatLogToolsOn(t, domain.AgentChat{ID: "other", WorkspaceID: "repo-default"}, logs)

	_, err := ts.Call(context.Background(), "get_chat_log", json.RawMessage(`{"chatId":"other"}`))
	require.ErrorIs(t, err, agenttools.ErrOutOfScope)
	require.Empty(t, logs.read)
}

// An empty ledger is a normal state — a chat that has not spoken yet — and must
// read as such rather than as a failure the model tries to work around.
func TestGetChatLog_EmptyLedgerIsExplicitNotAnError(t *testing.T) {
	logs := &stubChatLogs{log: ""}
	ts := chatLogToolsOn(t, domain.AgentChat{ID: "other", WorkspaceID: "ws-a"}, logs)

	out, err := ts.Call(context.Background(), "get_chat_log", json.RawMessage(`{"chatId":"other"}`))
	require.NoError(t, err)
	require.Contains(t, out, "no turns")
}
```

- [ ] **Step 2: Run to verify they fail, then implement**

Resolve the chat via `deps.Chats2.Get`, check `c.CanSee(chat.WorkspaceID)` → `ErrOutOfScope`, then read the ledger. Prose comes back as-is — it is 95% free text and nesting it in any structure only adds escaping.

- [ ] **Step 3: Run tests, then commit**

```bash
cd api && go test ./internal/app/usecases/agenttools/... -v
git add api/internal/app/usecases/agenttools/
git commit -m "feat(agenttools): get_chat_log over the visible set"
```

---

### Task 17: Compliance instrumentation

**Files:**
- Create: `api/internal/app/usecases/agenttools/metrics.go`
- Modify: `api/internal/app/usecases/agenttools/toolset.go`
- Test: `api/internal/app/usecases/agenttools/metrics_test.go`

**Interfaces:**
- Produces: `type Metrics struct{ ... }`, `func NewMetrics() *Metrics`, `func (m *Metrics) Record(tool string, ok bool)`, `func (m *Metrics) Snapshot() map[string]ToolStat`, `type ToolStat struct{ Calls, Failures int }`. Wired as `Deps.Metrics` and called from `ToolSet.Call`.

**Why:** without a number, "do agents actually use these tools?" stays a feeling. `title_instruction` is known to be ignored; this is how the improvement gets measured.

- [ ] **Step 1: Write the failing test**

```go
func TestMetrics_CountsCallsAndFailuresPerTool(t *testing.T) {}
func TestMetrics_IsSafeUnderConcurrentCalls(t *testing.T) {
	// Run with -race: N goroutines calling Record concurrently.
}
func TestToolSet_RecordsEveryCallIncludingUnauthorized(t *testing.T) {
	// An unauthorized call is the most interesting datum — it must be counted.
}
```

- [ ] **Step 2: Implement, run with `-race`, commit**

```bash
cd api && go test -race ./internal/app/usecases/agenttools/... -v
git add api/internal/app/usecases/agenttools/
git commit -m "feat(agenttools): per-tool call and failure counters"
```

---

### Task 18: Live verification in the Tauri dev app

**Files:** none — this is a manual gate whose result is recorded in the plan and the spec.

**Prerequisites:** no other Crowbar dev instance running (`pgrep -fl crowbar-api` must be empty of dev instances; reuse rather than stack).

- [ ] **Step 1: Build and launch the dev app**

Run: `make dev-desktop`
This builds the sidecar from source and uses the isolated dev `CROWBAR_HOME`. Never verify against the production Crowbar.

- [ ] **Step 2: Set up the scenario**

1. Import or open a repo, create a workspace with a branch carrying a real diff.
2. Open the review pane and leave a review comment on a specific line.

- [ ] **Step 3: Confirm the tools registered**

In the workspace, open an agent chat on claude. Drive it by writing into its PTY through the daemon's terminal API (the MCP bridge cannot inject xterm keystrokes) with: `List your available MCP tools.`
Expected: `mcp__crowbar__*` tools appear, including `list_review_threads`.

- [ ] **Step 4: The product thesis**

Drive: `There is a review comment on this branch. Read it and reply to it.`
Then screenshot the **review pane** through the Tauri MCP bridge.

Expected: the agent's reply is rendered **in the review pane** as a reply on the user's thread, styled as agent-authored — not as prose in the chat.

- [ ] **Step 5: Repeat on codex**

Same scenario, codex provider. Both must pass.

- [ ] **Step 6: Record the result**

Append a short "Live verification" section to the spec recording: date, both provider results, the compliance observation from Task 9, and any behaviour that differed from the design. If step 4 fails on either provider, that is a blocking result — report it rather than narrowing the claim.

---

## Self-review notes

- **Spec coverage:** §4 architecture → Tasks 1,2,6,7. §5 in-daemon rationale → Task 6/7. §6 injection → Task 8. §7.1 token → Task 3. §7.2 scope → Task 4. §8.1 tools → Tasks 5,11,12,13. §8.2 → Tasks 15,16. §8.3 held (`create_workspace` deliberately absent). §8.4 schema requirements → Task 12. §9 response format → Task 11 render. §10 triggering → Task 8 step 6. §11 build order → phase headings. §12 testing → Tasks 9,14,18. §13 rejected alternatives → Global Constraints (no skills). §14 risks → Task 9 step 4 gate.
- **Deliberately not built:** `create_workspace`, any skill artifact, `--disallowed-tools` enforcement, thread anchor revisions. All are named in the spec as held or out of scope.
- **Tool count trajectory:** 1 (T5) → 3 (T11) → 4 (T12) → 6 (T13) → 7 (T15) → 8 (T16). The ceiling test in Task 5 guards it at every step.
