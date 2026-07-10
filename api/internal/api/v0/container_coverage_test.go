package v0

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// This file exercises the hub.Subscriber push wiring (PushProject...PushFile),
// the terminal lifecycle callbacks (onTerminalState/onTerminalEnded), and their
// shared resolveWorkspaceScope/scopeWsID helpers. These are otherwise only
// covered by the //go:build integration suite, which CI's coverage gate does
// not run (see container_test.go et al.), so container.go's push/lifecycle
// wiring shows 0% in the gate despite being exercised there. These tests are
// deliberately untagged so `go test ./...` (the CI invocation) exercises them
// directly, calling the unexported methods/helpers in-package rather than
// standing up the full Register() router.

// dialWSAt dials a WS client against srv at path and fails the test on error.
func dialWSAt(t *testing.T, srv *httptest.Server, path string) *websocket.Conn {
	t.Helper()
	url := "ws" + srv.URL[len("http"):] + path
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func readJSON(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(msg, &got))
	return got
}

func TestContainer_PushProject_ReachesClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := newAppForSnapshot(t)
	c := New(a, nil)
	r := gin.New()
	r.GET("/v0/projects/:projectId", func(ctx *gin.Context) { c.projects.Handle(ctx) })
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	conn := dialWSAt(t, srv, "/v0/projects/p1")
	c.projects.WaitRegistered()

	c.PushProject(dto.ProjectDTO{ID: "p1", Name: "keep"})

	got := readJSON(t, conn)
	assert.Equal(t, "p1", got["id"])
	assert.Equal(t, "keep", got["name"])
}

func TestContainer_PushRepo_ReachesClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := newAppForSnapshot(t)
	c := New(a, nil)
	r := gin.New()
	r.GET("/v0/projects/:projectId/repos", func(ctx *gin.Context) { c.repos.Handle(ctx) })
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	conn := dialWSAt(t, srv, "/v0/projects/p1/repos")
	c.repos.WaitRegistered()

	c.PushRepo(dto.RepoDTO{ID: "r1", ProjectID: "p1", Name: "keep"})

	got := readJSON(t, conn)
	assert.Equal(t, "r1", got["id"])
	assert.Equal(t, "p1", got["projectId"])
}

func TestContainer_PushWorkspace_ReachesClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := newAppForSnapshot(t)
	c := New(a, nil)
	r := gin.New()
	r.GET("/v0/projects/:projectId/repos/:repoId/workspaces", func(ctx *gin.Context) { c.workspaces.Handle(ctx) })
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	conn := dialWSAt(t, srv, "/v0/projects/p1/repos/r1/workspaces")
	c.workspaces.WaitRegistered()

	c.PushWorkspace(dto.WorkspaceDTO{ID: "w1", ProjectID: "p1", RepoID: "r1"})

	got := readJSON(t, conn)
	assert.Equal(t, "w1", got["id"])
}

func TestContainer_PushThread_ReachesClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := newAppForSnapshot(t)
	c := New(a, nil)
	r := gin.New()
	r.GET(
		"/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/threads",
		func(ctx *gin.Context) { c.threads.Handle(ctx) },
	)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	conn := dialWSAt(t, srv, "/v0/projects/p1/repos/r1/workspaces/w1/threads")
	c.threads.WaitRegistered()

	c.PushThread(dto.ThreadDTO{ID: "t1", ProjectID: "p1", RepoID: "r1", WorkspaceID: "w1"})

	got := readJSON(t, conn)
	assert.Equal(t, "t1", got["id"])
}

func TestContainer_PushTerminalSession_ReachesClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := newAppForSnapshot(t)
	c := New(a, nil)
	r := gin.New()
	r.GET(
		"/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/terminals",
		func(ctx *gin.Context) { c.terminals.Handle(ctx) },
	)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	conn := dialWSAt(t, srv, "/v0/projects/p1/repos/r1/workspaces/w1/terminals")
	c.terminals.WaitRegistered()

	c.PushTerminalSession(dto.TerminalSessionDTO{ID: "s1", ProjectID: "p1", RepoID: "r1", WorkspaceID: "w1"})

	got := readJSON(t, conn)
	assert.Equal(t, "s1", got["id"])
}

func TestContainer_PushGit_ReachesFilteredClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := newAppForSnapshot(t)
	c := New(a, nil)
	r := gin.New()
	r.GET(
		"/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/git/status",
		func(ctx *gin.Context) { c.git.Handle(ctx) },
	)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	conn := dialWSAt(t, srv, "/v0/projects/p1/repos/r1/workspaces/A/git/status")
	c.git.WaitRegistered()

	c.PushGit("B", gitdomain.GitStatus{Branch: "branch-B"})
	c.PushGit("A", gitdomain.GitStatus{Branch: "branch-A"})

	got := readJSON(t, conn)
	assert.Equal(t, "branch-A", got["branch"])
	// A clean tree's nil Files slice is normalised to an empty array, never null
	// (see PushGit's doc comment).
	files, ok := got["files"].([]any)
	require.True(t, ok, "files must serialise as an array, not null")
	assert.Empty(t, files)
}

func TestContainer_PushFile_ReachesFilteredClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := newAppForSnapshot(t)
	c := New(a, nil)
	r := gin.New()
	r.GET(
		"/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/files/ws",
		func(ctx *gin.Context) { c.files.Handle(ctx) },
	)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	conn := dialWSAt(t, srv, "/v0/projects/p1/repos/r1/workspaces/w1/files/ws")
	c.files.WaitRegistered()

	c.PushFile(domain.FileChangeEvent{WsID: "other", Path: "skip.go"})
	c.PushFile(domain.FileChangeEvent{WsID: "w1", Path: "a.go"})

	got := readJSON(t, conn)
	assert.Equal(t, "a.go", got["path"])
}

// TestContainer_PushAgentChat_ReachesFilteredClient proves PushAgentChat
// reaches only subscribers of the agent-chat WebSocket whose :wsId matches
// the frame's workspace (Task 3), mirroring
// TestContainer_PushGit_ReachesFilteredClient's proof shape for gitDef.
func TestContainer_PushAgentChat_ReachesFilteredClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := newAppForSnapshot(t)
	c := New(a, nil)
	r := gin.New()
	r.GET(
		"/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/agent/ws/chats",
		func(ctx *gin.Context) { c.agentChats.Handle(ctx) },
	)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	conn := dialWSAt(t, srv, "/v0/projects/p1/repos/r1/workspaces/A/agent/ws/chats")
	c.agentChats.WaitRegistered()

	c.PushAgentChat("chat-in-b", "B", "bound")
	c.PushAgentChat("chat-1", "A", "bound")

	got := readJSON(t, conn)
	assert.Equal(t, "chat-1", got["chatId"])
	assert.Equal(t, "A", got["workspaceId"])
	assert.Equal(t, "bound", got["kind"])
}

// TestScopeWsID_PrefersPathThenQuery proves scopeWsID reads the :wsId path
// param when present, falling back to the "wsId" query param, and returning ""
// when neither is set (T15's dual-served-route scoping).
func TestScopeWsID_PrefersPathThenQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name      string
		pathID    string
		query     string
		wantScope string
	}{
		{name: "path param wins", pathID: "from-path", query: "wsId=from-query", wantScope: "from-path"},
		{name: "falls back to query", pathID: "", query: "wsId=from-query", wantScope: "from-query"},
		{name: "empty when neither set", pathID: "", query: "", wantScope: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(w)
			req := httptest.NewRequest("GET", "/x?"+tt.query, nil)
			ctx.Request = req
			if tt.pathID != "" {
				ctx.Params = gin.Params{{Key: "wsId", Value: tt.pathID}}
			}
			assert.Equal(t, tt.wantScope, scopeWsID(ctx))
		})
	}
}

// TestResolveWorkspaceScope_NilApp proves resolveWorkspaceScope degrades to
// empty ids rather than panicking when the container has no app wired (e.g. a
// bare Container built in a test/degraded context).
func TestResolveWorkspaceScope_NilApp(t *testing.T) {
	c := &Container{}
	projectID, repoID := c.resolveWorkspaceScope(context.Background(), "w1")
	assert.Equal(t, "", projectID)
	assert.Equal(t, "", repoID)
}

// TestResolveWorkspaceScope_UnknownWorkspaceReturnsEmpty proves a workspace id
// that fails to resolve (Get error, e.g. not found) degrades to empty ids.
func TestResolveWorkspaceScope_UnknownWorkspaceReturnsEmpty(t *testing.T) {
	a := newAppForSnapshot(t)
	c := &Container{app: a}
	projectID, repoID := c.resolveWorkspaceScope(context.Background(), "does-not-exist")
	assert.Equal(t, "", projectID)
	assert.Equal(t, "", repoID)
}

// TestResolveWorkspaceScope_ResolvesProjectAndRepo proves a known workspace id
// resolves to its owning project and repo.
func TestResolveWorkspaceScope_ResolvesProjectAndRepo(t *testing.T) {
	a := newAppForSnapshot(t)
	_, err := a.Repositories.Workspace.Create(
		context.Background(),
		workspace.CreateInput{ID: "w1", ProjectID: "p1", RepoID: "r1"},
		time.Unix(1, 0).UTC(),
	)
	require.NoError(t, err)

	c := &Container{app: a}
	projectID, repoID := c.resolveWorkspaceScope(context.Background(), "w1")
	assert.Equal(t, "p1", projectID)
	assert.Equal(t, "r1", repoID)
}

// TestOnTerminalEnded_PushesEndedFrame proves the reap-path callback emits an
// "ended" lifecycle frame carrying the resolved project/repo scope, EndedAt,
// and (when known) the exit code.
func TestOnTerminalEnded_PushesEndedFrame(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := newAppForSnapshot(t)
	_, err := a.Repositories.Workspace.Create(
		context.Background(),
		workspace.CreateInput{ID: "w1", ProjectID: "p1", RepoID: "r1"},
		time.Unix(1, 0).UTC(),
	)
	require.NoError(t, err)

	c := New(a, nil)
	r := gin.New()
	r.GET(
		"/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/terminals",
		func(ctx *gin.Context) { c.terminals.Handle(ctx) },
	)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	conn := dialWSAt(t, srv, "/v0/projects/p1/repos/r1/workspaces/w1/terminals")
	c.terminals.WaitRegistered()

	c.onTerminalEnded(context.Background(), "w1", "s1", 7)

	got := readJSON(t, conn)
	assert.Equal(t, "s1", got["id"])
	assert.Equal(t, "w1", got["workspaceId"])
	assert.Equal(t, "p1", got["projectId"])
	assert.Equal(t, "r1", got["repoId"])
	assert.Equal(t, "ended", got["status"])
	assert.NotNil(t, got["endedAt"])
	assert.Equal(t, float64(7), got["exitCode"])
}

// TestOnTerminalEnded_UnknownExitCodeOmitted proves a negative (unknown) exit
// code is not stamped onto the frame.
func TestOnTerminalEnded_UnknownExitCodeOmitted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := newAppForSnapshot(t)
	_, err := a.Repositories.Workspace.Create(
		context.Background(),
		workspace.CreateInput{ID: "w1", ProjectID: "p1", RepoID: "r1"},
		time.Unix(1, 0).UTC(),
	)
	require.NoError(t, err)
	c := New(a, nil)
	r := gin.New()
	r.GET(
		"/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/terminals",
		func(ctx *gin.Context) { c.terminals.Handle(ctx) },
	)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	conn := dialWSAt(t, srv, "/v0/projects/p1/repos/r1/workspaces/w1/terminals")
	c.terminals.WaitRegistered()

	c.onTerminalEnded(context.Background(), "w1", "s1", -1)

	got := readJSON(t, conn)
	assert.Equal(t, "ended", got["status"])
	_, hasExitCode := got["exitCode"]
	assert.False(t, hasExitCode, "unknown (negative) exit code must be omitted from the frame")
}

// TestOnTerminalState_PushesStateFrame proves the detach/suspend transition
// callback emits a lifecycle frame carrying the resolved project/repo scope and
// the given state.
func TestOnTerminalState_PushesStateFrame(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := newAppForSnapshot(t)
	_, err := a.Repositories.Workspace.Create(
		context.Background(),
		workspace.CreateInput{ID: "w1", ProjectID: "p1", RepoID: "r1"},
		time.Unix(1, 0).UTC(),
	)
	require.NoError(t, err)

	c := New(a, nil)
	r := gin.New()
	r.GET(
		"/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/terminals",
		func(ctx *gin.Context) { c.terminals.Handle(ctx) },
	)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	conn := dialWSAt(t, srv, "/v0/projects/p1/repos/r1/workspaces/w1/terminals")
	c.terminals.WaitRegistered()

	c.onTerminalState(context.Background(), "w1", "s1", "detached")

	got := readJSON(t, conn)
	assert.Equal(t, "s1", got["id"])
	assert.Equal(t, "p1", got["projectId"])
	assert.Equal(t, "r1", got["repoId"])
	assert.Equal(t, "detached", got["status"])
}
