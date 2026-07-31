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

func TestRequest_ExplicitNullIDIsANotification(t *testing.T) {
	var req mcp.Request
	require.NoError(t, json.Unmarshal([]byte(`{"jsonrpc":"2.0","id":null,"method":"notifications/initialized"}`), &req))
	require.True(t, req.IsNotification())
}
