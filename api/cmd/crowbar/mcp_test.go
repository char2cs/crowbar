package main

import (
	"bytes"
	"encoding/json"
	"errors"
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
