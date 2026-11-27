package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type postCall struct {
	path string
	body map[string]any
}

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

func testToken() (string, error) { return "TOK", nil }

func TestRelay_APlacementFreeRouteForARelayID(t *testing.T) {
	calls, post := recorder()
	var out bytes.Buffer
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n")

	require.NoError(t, runMCPRelay(in, &out, post, mcpPath("RELAY", "", "", "", ""), testToken))
	require.Equal(t, "/v0/relays/RELAY/mcp", (*calls)[0].path)
}

func TestRelay_ReadsTheTokenFileForEveryMessage(t *testing.T) {
	tokens := make(chan any, 2)
	post := func(_ string, body any) (int, []byte, error) {
		payload, ok := body.(map[string]any)
		if !ok {
			return 0, nil, errors.New("the relay posted a body that is not an object")
		}
		tokens <- payload["token"]
		return 200, []byte(`{"success":true,"data":{}}`), nil
	}
	tokenFile := filepath.Join(t.TempDir(), "relay.token")
	require.NoError(t, os.WriteFile(tokenFile, []byte("BOOT-1\n"), 0o600))
	ping := `{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n"
	var out bytes.Buffer
	in, feed := io.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- runMCPRelay(in, &out, post, mcpPath("RELAY", "", "", "", ""), func() (string, error) {
			return readRunnerToken(tokenFile)
		})
	}()

	_, err := feed.Write([]byte(ping))
	require.NoError(t, err)
	require.Equal(t, "BOOT-1", <-tokens)
	require.NoError(t, os.WriteFile(tokenFile, []byte("BOOT-2\n"), 0o600))
	_, err = feed.Write([]byte(ping))
	require.NoError(t, err)
	require.Equal(t, "BOOT-2", <-tokens)
	require.NoError(t, feed.Close())
	require.NoError(t, <-done)
}

func TestRelay_AnUnreadableTokenFileBecomesAnRPCError(t *testing.T) {
	calls, post := recorder()
	var out bytes.Buffer
	in := strings.NewReader(`{"jsonrpc":"2.0","id":3,"method":"ping"}` + "\n")
	missing := filepath.Join(t.TempDir(), "gone.token")

	require.NoError(t, runMCPRelay(in, &out, post, "/p", func() (string, error) { return readRunnerToken(missing) }))

	require.Empty(t, *calls, "nothing is posted without a token")
	require.Contains(t, out.String(), `"id":3`)
	require.Contains(t, out.String(), "read the runner token")
}

func TestRelay_ForwardsEachLineAndWritesTheReply(t *testing.T) {
	calls, post := recorder(
		`{"success":true,"data":{"rpc":{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-11-25"}}}}`,
	)
	var out bytes.Buffer
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n")

	require.NoError(t, runMCPRelay(in, &out, post, mcpPath("", "SEG", "P", "R", "W"), testToken))

	require.Len(t, *calls, 1)
	require.Equal(t, "/v0/projects/P/repos/R/chats/runners/SEG/mcp", (*calls)[0].path)
	require.Equal(t, "TOK", (*calls)[0].body["token"])

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	require.Len(t, lines, 1)
	require.JSONEq(t, `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-11-25"}}`, lines[0])
}

func TestRelay_WritesNothingForANotification(t *testing.T) {
	_, post := recorder(``)
	var out bytes.Buffer
	in := strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n")

	require.NoError(t, runMCPRelay(in, &out, post, mcpPath("", "SEG", "P", "R", "W"), testToken))
	require.Empty(t, out.String())
}

func TestRelay_SkipsBlankLines(t *testing.T) {
	calls, post := recorder()
	var out bytes.Buffer
	in := strings.NewReader("\n   \n" + `{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n")

	require.NoError(t, runMCPRelay(in, &out, post, mcpPath("", "SEG", "P", "R", "W"), testToken))
	require.Len(t, *calls, 1)
}

func TestRelay_UsesHomePathWhenRepoIsEmpty(t *testing.T) {
	calls, post := recorder()
	var out bytes.Buffer
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n")

	require.NoError(t, runMCPRelay(in, &out, post, mcpPath("", "SEG", "P", "", "W"), testToken))
	require.Equal(t, "/v0/projects/P/home/chats/runners/SEG/mcp", (*calls)[0].path)
}

func TestRelay_TransportFailureBecomesAnRPCError(t *testing.T) {
	var out bytes.Buffer
	post := func(string, any) (int, []byte, error) { return 0, nil, errBoom }
	in := strings.NewReader(`{"jsonrpc":"2.0","id":9,"method":"tools/list"}` + "\n")

	require.NoError(t, runMCPRelay(in, &out, post, mcpPath("", "SEG", "P", "R", "W"), testToken))

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

func TestRelay_NonSuccessStatusBecomesAnRPCError(t *testing.T) {
	var out bytes.Buffer
	post := func(string, any) (int, []byte, error) {
		return 500, []byte(`{"success":false,"error":"tool surface not configured"}`), nil
	}
	in := strings.NewReader(`{"jsonrpc":"2.0","id":7,"method":"tools/list"}` + "\n")

	require.NoError(t, runMCPRelay(in, &out, post, mcpPath("", "SEG", "P", "R", "W"), testToken))

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

func TestRelay_WritesNothingWhenANotificationsPostFails(t *testing.T) {
	var out bytes.Buffer
	post := func(string, any) (int, []byte, error) { return 0, nil, errBoom }
	in := strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n")

	require.NoError(t, runMCPRelay(in, &out, post, mcpPath("", "SEG", "P", "R", "W"), testToken))
	require.Empty(t, out.String(), "a notification gets no reply, not even an error")
}

func TestRelay_WritesNothingWhenANotificationGetsANonSuccessStatus(t *testing.T) {
	var out bytes.Buffer
	post := func(string, any) (int, []byte, error) {
		return 500, []byte(`{"success":false,"error":"tool surface not configured"}`), nil
	}
	in := strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n")

	require.NoError(t, runMCPRelay(in, &out, post, mcpPath("", "SEG", "P", "R", "W"), testToken))
	require.Empty(t, out.String())
}

func TestRelay_MalformedLineStillGetsANullIDError(t *testing.T) {
	var out bytes.Buffer
	post := func(string, any) (int, []byte, error) { return 0, nil, errBoom }
	in := strings.NewReader(`{"jsonrpc":"2.0","id` + "\n")

	require.NoError(t, runMCPRelay(in, &out, post, mcpPath("", "SEG", "P", "R", "W"), testToken))
	require.JSONEq(t,
		`{"jsonrpc":"2.0","id":null,"error":{"code":-32603,"message":"crowbar daemon unreachable: daemon down"}}`,
		strings.TrimSpace(out.String()))
}

var errBoom = errors.New("daemon down")

func TestMCP_RelaysToADaemonOnTCP(t *testing.T) {
	for name, tc := range map[string]struct {
		flags []string
		path  string
	}{
		"a runner": {
			[]string{"--segment", "SEG", "--project", "P", "--repo", "R"},
			"/v0/projects/P/repos/R/chats/runners/SEG/mcp",
		},
		"a session's relay": {[]string{"--relay", "RELAY"}, "/v0/relays/RELAY/mcp"},
	} {
		t.Run(name, func(t *testing.T) {
			ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
			require.NoError(t, err)
			reached := make(chan string, 1)
			tokens := make(chan string, 1)
			srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Token string `json:"token"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				reached <- r.URL.Path
				tokens <- body.Token
				_, _ = w.Write([]byte(`{"success":true,"data":{"rpc":{"jsonrpc":"2.0","id":1,"result":{}}}}`))
			})}
			go func() { _ = srv.Serve(ln) }()
			t.Cleanup(func() { _ = srv.Close() })

			tokenFile := filepath.Join(t.TempDir(), "mcp.token")
			require.NoError(t, os.WriteFile(tokenFile, []byte("TOK\n"), 0o600))
			root := newRootCmd()
			var out bytes.Buffer
			root.SetIn(strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n"))
			root.SetOut(&out)
			root.SetArgs(append([]string{"mcp", "--host", "tcp://" + ln.Addr().String(), "--token-file", tokenFile}, tc.flags...))
			require.NoError(t, root.Execute())

			select {
			case path := <-reached:
				require.Equal(t, tc.path, path)
				require.Equal(t, "TOK", <-tokens, "the relay authenticates with the token the spawn wrote")
			default:
				t.Fatalf("the relay never reached the daemon on %s: %s", ln.Addr(), out.String())
			}
			require.JSONEq(t, `{"jsonrpc":"2.0","id":1,"result":{}}`, strings.TrimSpace(out.String()))
		})
	}
}
