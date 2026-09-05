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
	"github.com/char2cs/crowbar/api/internal/app/apperr"
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

// readJSON blocks until the next frame arrives, then decodes it. No read
// deadline: the frame's arrival IS the signal, and a frame that never comes
// hangs until `go test -timeout` fires and names this test — strictly better
// than a two-second guess that reddens under load.
func readJSON(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
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

// TestContainer_PushWorkspace_ReachesChatClient proves PushWorkspace fans a
// workspace's state onto the CHAT topic — the only topic it serves now — keyed
// on the chat that owns the worktree.
//
// The orphan pushed FIRST is the other half of the assertion: a workspace with
// no owning chat has no chat for a client to draw it on and pushes nothing at
// all, so it must not be this client's first frame.
func TestContainer_PushWorkspace_ReachesChatClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := newAppForSnapshot(t)
	c := New(a, nil)
	r := gin.New()
	r.GET("/v0/chats/:chatId/ws", func(ctx *gin.Context) { c.agentChats.Handle(ctx) })
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	conn := dialWSAt(t, srv, "/v0/chats/chat-1/ws")
	c.agentChats.WaitRegistered()

	c.PushWorkspace(dto.WorkspaceDTO{ID: "orphan", ProjectID: "p1", RepoID: "r1"})
	c.PushWorkspace(dto.WorkspaceDTO{ID: "w1", ProjectID: "p1", RepoID: "r1", OwningChatID: "chat-1"})

	got := readJSON(t, conn)
	assert.Equal(t, dto.AgentChatKindWorktreeState, got["kind"])
	assert.Equal(t, "chat-1", got["chatId"])
	assert.Equal(t, "w1", got["workspaceId"])
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
		"/v0/chats/:chatId/terminals",
		func(ctx *gin.Context) { c.terminals.Handle(ctx) },
	)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	conn := dialWSAt(t, srv, "/v0/chats/c1/terminals")
	c.terminals.WaitRegistered()

	c.PushTerminalSession(dto.TerminalSessionDTO{ID: "s1", ChatID: "c1"})

	got := readJSON(t, conn)
	assert.Equal(t, "s1", got["id"])
}

// stubChatsHolding is the minimal usecases.WorktreeResolver PushGit's fan-out
// (chatsHolding) needs: ChatsForWorkspace answers a fixed roster per
// workspace id, and Resolve is never called from this path so it degrades to
// not-found.
type stubChatsHolding map[string][]string

func (s stubChatsHolding) Resolve(
	_ context.Context,
	_ string,
) (domain.Workspace, error) {
	return domain.Workspace{}, apperr.ErrNotFound
}

func (s stubChatsHolding) ChatsForWorkspace(
	_ context.Context,
	workspaceID string,
) ([]string, error) {
	return s[workspaceID], nil
}

// TestContainer_PushGit_ReachesFilteredClient proves PushGit reaches only a
// subscriber whose :chatId is among the fan-out set it resolves for the
// pushed workspace — gitDef's chatId filter is a Required membership match
// (spec §8 step 6: the old :wsId-bound route this test used to dial is gone,
// and so is its non-required wsId filter), so the route here binds :chatId
// like the real /v0/chats/:chatId/git/status mount does.
func TestContainer_PushGit_ReachesFilteredClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := newAppForSnapshot(t)
	a.Usecases.Worktree = stubChatsHolding{"A": {"chat-a"}, "B": {"chat-b"}}
	c := New(a, nil)
	r := gin.New()
	r.GET(
		"/v0/chats/:chatId/git/status",
		func(ctx *gin.Context) { c.git.Handle(ctx) },
	)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	conn := dialWSAt(t, srv, "/v0/chats/chat-a/git/status")
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
// reaches only subscribers whose subscription the frame's namespace falls
// under, at the mount the routes actually carry today: the REPO-scoped
// .../repos/:repoId/chats/ws, which binds no :wsId at all.
//
// It used to dial a workspace-scoped chat route Task 17 retired, and to lean on
// agentChatDef's wsId Filter for the scoping. That Filter resolves inactive
// where there is no :wsId, which is exactly why the stream has a repoId Filter
// now — a repo-scoped subscriber is held to its own repo and a chat in ANOTHER
// repo cannot reach it.
//
// The two workspaces are seeded for real, because the frame's repo is resolved
// from the workspace record: a frame naming a workspace nothing can resolve has
// no repo to be held to and reaches EVERY subscriber, which would make an
// unseeded fixture prove the isolation backwards.
func TestContainer_PushAgentChat_ReachesFilteredClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := newAppForSnapshot(t)
	seedWorkspace(t, a, "A", "p1", "r1", "", "")
	seedWorkspace(t, a, "B", "p1", "r2", "", "")
	c := New(a, nil)
	r := gin.New()
	r.GET(
		"/v0/projects/:projectId/repos/:repoId/chats/ws",
		func(ctx *gin.Context) { c.agentChats.Handle(ctx) },
	)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	conn := dialWSAt(t, srv, "/v0/projects/p1/repos/r1/chats/ws")
	c.agentChats.WaitRegistered()

	c.PushAgentChat("chat-in-r2", "B", "bound", false)
	c.PushAgentChat("chat-1", "A", "bound", false)

	got := readJSON(t, conn)
	assert.Equal(t, "chat-1", got["chatId"],
		"a repo-scoped subscriber must never receive another repo's chat frames")
	assert.Equal(t, "A", got["workspaceId"])
	assert.Equal(t, "bound", got["kind"])
	assert.Equal(t, "r1", got["repoId"])
}

// TestContainer_PushAgentChatTerminalWait_ReachesFilteredClient proves the
// terminal-wait edge fans out on the SAME workspace-scoped agent-chat
// WebSocket as PushAgentChat, carrying the wait's kind through untouched.
func TestContainer_PushAgentChatTerminalWait_ReachesFilteredClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := newAppForSnapshot(t)
	c := New(a, nil)
	r := gin.New()
	r.GET(
		"/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/chats/ws",
		func(ctx *gin.Context) { c.agentChats.Handle(ctx) },
	)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	conn := dialWSAt(t, srv, "/v0/projects/p1/repos/r1/workspaces/A/chats/ws")
	c.agentChats.WaitRegistered()

	c.PushAgentChatTerminalWait("chat-in-b", "B", &dto.AgentTerminalWaitDTO{Kind: "skip"})
	c.PushAgentChatTerminalWait("chat-1", "A", &dto.AgentTerminalWaitDTO{Kind: "permission"})

	got := readJSON(t, conn)
	assert.Equal(t, "chat-1", got["chatId"])
	assert.Equal(t, "terminal_wait", got["kind"])
	wait, ok := got["terminalWait"].(map[string]any)
	require.True(t, ok, "terminalWait must be present on a terminal_wait frame")
	assert.Equal(t, "permission", wait["kind"])
}

// TestContainer_PushAgentChatPromptSettled_ReachesFilteredClient proves the
// prompt-settled fact fans out on the SAME workspace-scoped feed, carrying the
// client's original request id so it can retire the pending submission.
func TestContainer_PushAgentChatPromptSettled_ReachesFilteredClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := newAppForSnapshot(t)
	c := New(a, nil)
	r := gin.New()
	r.GET(
		"/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/chats/ws",
		func(ctx *gin.Context) { c.agentChats.Handle(ctx) },
	)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	conn := dialWSAt(t, srv, "/v0/projects/p1/repos/r1/workspaces/A/chats/ws")
	c.agentChats.WaitRegistered()

	c.PushAgentChatPromptSettled("chat-in-b", "B", "req-skip")
	c.PushAgentChatPromptSettled("chat-1", "A", "req-1")

	got := readJSON(t, conn)
	assert.Equal(t, "chat-1", got["chatId"])
	assert.Equal(t, "prompt_settled", got["kind"])
	assert.Equal(t, "req-1", got["clientRequestId"])
}

// TestContainer_PushAgentChatMessageDelta_ReachesFilteredClient proves a
// streaming message delta fans out on the SAME workspace-scoped feed, and
// exercises the broadcaster's CoalesceKey path (message_delta is the one kind
// this feed coalesces).
func TestContainer_PushAgentChatMessageDelta_ReachesFilteredClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := newAppForSnapshot(t)
	c := New(a, nil)
	r := gin.New()
	r.GET(
		"/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/chats/ws",
		func(ctx *gin.Context) { c.agentChats.Handle(ctx) },
	)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	conn := dialWSAt(t, srv, "/v0/projects/p1/repos/r1/workspaces/A/chats/ws")
	c.agentChats.WaitRegistered()

	c.PushAgentChatMessageDelta("chat-in-b", "B", "m1", "skip")
	c.PushAgentChatMessageDelta("chat-1", "A", "m1", "hello")

	got := readJSON(t, conn)
	assert.Equal(t, "chat-1", got["chatId"])
	assert.Equal(t, "message_delta", got["kind"])
	msg, ok := got["message"].(map[string]any)
	require.True(t, ok, "message must be present on a message_delta frame")
	assert.Equal(t, "m1", msg["id"])
	assert.Equal(t, "hello", msg["text"])
}

// TestContainer_PushAgentChatCompaction_ReachesFilteredClient proves the
// compaction lifecycle kind picks compaction_started/compaction_stopped by the
// active flag, on the SAME workspace-scoped feed.
func TestContainer_PushAgentChatCompaction_ReachesFilteredClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := newAppForSnapshot(t)
	c := New(a, nil)
	r := gin.New()
	r.GET(
		"/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/chats/ws",
		func(ctx *gin.Context) { c.agentChats.Handle(ctx) },
	)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	conn := dialWSAt(t, srv, "/v0/projects/p1/repos/r1/workspaces/A/chats/ws")
	c.agentChats.WaitRegistered()

	c.PushAgentChatCompaction("chat-in-b", "B", true)
	c.PushAgentChatCompaction("chat-1", "A", true)

	got := readJSON(t, conn)
	assert.Equal(t, "chat-1", got["chatId"])
	assert.Equal(t, "compaction_started", got["kind"])

	c.PushAgentChatCompaction("chat-1", "A", false)
	got = readJSON(t, conn)
	assert.Equal(t, "compaction_stopped", got["kind"])
}

// TestContainer_PushAgentChatFolder_ReachesFilteredClient proves a chat-folder
// lifecycle event fans out on the SAME workspace-scoped feed as PushAgentChat,
// carrying the folder id and kind rather than a row.
func TestContainer_PushAgentChatFolder_ReachesFilteredClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := newAppForSnapshot(t)
	c := New(a, nil)
	r := gin.New()
	r.GET(
		"/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/chats/ws",
		func(ctx *gin.Context) { c.agentChats.Handle(ctx) },
	)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	conn := dialWSAt(t, srv, "/v0/projects/p1/repos/r1/workspaces/A/chats/ws")
	c.agentChats.WaitRegistered()

	c.PushAgentChatFolder("f-skip", "B", "folder_created")
	c.PushAgentChatFolder("f1", "A", "folder_created")

	got := readJSON(t, conn)
	assert.Equal(t, "f1", got["folderId"])
	assert.Equal(t, "folder_created", got["kind"])
}

// TestContainer_PushAgentRunner_ReachesFilteredClient proves a runner lifecycle
// event fans out on the SAME workspace-scoped feed, carrying RunnerID (empty
// on the chat kinds) alongside the chat it is currently pointed at.
func TestContainer_PushAgentRunner_ReachesFilteredClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := newAppForSnapshot(t)
	c := New(a, nil)
	r := gin.New()
	r.GET(
		"/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/chats/ws",
		func(ctx *gin.Context) { c.agentChats.Handle(ctx) },
	)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	conn := dialWSAt(t, srv, "/v0/projects/p1/repos/r1/workspaces/A/chats/ws")
	c.agentChats.WaitRegistered()

	c.PushAgentRunner("runner-skip", "B", "chat-skip", "moved")
	c.PushAgentRunner("runner-1", "A", "chat-1", "moved")

	got := readJSON(t, conn)
	assert.Equal(t, "runner-1", got["runnerId"])
	assert.Equal(t, "chat-1", got["chatId"])
	assert.Equal(t, "moved", got["kind"])
}

// TestNew_PanicsOnNilAppContainer proves the constructor fails fast rather than
// building a Container that would nil-panic on its first real request.
func TestNew_PanicsOnNilAppContainer(t *testing.T) {
	assert.PanicsWithValue(t, "v0: appContainer is required", func() {
		New(nil, nil)
	})
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
// "ended" lifecycle frame carrying the owning chat, EndedAt, and (when known)
// the exit code.
func TestOnTerminalEnded_PushesEndedFrame(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := newAppForSnapshot(t)

	c := New(a, nil)
	r := gin.New()
	r.GET(
		"/v0/chats/:chatId/terminals",
		func(ctx *gin.Context) { c.terminals.Handle(ctx) },
	)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	conn := dialWSAt(t, srv, "/v0/chats/c1/terminals")
	c.terminals.WaitRegistered()

	c.onTerminalEnded(context.Background(), "c1", "s1", 7)

	got := readJSON(t, conn)
	assert.Equal(t, "s1", got["id"])
	assert.Equal(t, "c1", got["chatId"])
	assert.Equal(t, "ended", got["status"])
	assert.NotNil(t, got["endedAt"])
	assert.Equal(t, float64(7), got["exitCode"])
}

// TestOnTerminalEnded_UnknownExitCodeOmitted proves a negative (unknown) exit
// code is not stamped onto the frame.
func TestOnTerminalEnded_UnknownExitCodeOmitted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := newAppForSnapshot(t)
	c := New(a, nil)
	r := gin.New()
	r.GET(
		"/v0/chats/:chatId/terminals",
		func(ctx *gin.Context) { c.terminals.Handle(ctx) },
	)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	conn := dialWSAt(t, srv, "/v0/chats/c1/terminals")
	c.terminals.WaitRegistered()

	c.onTerminalEnded(context.Background(), "c1", "s1", -1)

	got := readJSON(t, conn)
	assert.Equal(t, "ended", got["status"])
	_, hasExitCode := got["exitCode"]
	assert.False(t, hasExitCode, "unknown (negative) exit code must be omitted from the frame")
}

// TestOnTerminalState_PushesStateFrame proves the detach/suspend transition
// callback emits a lifecycle frame carrying the owning chat and the given
// state.
func TestOnTerminalState_PushesStateFrame(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := newAppForSnapshot(t)

	c := New(a, nil)
	r := gin.New()
	r.GET(
		"/v0/chats/:chatId/terminals",
		func(ctx *gin.Context) { c.terminals.Handle(ctx) },
	)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	conn := dialWSAt(t, srv, "/v0/chats/c1/terminals")
	c.terminals.WaitRegistered()

	c.onTerminalState(context.Background(), "c1", "s1", "detached")

	got := readJSON(t, conn)
	assert.Equal(t, "s1", got["id"])
	assert.Equal(t, "c1", got["chatId"])
	assert.Equal(t, "detached", got["status"])
}
