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
}

func TestServer_PingReturnsEmptyResult(t *testing.T) {
	out, _ := srv(&fakeTools{}).Handle(context.Background(),
		[]byte(`{"jsonrpc":"2.0","id":6,"method":"ping"}`))
	require.JSONEq(t, `{"jsonrpc":"2.0","id":6,"result":{}}`, string(out))
}
