package agent_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/agent"
	"github.com/char2cs/crowbar/api/internal/app/usecases/agenttools"
	enginemcp "github.com/char2cs/crowbar/api/internal/engine/mcp"
)

// mcpResult decodes a JSON-RPC response's result member, failing the test on a
// JSON-RPC error (which is a TRANSPORT fault here — a tool that merely failed
// comes back as a successful result carrying isError).
func mcpResult(
	t *testing.T,
	raw []byte,
	into any,
) {
	t.Helper()
	var resp struct {
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	require.NoError(t, json.Unmarshal(raw, &resp))
	require.Nil(t, resp.Error, "unexpected JSON-RPC error: %s", resp.Error)
	require.NoError(t, json.Unmarshal(resp.Result, into))
}

type mcpCallResult struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

// TestDispatchMCP_InitializeAnnouncesCrowbar proves the seam builds a server
// that speaks the pinned protocol revision and identifies itself as crowbar —
// the handshake every vendor CLI's MCP client performs before it will call a
// tool.
func TestDispatchMCP_InitializeAnnouncesCrowbar(
	t *testing.T,
) {
	f := newFixture(t)
	_, runnerID := f.spawn(t, "claude")

	out, send, err := f.usecase.DispatchMCP(f.ctx, runnerID, f.minter.Mint(runnerID),
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	require.NoError(t, err)
	require.True(t, send)

	var res struct {
		ProtocolVersion string `json:"protocolVersion"`
		ServerInfo      struct {
			Name string `json:"name"`
		} `json:"serverInfo"`
	}
	mcpResult(t, out, &res)
	assert.Equal(t, enginemcp.ProtocolVersion, res.ProtocolVersion)
	assert.Equal(t, "crowbar", res.ServerInfo.Name)
}

// TestDispatchMCP_ListsTheChatTools proves the usecase wires itself in as the
// tool surface's ChatRenamer. agenttools registers nothing for a nil dependency,
// so a seam that failed to do this would still answer tools/list — with an empty
// array, and an agent that silently has no tools is the failure mode worth a
// test of its own.
func TestDispatchMCP_ListsTheChatTools(
	t *testing.T,
) {
	f := newFixture(t)
	_, runnerID := f.spawn(t, "claude")

	out, send, err := f.usecase.DispatchMCP(f.ctx, runnerID, f.minter.Mint(runnerID),
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	require.NoError(t, err)
	require.True(t, send)

	var res struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	mcpResult(t, out, &res)

	names := make([]string, 0, len(res.Tools))
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	assert.Contains(t, names, "set_chat_title")
}

// TestDispatchMCP_ToolCallRenamesTheCallersChat is the end-to-end proof of the
// seam: an authenticated tools/call reaches a handler, and that handler's effect
// lands on the chat the CALLING runner is on right now — resolved from the
// runner, never from an argument, which is why no tool takes a chat id.
func TestDispatchMCP_ToolCallRenamesTheCallersChat(
	t *testing.T,
) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	out, send, err := f.usecase.DispatchMCP(f.ctx, runnerID, f.minter.Mint(runnerID),
		[]byte(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":`+
			`{"name":"set_chat_title","arguments":{"title":"Fix The Parser"}}}`))
	require.NoError(t, err)
	require.True(t, send)

	var res mcpCallResult
	mcpResult(t, out, &res)
	require.False(t, res.IsError, "tool reported an error: %+v", res.Content)

	assert.Equal(t, "Fix The Parser", f.chat(t, chatID).Title)
}

// TestDispatchMCP_RejectsAnotherRunnersToken is the authority test: holding a
// token minted for a DIFFERENT runner must not let a caller act as this one. The
// refusal comes back as a tool error rather than a transport error — that is how
// the model is told — but the write must not happen.
func TestDispatchMCP_RejectsAnotherRunnersToken(
	t *testing.T,
) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	_, otherRunnerID := f.spawn(t, "claude")

	out, send, err := f.usecase.DispatchMCP(f.ctx, runnerID, f.minter.Mint(otherRunnerID),
		[]byte(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":`+
			`{"name":"set_chat_title","arguments":{"title":"Stolen"}}}`))
	require.NoError(t, err)
	require.True(t, send)

	var res mcpCallResult
	mcpResult(t, out, &res)
	assert.True(t, res.IsError, "a token minted for another runner must not authenticate")
	assert.Empty(t, f.chat(t, chatID).Title, "the rejected call must not have written")
}

// TestDispatchMCP_NotificationIsSilent proves a JSON-RPC notification produces
// no reply at all, which the HTTP seam turns into 204.
func TestDispatchMCP_NotificationIsSilent(
	t *testing.T,
) {
	f := newFixture(t)
	_, runnerID := f.spawn(t, "claude")

	out, send, err := f.usecase.DispatchMCP(f.ctx, runnerID, f.minter.Mint(runnerID),
		[]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	require.NoError(t, err)
	assert.False(t, send)
	assert.Empty(t, out)
}

// TestDispatchMCP_UnconfiguredSurfaceIsAnError proves a usecase built without a
// tool surface REFUSES the call instead of answering with an empty tool list. A
// wiring mistake has to be loud: an agent that silently has no tools looks
// identical to an agent that chose not to use them.
func TestDispatchMCP_UnconfiguredSurfaceIsAnError(
	t *testing.T,
) {
	u := agentusecase.New(nil, nil, nil, nil, nil, nil, nil, nil, nil, agenttools.Deps{})

	_, send, err := u.DispatchMCP(t.Context(), "seg-1", "tok",
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	require.Error(t, err)
	assert.False(t, send)
}
