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

// recorder canned-answers a sequence of 200 responses; a case that needs a
// different status builds its own post func inline instead.
func recorder(responses ...string) (*[]postCall, func(string, any) (int, []byte, error)) {
	calls := &[]postCall{}
	i := 0
	return calls, func(path string, body any) (int, []byte, error) {
		raw, _ := json.Marshal(body)
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		*calls = append(*calls, postCall{path: path, body: m})
		if i >= len(responses) {
			return 200, []byte(`{"success":true,"data":{}}`), nil
		}
		r := responses[i]
		i++
		return 200, []byte(r), nil
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
	require.Equal(t, "/v0/projects/P/repos/R/workspaces/W/chats/runners/SEG/mcp", (*calls)[0].path)
	require.Equal(t, "TOK", (*calls)[0].body["token"])

	// Exactly one line out, and it is the unwrapped rpc object.
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	require.Len(t, lines, 1)
	require.JSONEq(t, `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-11-25"}}`, lines[0])
}

// A 2xx with no rpc field is the daemon's 204: the relay must stay silent,
// because replying to a notification confuses the client. recorder returns 200
// for every call, so an empty body here is genuinely the no-rpc/notification
// case, not the non-2xx case covered separately below.
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
	require.Equal(t, "/v0/projects/P/home/chats/runners/SEG/mcp", (*calls)[0].path)
}

// A daemon that is down must not kill the CLI session: the relay reports the
// failure as a JSON-RPC error and keeps going.
func TestRelay_TransportFailureBecomesAnRPCError(t *testing.T) {
	var out bytes.Buffer
	post := func(string, any) (int, []byte, error) { return 0, nil, errBoom }
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

// A daemon application error must not be answered with silence — that hangs the
// client on a request it is entitled to a reply for. PostJSON returns err == nil
// for a non-2xx, and that body carries no rpc field either, so the relay must
// tell the two apart by status code rather than by shape alone.
func TestRelay_NonSuccessStatusBecomesAnRPCError(t *testing.T) {
	var out bytes.Buffer
	post := func(string, any) (int, []byte, error) {
		return 500, []byte(`{"success":false,"error":"tool surface not configured"}`), nil
	}
	in := strings.NewReader(`{"jsonrpc":"2.0","id":7,"method":"tools/list"}` + "\n")

	require.NoError(t, runMCPRelay(in, &out, post, "SEG", "P", "R", "W", "TOK"))

	var resp struct {
		ID    int `json:"id"`
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out.String())), &resp))
	require.Equal(t, 7, resp.ID)
	require.Equal(t, -32603, resp.Error.Code)
	require.Contains(t, resp.Error.Message, "500")
}

// A notification whose POST FAILS must still be answered with silence.
// TestRelay_WritesNothingForANotification above covers only the 200 path, so the
// error path shipped untested — and it is the one that matters most, because
// notifications/initialized is sent during the handshake, the likeliest moment
// for a transient daemon failure. JSON-RPC 2.0 forbids any response to a
// notification; an unsolicited {"id":null,"error":…} is not a courtesy.
func TestRelay_WritesNothingWhenANotificationsPostFails(t *testing.T) {
	var out bytes.Buffer
	post := func(string, any) (int, []byte, error) { return 0, nil, errBoom }
	in := strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n")

	require.NoError(t, runMCPRelay(in, &out, post, "SEG", "P", "R", "W", "TOK"))
	require.Empty(t, out.String(), "a notification gets no reply, not even an error")
}

// The same holds for the OTHER failure site: a non-2xx is an application error,
// but a notification is still not entitled to hear about it.
func TestRelay_WritesNothingWhenANotificationGetsANonSuccessStatus(t *testing.T) {
	var out bytes.Buffer
	post := func(string, any) (int, []byte, error) {
		return 500, []byte(`{"success":false,"error":"tool surface not configured"}`), nil
	}
	in := strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n")

	require.NoError(t, runMCPRelay(in, &out, post, "SEG", "P", "R", "W", "TOK"))
	require.Empty(t, out.String())
}

// The counterpart, and the reason the silence above is decided by PARSING rather
// than by "no id field found": a line too malformed to parse is not a
// notification — its id could not be DETERMINED, which is exactly the case
// JSON-RPC 2.0 allows "id":null for. Swallowing it would hang a client that is
// waiting for a reply it is entitled to.
func TestRelay_MalformedLineStillGetsANullIDError(t *testing.T) {
	var out bytes.Buffer
	post := func(string, any) (int, []byte, error) { return 0, nil, errBoom }
	in := strings.NewReader(`{"jsonrpc":"2.0","id` + "\n")

	require.NoError(t, runMCPRelay(in, &out, post, "SEG", "P", "R", "W", "TOK"))
	require.JSONEq(t,
		`{"jsonrpc":"2.0","id":null,"error":{"code":-32603,"message":"crowbar daemon unreachable: daemon down"}}`,
		strings.TrimSpace(out.String()))
}

var errBoom = errors.New("daemon down")
