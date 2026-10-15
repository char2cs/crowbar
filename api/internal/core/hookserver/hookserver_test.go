package hookserver_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/hookserver"
)

const hookPath = "/v0/projects/p1/repos/r1/chats/runners/seg-1/hooks/claude/tool_pre"

func gateStatus(
	t *testing.T,
	method, path string,
) (status int, reached bool) {
	t.Helper()
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	hookserver.Gate(next).ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec.Code, reached
}

func TestGate_AllowsOnlyTheRunnerHookRoute(t *testing.T) {
	for _, path := range []string{
		hookPath,
		"/v0/projects/p1/home/chats/runners/seg-1/hooks/codex/session_start",
	} {
		status, reached := gateStatus(t, http.MethodPost, path)
		assert.Equal(t, http.StatusOK, status, path)
		assert.True(t, reached, path)
	}
}

func TestGate_RefusesEverythingElse(t *testing.T) {
	cases := []struct{ method, path string }{
		{http.MethodGet, hookPath},
		{http.MethodDelete, hookPath},
		{http.MethodPost, "/v0/projects/p1/repos/r1/chats"},
		{http.MethodPost, "/v0/projects/p1/repos/r1/chats/hooks"},
		{http.MethodPost, "/v0/projects/p1/repos/r1/chats/runners/seg-1/mcp"},
		{http.MethodGet, "/v0/projects"},
		{http.MethodGet, "/v0/console/logs"},
		{http.MethodPost, hookPath + "/extra"},
		{http.MethodPost, "/v0/projects/p1/repos/r1/chats/runners//hooks/claude/tool_pre"},
		{http.MethodPost, "/v0/projects/p1/repos/r1/chats/runners/seg-1/hooks/claude"},
		{http.MethodPost, "/anything"},
	}
	for _, tc := range cases {
		status, reached := gateStatus(t, tc.method, tc.path)
		assert.Equal(t, http.StatusNotFound, status, "%s %s", tc.method, tc.path)
		assert.False(t, reached, "%s %s must never reach the router", tc.method, tc.path)
	}
}

func TestListen_BindsLoopbackOnly(t *testing.T) {
	s, err := hookserver.Listen(http.NotFoundHandler())
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })

	host, _, err := net.SplitHostPort(strings.TrimPrefix(s.URL(), "http://"))
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1", host)
}

func TestServe_AnswersTheHookRouteAndNothingElse(t *testing.T) {
	s, err := hookserver.Listen(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "routed")
	}))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	go func() { _ = s.Serve() }()

	resp, err := http.Post(s.URL()+hookPath, "application/json", strings.NewReader("{}"))
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	assert.Equal(t, "routed", string(body))

	resp, err = http.Get(s.URL() + "/v0/projects")
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestShutdown_StopsServing(t *testing.T) {
	s, err := hookserver.Listen(http.NotFoundHandler())
	require.NoError(t, err)
	served := make(chan error, 1)
	go func() { served <- s.Serve() }()

	require.NoError(t, s.Shutdown(context.Background()))

	assert.ErrorIs(t, <-served, http.ErrServerClosed)
}

func TestPublish_ThenRead_RoundTripsThePrivateEndpoint(t *testing.T) {
	home := t.TempDir()

	require.NoError(t, hookserver.Publish(home, "http://127.0.0.1:4242"))

	got, err := hookserver.Read(home)
	require.NoError(t, err)
	assert.Equal(t, "http://127.0.0.1:4242", got)
	info, err := os.Stat(filepath.Join(home, "run", "hooks.url"))
	require.NoError(t, err)
	assert.Zero(t, info.Mode().Perm()&0o077, "the endpoint file is for the owner only")
}

func TestRead_WithoutAPublishedEndpointFails(t *testing.T) {
	_, err := hookserver.Read(t.TempDir())

	assert.Error(t, err)
}
