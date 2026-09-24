package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/protocol"
)

// send writes a raw JSON-RPC message from the fake server to the client.
func (f *fakeServer) send(
	t *testing.T,
	raw string,
) {
	t.Helper()
	require.NoError(t, protocol.WriteMessage(f.conn, []byte(raw)))
}

func (f *fakeServer) reply(
	t *testing.T,
) map[string]json.RawMessage {
	t.Helper()
	var r map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(<-f.gotReply, &r))
	return r
}

func TestServer_AnswersEveryServerRequestWithItsOwnID(t *testing.T) {
	_, fake := newTestServer(t)

	fake.send(t, `{"jsonrpc":"2.0","id":"cfg-1","method":"workspace/configuration",`+
		`"params":{"items":[{"section":"gopls"},{"section":"go"}]}}`)
	r := fake.reply(t)
	assert.JSONEq(t, `"cfg-1"`, string(r["id"]))
	assert.JSONEq(t, `[null,null]`, string(r["result"]))

	fake.send(t, `{"jsonrpc":"2.0","id":7,"method":"client/registerCapability","params":{}}`)
	r = fake.reply(t)
	assert.JSONEq(t, `7`, string(r["id"]))
	assert.JSONEq(t, `null`, string(r["result"]))

	fake.send(t, `{"jsonrpc":"2.0","id":8,"method":"window/showDocument","params":{}}`)
	r = fake.reply(t)
	assert.JSONEq(t, `8`, string(r["id"]))
	assert.NotContains(t, r, "result")
	assert.Contains(t, string(r["error"]), "-32601")
}

// A server request must never be mistaken for the response to our own request
// that happens to share its numeric id.
func TestServer_ServerRequestDoesNotResolveAPendingRequest(t *testing.T) {
	srv, fake := newTestServer(t)

	done := make(chan json.RawMessage, 1)
	go func() {
		raw, _ := srv.Request(context.Background(), "textDocument/hover", map[string]any{})
		done <- raw
	}()
	req := <-fake.gotReq

	fake.send(t, `{"jsonrpc":"2.0","id":1,"method":"window/workDoneProgress/create","params":{}}`)
	fake.reply(t)
	fake.respond(req.ID, map[string]any{"contents": "real"})

	assert.JSONEq(t, `{"contents":"real"}`, string(<-done))
}

func TestServer_ExecuteCommandCollectsTheEditsTheServerApplies(t *testing.T) {
	srv, fake := newTestServer(t)

	type outcome struct {
		result json.RawMessage
		edits  []json.RawMessage
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, edits, err := srv.ExecuteCommand(context.Background(),
			map[string]any{"command": "gopls.tidy"})
		done <- outcome{result, edits, err}
	}()
	req := <-fake.gotReq
	assert.Equal(t, "workspace/executeCommand", req.Method)

	edit := `{"changes":{"file:///repo/go.mod":[]}}`
	fake.send(t, `{"jsonrpc":"2.0","id":100,"method":"workspace/applyEdit","params":{"edit":`+edit+`}}`)
	assert.JSONEq(t, `{"applied":true}`, string(fake.reply(t)["result"]))
	fake.respond(req.ID, nil)

	got := <-done
	require.NoError(t, got.err)
	require.Len(t, got.edits, 1)
	assert.JSONEq(t, edit, string(got.edits[0]))
}

func TestServer_ApplyEditOutsideACommandIsRefused(t *testing.T) {
	_, fake := newTestServer(t)

	fake.send(t, `{"jsonrpc":"2.0","id":3,"method":"workspace/applyEdit","params":{"edit":{"changes":{}}}}`)
	var result struct {
		Applied bool `json:"applied"`
	}
	require.NoError(t, json.Unmarshal(fake.reply(t)["result"], &result))
	assert.False(t, result.Applied)
}

func TestServer_HandshakeRecordsServerFeaturesAndSendsInitOptions(t *testing.T) {
	srv, fake := newTestServer(t)
	impl, ok := srv.(*server)
	require.True(t, ok)
	impl.initOptions = map[string]any{"semanticTokens": true}

	done := make(chan error, 1)
	go func() { done <- srv.Initialize(context.Background(), "/repo") }()
	req := <-fake.gotReq
	assert.Contains(t, string(req.Params), `"initializationOptions":{"semanticTokens":true}`)

	fake.respond(req.ID, map[string]any{"capabilities": map[string]any{
		"semanticTokensProvider": map[string]any{
			"legend": map[string]any{"tokenTypes": []string{"type"}, "tokenModifiers": []string{}},
			"full":   map[string]any{"delta": true},
		},
		"executeCommandProvider": map[string]any{"commands": []string{"gopls.tidy"}},
	}})
	<-fake.gotNotif
	require.NoError(t, <-done)

	support := srv.SemanticTokens()
	assert.True(t, support.Full())
	assert.True(t, support.Delta())
	assert.False(t, support.Range())
	assert.True(t, srv.CanExecute("gopls.tidy"))
	assert.False(t, srv.CanExecute("editor.action.showReferences"))
}
