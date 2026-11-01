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
)

// The hub.Subscriber push wiring (PushProject...PushFile), the terminal
// lifecycle callbacks and their workspace-scope helpers, driven in-package
// without standing up the full Register() router.

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

// wsReadBound is a failure bound, never reached on a correct run: a frame that
// never comes fails its own test in seconds instead of stalling the package.
const wsReadBound = 10 * time.Second

// readJSON waits for the next frame, then decodes it.
func readJSON(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(wsReadBound)))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err, "no frame within %v", wsReadBound)
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

	c.PushAgentChatEvent(dto.AgentChatEvent{ChatID: "chat-in-r2", WorkspaceID: "B", Kind: "bound"})
	c.PushAgentChatEvent(dto.AgentChatEvent{ChatID: "chat-1", WorkspaceID: "A", Kind: "bound"})

	got := readJSON(t, conn)
	assert.Equal(t, "chat-1", got["chatId"],
		"a repo-scoped subscriber must never receive another repo's chat frames")
	assert.Equal(t, "A", got["workspaceId"])
	assert.Equal(t, "bound", got["kind"])
	assert.Equal(t, "r1", got["repoId"])
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

// TestContainer_PushAgentChatEvent_ReachesFilteredClient proves a chat
// snapshot frame fans out on the workspace-scoped feed carrying the runner it
// was about and the chat's full versioned snapshot.
func TestContainer_PushAgentChatEvent_ReachesFilteredClient(t *testing.T) {
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

	c.PushAgentChatEvent(dto.AgentChatEvent{ChatID: "chat-skip", WorkspaceID: "B", Kind: "moved", RunnerID: "runner-skip"})
	c.PushAgentChatEvent(dto.AgentChatEvent{
		ChatID: "chat-1", WorkspaceID: "A", Kind: "moved", RunnerID: "runner-1", Version: 9,
		Chat: &dto.AgentChatDTO{ID: "chat-1", WorkspaceID: "A", Version: 9, Phase: "live", LiveRunnerID: "runner-1"},
	})

	got := readJSON(t, conn)
	assert.Equal(t, "runner-1", got["runnerId"])
	assert.Equal(t, "chat-1", got["chatId"])
	assert.Equal(t, "moved", got["kind"])
	assert.EqualValues(t, 9, got["version"])
	chat, ok := got["chat"].(map[string]any)
	require.True(t, ok, "the frame carries the chat snapshot")
	assert.Equal(t, "live", chat["phase"])
	assert.Equal(t, "runner-1", chat["liveRunnerId"])
}

// TestNew_PanicsOnNilAppContainer proves the constructor fails fast rather than
// building a Container that would nil-panic on its first real request.
func TestNew_PanicsOnNilAppContainer(t *testing.T) {
	assert.PanicsWithValue(t, "v0: appContainer is required", func() {
		New(nil, nil)
	})
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
		workspace.CreateInput{ID: "w1", ProjectID: "p1", RepoID: "r1", Provisioning: domain.WorkspacePlaceholder},
		time.Unix(1, 0).UTC(),
	)
	require.NoError(t, err)

	c := &Container{app: a}
	projectID, repoID := c.resolveWorkspaceScope(context.Background(), "w1")
	assert.Equal(t, "p1", projectID)
	assert.Equal(t, "r1", repoID)
}
