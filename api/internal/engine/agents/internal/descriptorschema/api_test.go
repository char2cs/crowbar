package descriptorschema_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const socketAPI = "id: x\nruntime:\n  api:\n    protocol: jsonrpc2\n" +
	"    serve: [cli, app-server, --listen, \"unix://{socket}\"]\n" +
	"    handshake: { call: initialize }\n    session_lost_codes: [-32600, -1]\n"

// A serve that listened on {socket} speaks stdio now, with the handshake
// Crowbar used to send unasked and the session-lost code where it lives today.
func TestUpgrade_ASocketServeBecomesAStdioServe(t *testing.T) {
	up := upgrade(t, socketAPI+"session:\n  resume: { arg: \"resume {id}\" }\n")
	doc := decode(t, up.Raw)

	api := sub(t, sub(t, doc, "runtime"), "api")
	assert.Equal(t, []any{"cli", "app-server"}, api["serve"])
	assert.Equal(t, "stdio", api["delivery"])
	assert.Equal(t, "ndjson", api["format"])
	assert.NotContains(t, api, "session_lost_codes")
	assert.Equal(t, map[string]any{
		"call":   "initialize",
		"params": map[string]any{"clientInfo": map[string]any{"name": "crowbar", "title": "Crowbar", "version": "0.0.0"}},
		"notify": "initialized",
	}, api["handshake"])

	session := sub(t, doc, "session")
	assert.Equal(t, map[string]any{"rpc_code": -32600}, session["refused"])
	assert.Equal(t, "turn_start", session["persists"])

	notes := map[string]string{}
	for _, c := range up.Changes {
		notes[c.Path] = c.Note
	}
	assert.Contains(t, notes["runtime.api.serve"], `"--listen unix://{socket}"`)
	assert.Contains(t, notes["runtime.api.session_lost_codes"], "the other 1 were dropped")
	assert.Contains(t, notes, "runtime.api.handshake")
}

// A serve that already states its delivery is today's shape: its handshake is
// what its author wrote, nothing is added to it.
func TestUpgrade_AStdioServeKeepsItsOwnHandshake(t *testing.T) {
	in := "id: x\nruntime:\n  api:\n    delivery: stdio\n    format: ndjson\n    serve: [cli]\n    handshake: { call: hello }\n"
	up := upgrade(t, in)
	assert.Empty(t, up.Changes)
	assert.Equal(t, in, string(up.Raw))
}

// A refusal the document already declares wins over the retired code list.
func TestUpgrade_TheSessionLostCodeNeverOverwritesADeclaredRefusal(t *testing.T) {
	up := upgrade(t, socketAPI+"session:\n  refused: { exit_code: 1, rpc_code: 7, needles: [gone] }\n")
	assert.Equal(t, 7, sub(t, sub(t, decode(t, up.Raw), "session"), "refused")["rpc_code"])
	for _, c := range up.Changes {
		if c.Path == "runtime.api.session_lost_codes" {
			require.Contains(t, c.Note, "already says it")
		}
	}
}

func sub(t *testing.T, m map[string]any, key string) map[string]any {
	t.Helper()
	v, ok := m[key].(map[string]any)
	require.True(t, ok, "%s is a mapping in %v", key, m)
	return v
}
