//go:build integration

package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	crowbarapi "github.com/char2cs/crowbar/api/internal/api"
	v0 "github.com/char2cs/crowbar/api/internal/api/v0"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/engine"
	"github.com/char2cs/crowbar/api/internal/testutil/accessfake"
	"github.com/char2cs/crowbar/api/tests/kit"
)

type harness struct {
	t         *testing.T
	server    *httptest.Server
	handler   http.Handler
	url       string
	home      string
	app       *app.Container
	eng       *engine.Container
	stopOnce  sync.Once
	stop      func()
	crashDie  func()
	drainOnce sync.Once
	drainDown func()
}

func newHarness(
	t *testing.T,
) *harness {
	t.Helper()
	return newHarnessAt(t, t.TempDir())
}

func newHarnessAt(
	t *testing.T,
	home string,
) *harness {
	t.Helper()
	return bootHarness(t, home, "")
}

func bootHarness(t *testing.T, home, fingerprint string) *harness {
	t.Helper()
	ctx, cancelBoot := context.WithCancel(context.Background())

	session, err := kit.BootHome(ctx, home)
	releaseHome := func() { require.NoError(t, session.Close()) }
	if session != nil {
		t.Cleanup(releaseHome)
	}
	require.NoError(t, err)

	engines, err := engine.New(ctx, engine.WithHomeDir(home))
	require.NoError(t, err)

	adapters, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)

	appContainer, err := app.New(ctx, engines, adapters, session, v0.EncodeEntity, v0.NewSidebar)
	require.NoError(t, err)

	access := accessfake.Closed()
	if fingerprint != "" {
		access = accessfake.Listening(fingerprint)
	}
	apiContainer, err := crowbarapi.New(appContainer, engines, fstest.MapFS{}, access, time.Minute)
	require.NoError(t, err)

	srv := httptest.NewServer(apiContainer.Handler())

	h := &harness{
		t:       t,
		home:    home,
		app:     appContainer,
		eng:     engines,
		handler: apiContainer.Handler(),
	}
	h.server = srv
	h.url = srv.URL
	h.drainDown = func() {
		cancelBoot()
		srv.Close()
		_ = appContainer.Shutdown(context.Background())
	}
	h.stop = func() {
		h.drain()
		appContainer.Close()
		engines.Close()
		_ = adapters.Close()
		releaseHome()
	}
	h.crashDie = func() {
		cancelBoot()
		srv.Close()
		_ = adapters.Close()
		appContainer.Close()
		engines.Close()
		releaseHome()
	}
	t.Cleanup(h.shutdown)

	return h
}

func (h *harness) drain() {
	h.drainOnce.Do(h.drainDown)
}

func (h *harness) shutdown() {
	h.stopOnce.Do(h.stop)
}

func (h *harness) crash() {
	h.stopOnce.Do(h.crashDie)
}

func (h *harness) Quiesce() {
	h.app.Repositories.WaitQuiescent()
}

func (h *harness) QuiesceReactors() {
	h.app.Repositories.QuiesceReactors(context.Background())
}

func (h *harness) get(
	path string,
	out any,
) {
	h.do(http.MethodGet, path, nil, http.StatusOK, out)
}

func (h *harness) post(
	path string,
	body any,
	wantStatus int,
	out any,
) {
	h.do(http.MethodPost, path, body, wantStatus, out)
}

func (h *harness) put(
	path string,
	body any,
	out any,
) {
	h.do(http.MethodPut, path, body, http.StatusOK, out)
}

func (h *harness) patch(
	path string,
	body any,
	out any,
) {
	h.do(http.MethodPatch, path, body, http.StatusOK, out)
}

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

func (h *harness) postError(
	path string,
	body any,
	wantStatus int,
) {
	h.t.Helper()
	h.mutationError(http.MethodPost, path, body, wantStatus)
}

func (h *harness) mutationError(
	method string,
	path string,
	body any,
	wantStatus int,
) string {
	h.t.Helper()
	resp := h.raw(method, path, body, wantStatus)
	defer func() { _ = resp.Body.Close() }()

	var env struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	require.NoError(h.t, json.NewDecoder(resp.Body).Decode(&env))
	require.False(h.t, env.Success, "expected error envelope for %s %s", method, path)
	require.NotEmpty(h.t, env.Error, "error envelope must carry a message")
	return env.Error
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

func readUntil(
	t *testing.T,
	conn *websocket.Conn,
	match func(map[string]any) bool,
) map[string]any {
	t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	for {
		require.NoError(t, conn.SetReadDeadline(deadline))
		mt, raw, err := conn.ReadMessage()
		require.NoError(t, err, "no matching frame within 30s — it was most "+
			"likely broadcast before this connection subscribed")
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
	}
}

const sentinelFile = "crowbar-sentinel.txt"

func statusHasFile(
	frame map[string]any,
	name string,
) bool {
	files, ok := frame["files"].([]any)
	if !ok {
		return false
	}
	for _, f := range files {
		entry, ok := f.(map[string]any)
		if ok && entry["path"] == name {
			return true
		}
	}
	return false
}

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
