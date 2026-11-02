package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMCP_ReturnsRPCResponseInEnvelope proves MCP forwards the :segid path
// param, the body's token and the RAW rpc message to DispatchMCP, and returns
// the reply verbatim under the standard envelope's data.rpc. The rpc message
// must reach the usecase unparsed: the JSON-RPC framing is the engine's
// business, and re-encoding it here would let the handler silently reshape a
// message it does not understand.
func TestMCP_ReturnsRPCResponseInEnvelope(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{
		dispatchMCPOut:  []byte(`{"jsonrpc":"2.0","id":1,"result":{}}`),
		dispatchMCPSend: true,
	}
	h := newChatHandlers(uc)

	body := []byte(`{"token":"TOK","rpc":{"jsonrpc":"2.0","id":1,"method":"ping"}}`)
	ctx, rec := newTestContext(t, http.MethodPost, "/v0/chats/runners/seg-1/mcp", body)
	ctx.Params = gin.Params{{Key: "segid", Value: "seg-1"}}

	h.MCP(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, uc.dispatchMCPCalls, 1)
	assert.Equal(t, "seg-1", uc.dispatchMCPCalls[0].runnerID)
	assert.Equal(t, "TOK", uc.dispatchMCPCalls[0].token)
	assert.JSONEq(t, `{"jsonrpc":"2.0","id":1,"method":"ping"}`, string(uc.dispatchMCPCalls[0].message))

	var env struct {
		Success bool `json:"success"`
		Data    struct {
			RPC json.RawMessage `json:"rpc"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	assert.True(t, env.Success)
	assert.JSONEq(t, `{"jsonrpc":"2.0","id":1,"result":{}}`, string(env.Data.RPC))
}

// TestMCP_NotificationGets204 proves a message the engine answers with silence
// (a JSON-RPC notification) produces an empty 204, never an envelope: a client
// that sent notifications/initialized must not receive a stray reply.
func TestMCP_NotificationGets204(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{dispatchMCPSend: false}
	h := newChatHandlers(uc)

	body := []byte(`{"token":"TOK","rpc":{"jsonrpc":"2.0","method":"notifications/initialized"}}`)
	ctx, rec := newTestContext(t, http.MethodPost, "/v0/chats/runners/seg-1/mcp", body)
	ctx.Params = gin.Params{{Key: "segid", Value: "seg-1"}}

	h.MCP(ctx)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Empty(t, rec.Body.Bytes())
}
