//go:build integration

// Package tests holds the end-to-end integration suite for the Crowbar /v0 API.
// Each test boots the REAL wired backend (engine -> adapter -> app -> api) over
// an httptest.Server and drives the DoD walkthrough at the HTTP and WebSocket
// level. The frontend is stubbed, so this suite is the authoritative end-to-end
// verification of the backend.
package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	crowbarapi "github.com/char2cs/crowbar/api/internal/api"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/engine"
)

// harness owns the booted server and its base URL. It exposes typed helpers for
// the REST envelope and the WebSocket topics so individual suites stay terse.
type harness struct {
	t      *testing.T
	server *httptest.Server
	url    string
	home   string
}

// newHarness boots the full backend over an httptest.Server rooted at a
// per-test temp home dir. A stub fs.FS stands in for the unbuilt web/dist embed.
func newHarness(
	t *testing.T,
) *harness {
	t.Helper()
	ctx := context.Background()
	tmp := t.TempDir()

	engines, err := engine.New(ctx, engine.WithHomeDir(tmp))
	require.NoError(t, err)

	adapters, err := adapter.New(adapter.WithHomeDir(tmp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	appContainer, err := app.New(ctx, engines, adapters)
	require.NoError(t, err)

	apiContainer, err := crowbarapi.New(appContainer, engines, fstest.MapFS{})
	require.NoError(t, err)

	srv := httptest.NewServer(apiContainer.Handler())
	t.Cleanup(srv.Close)
	t.Cleanup(appContainer.Close)

	return &harness{
		t:      t,
		server: srv,
		url:    srv.URL,
		home:   tmp,
	}
}

// get issues GET path, asserts the success envelope, and decodes data into out.
func (h *harness) get(
	path string,
	out any,
) {
	h.do(http.MethodGet, path, nil, http.StatusOK, out)
}

// post issues POST path with a JSON body and decodes the data envelope into out
// asserting the given status.
func (h *harness) post(
	path string,
	body any,
	wantStatus int,
	out any,
) {
	h.do(http.MethodPost, path, body, wantStatus, out)
}

// put issues PUT path with a JSON body and decodes the data envelope into out.
func (h *harness) put(
	path string,
	body any,
	out any,
) {
	h.do(http.MethodPut, path, body, http.StatusOK, out)
}

// patch issues PATCH path with a JSON body and decodes the data envelope.
func (h *harness) patch(
	path string,
	body any,
	out any,
) {
	h.do(http.MethodPatch, path, body, http.StatusOK, out)
}

// del issues DELETE path with an optional JSON body and asserts the status. It
// decodes the data envelope into out when wantStatus is not 204.
func (h *harness) del(
	path string,
	body any,
	wantStatus int,
	out any,
) {
	if wantStatus == http.StatusNoContent {
		resp := h.raw(http.MethodDelete, path, body, wantStatus)
		_ = resp.Body.Close()
		return
	}
	h.do(http.MethodDelete, path, body, wantStatus, out)
}

// postError issues POST path with a JSON body, asserts the HTTP status, and
// requires the error envelope (success=false with a non-empty message). It is
// the counterpart to post for the rejection paths (e.g. a locked-workspace 409).
func (h *harness) postError(
	path string,
	body any,
	wantStatus int,
) {
	h.t.Helper()
	resp := h.raw(http.MethodPost, path, body, wantStatus)
	defer func() { _ = resp.Body.Close() }()

	var env struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	require.NoError(h.t, json.NewDecoder(resp.Body).Decode(&env))
	require.False(h.t, env.Success, "expected error envelope for POST %s", path)
	require.NotEmpty(h.t, env.Error, "error envelope must carry a message")
}

func (h *harness) do(
	method string,
	path string,
	body any,
	wantStatus int,
	out any,
) {
	h.t.Helper()
	resp := h.raw(method, path, body, wantStatus)
	defer func() { _ = resp.Body.Close() }()

	var env struct {
		Success bool            `json:"success"`
		Error   string          `json:"error"`
		Data    json.RawMessage `json:"data"`
	}
	require.NoError(h.t, json.NewDecoder(resp.Body).Decode(&env))
	require.True(h.t, env.Success, "envelope error for %s %s: %s", method, path, env.Error)
	if out != nil {
		require.NoError(h.t, json.Unmarshal(env.Data, out))
	}
}

func (h *harness) raw(
	method string,
	path string,
	body any,
	wantStatus int,
) *http.Response {
	h.t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		require.NoError(h.t, err)
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequest(method, h.url+path, reader)
	require.NoError(h.t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := h.server.Client().Do(req)
	require.NoError(h.t, err)
	require.Equal(h.t, wantStatus, resp.StatusCode, "%s %s", method, path)
	return resp
}

// dial opens a WebSocket to the given /v0 path and registers cleanup.
func (h *harness) dial(
	path string,
) *websocket.Conn {
	h.t.Helper()
	wsURL := "ws" + h.url[len("http"):] + path
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.NoError(h.t, err)
	h.t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// readUntil loop-reads JSON frames under a deadline, skipping control frames,
// until match returns true for a decoded frame. It never sleeps: it advances on
// real frames and fails the test if the deadline elapses first.
func readUntil(
	t *testing.T,
	conn *websocket.Conn,
	match func(map[string]any) bool,
) map[string]any {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	_ = conn.SetReadDeadline(deadline)
	for {
		mt, raw, err := conn.ReadMessage()
		require.NoError(t, err)
		if mt != websocket.TextMessage {
			continue
		}
		var got map[string]any
		if json.Unmarshal(raw, &got) != nil {
			continue
		}
		if match(got) {
			return got
		}
		require.True(t, time.Now().Before(deadline), "deadline exceeded before match")
	}
}

// gitRepoWithCommit creates a real on-disk git repo with one committed file and
// returns its path. The repo backs a workspace so git/file/watcher flows are
// real rather than mocked.
func gitRepoWithCommit(
	t *testing.T,
) string {
	t.Helper()
	dir := t.TempDir()
	commands := [][]string{
		{"init"},
		{"config", "user.email", "t@t.dev"},
		{"config", "user.name", "t"},
		{"checkout", "-b", "main"},
	}
	for _, args := range commands {
		runGit(t, dir, args...)
	}
	require.NoError(t, writeFile(dir, "README.md", "hello\n"))
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "initial")
	return dir
}

func runGit(
	t *testing.T,
	dir string,
	args ...string,
) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
}
