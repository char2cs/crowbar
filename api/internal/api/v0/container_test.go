//go:build integration

package v0_test

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

	"github.com/char2cs/crowbar/api/internal/adapter"
	v0 "github.com/char2cs/crowbar/api/internal/api/v0"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	lspdomain "github.com/char2cs/crowbar/api/internal/domain/lsp"
	"github.com/char2cs/crowbar/api/internal/engine"
)

type testContainers struct {
	app *app.Container
	eng *engine.Container
}

func newApp(t *testing.T) testContainers {
	t.Helper()
	ctx := context.Background()
	eng, err := engine.New(ctx)
	require.NoError(t, err)
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })
	a, err := app.New(ctx, eng, adapters)
	require.NoError(t, err)
	return testContainers{app: a, eng: eng}
}

// workspaceFixture is a worktree a chat OWNS. The owning chat id is what keys
// the frame PushWorkspace fans out (pushChatWorktree); a row carrying none
// pushes nothing at all, so a fixture without it would prove nothing.
func workspaceFixture() dto.WorkspaceDTO {
	return dto.WorkspaceDTO{ID: "w1", RepoID: "r1", ProjectID: "p1", OwningChatID: "chat-1"}
}

// serveAgentChats mounts the agent-chat broadcaster on the two shapes its live
// routes take — the per-chat stream and the repo-scoped one — directly, rather
// than through Register: the chat group's resolveChatWorktree guard refuses a
// chat it cannot resolve to a worktree, and these tests are about which client a
// frame reaches, not about resolving one.
func serveAgentChats(
	t *testing.T,
	tc testContainers,
) (*v0.Container, *httptest.Server) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c := v0.New(tc.app, tc.eng)
	r := gin.New()
	r.GET("/v0/chats/:chatId/ws", c.AgentChatsHandle)
	r.GET("/v0/projects/:projectId/repos/:repoId/chats/ws", c.AgentChatsHandle)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return c, srv
}

// readFrame blocks until the next frame arrives, then decodes it. No read
// deadline: the frame's arrival IS the signal (see readSnapshot).
func readFrame(
	t *testing.T,
	conn *websocket.Conn,
) map[string]any {
	t.Helper()
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(msg, &got))
	return got
}

// chatWorktreeOf returns the worktree object riding a worktree_state frame.
func chatWorktreeOf(
	t *testing.T,
	frame map[string]any,
) map[string]any {
	t.Helper()
	worktree, ok := frame["worktree"].(map[string]any)
	require.True(t, ok, "a worktree_state frame carries the worktree it describes")
	return worktree
}

// seedRepo creates a real repository row under project p1 so the repo scope guard
// (scopeRepoToPath) admits a request to /projects/p1/repos/:repoId/...; a :repoId
// that resolves to no row, or to a different project, is now 404'd before the
// handler. Save is an upsert, so seeding the same repo twice is harmless.
func seedRepoIn(t *testing.T, tc testContainers, projectID, repoID string) {
	t.Helper()
	require.NoError(t, tc.app.GORM.Repositories.Save(
		context.Background(),
		domain.Repository{ID: repoID, ProjectID: projectID, Name: repoID, Path: t.TempDir()},
	))
}

func seedRepo(t *testing.T, tc testContainers, repoID string) {
	t.Helper()
	seedRepoIn(t, tc, "p1", repoID)
}

// seedWorkspace creates a real workspace row under p1/r1 (plus its repo) so both
// scope guards (scopeRepoToPath, scopeWorkspaceToPath) admit a request/WS upgrade
// to /workspaces/:id/...; a :repoId/:wsId that resolves to no row, or to a
// different scope, is now 404'd before the handler.
func seedWorkspace(t *testing.T, tc testContainers, id string) {
	t.Helper()
	seedRepo(t, tc, "r1")
	_, err := tc.app.Repositories.Workspace.Create(
		context.Background(),
		workspace.CreateInput{ID: id, RepoID: "r1", ProjectID: "p1", WorktreePath: t.TempDir()},
		time.Now(),
	)
	require.NoError(t, err)
}

// TestV0_HubWorkspaceBroadcastReachesChatWSClient proves the whole chain a
// workspace broadcast now takes: hub.BroadcastWorkspace -> PushWorkspace ->
// pushChatWorktree -> the agent-chat broadcaster -> a live WS client, keyed on
// the chat that owns the worktree rather than on the worktree itself (spec
// §7.4).
func TestV0_HubWorkspaceBroadcastReachesChatWSClient(t *testing.T) {
	tc := newApp(t)
	seedRepo(t, tc, "r1")
	c, srv := serveAgentChats(t, tc)

	conn := dialV0(t, srv, "/v0/chats/chat-1/ws")
	c.WaitAgentChatsRegistered()

	tc.app.Hub.BroadcastWorkspace(workspaceFixture())

	got := readFrame(t, conn)
	assert.Equal(t, dto.AgentChatKindWorktreeState, got["kind"])
	assert.Equal(t, "chat-1", got["chatId"])
	assert.Equal(t, "w1", got["workspaceId"])
	assert.Equal(t, "chat-1", chatWorktreeOf(t, got)["owningChatId"])
}

// TestV0_WorktreeState_ChatScopeIsolatesOtherChats proves the per-chat mount
// scopes a subscriber to its OWN chat: a worktree owned by another chat never
// reaches it.
//
// The isolation is proven by ORDER, not by a timeout: chat-1's frame is pushed
// first, so a leak would sit in chat-2's buffered channel ahead of its own
// frame and be the first thing it reads.
func TestV0_WorktreeState_ChatScopeIsolatesOtherChats(t *testing.T) {
	tc := newApp(t)
	seedRepo(t, tc, "r1")
	c, srv := serveAgentChats(t, tc)

	owner := dialV0(t, srv, "/v0/chats/chat-1/ws")
	other := dialV0(t, srv, "/v0/chats/chat-2/ws")
	c.WaitNAgentChatsRegistered(2)

	tc.app.Hub.BroadcastWorkspace(workspaceFixture())
	tc.app.Hub.BroadcastWorkspace(dto.WorkspaceDTO{
		ID: "w2", RepoID: "r1", ProjectID: "p1", OwningChatID: "chat-2",
	})

	assert.Equal(t, "w1", readFrame(t, owner)["workspaceId"])
	assert.Equal(t, "w2", readFrame(t, other)["workspaceId"],
		"chat-2 holds w2: the w1 frame must never have reached it")
}

// TestV0_WorktreeState_RepoScopeIsolatesOtherRepos proves the repo-scoped mount
// still holds a subscriber to its own repo once the frame is chat-keyed: the
// repo a worktree_state frame names is the one its workspace runs in, and a
// worktree in a sibling repo must not reach an r1-scoped client.
//
// Ordering carries the proof here too: the r2 frame is pushed FIRST, so a leak
// would be this client's first read.
func TestV0_WorktreeState_RepoScopeIsolatesOtherRepos(t *testing.T) {
	tc := newApp(t)
	seedRepo(t, tc, "r1")
	c, srv := serveAgentChats(t, tc)

	conn := dialV0(t, srv, "/v0/projects/p1/repos/r1/chats/ws")
	c.WaitAgentChatsRegistered()

	tc.app.Hub.BroadcastWorkspace(dto.WorkspaceDTO{
		ID: "w2", RepoID: "r2", ProjectID: "p1", OwningChatID: "chat-2",
	})
	tc.app.Hub.BroadcastWorkspace(workspaceFixture())

	got := readFrame(t, conn)
	assert.Equal(t, "w1", got["workspaceId"],
		"an r1-scoped subscriber must never receive another repo's worktree frame")
	assert.Equal(t, "r1", got["repoId"])
}

// TestContainer_PushProject_RouteByPrefix proves the Projects broadcaster routes
// by hierarchical prefix derived from the :projectId path param: a client scoped
// to "p1" receives only that project's frame; a sibling project's frame is
// filtered out (spec §5).
func TestContainer_PushProject_RouteByPrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tc := newApp(t)
	c := v0.New(tc.app, tc.eng)
	r := gin.New()
	r.GET("/v0/projects/:projectId", c.ProjectsHandle)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	url := "ws" + srv.URL[len("http"):] + "/v0/projects/p1"
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	c.WaitProjectsRegistered()

	// A sibling project's frame must be filtered out; only p1's arrives.
	tc.app.Hub.BroadcastProject(dto.ProjectDTO{ID: "p2", Name: "skip"})
	tc.app.Hub.BroadcastProject(dto.ProjectDTO{ID: "p1", Name: "keep"})

	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(msg, &got))
	assert.Equal(t, "p1", got["id"])
	assert.Equal(t, "keep", got["name"])
}

// TestContainer_PushRepo_RouteByPrefix proves the Repos broadcaster routes by
// hierarchical prefix derived from the :projectId path param: a client scoped to
// the project "p1" (prefix "p1") receives every child repo ("p1/r1") but not a
// sibling project's repo ("p2/r1") (spec §5).
func TestContainer_PushRepo_RouteByPrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tc := newApp(t)
	c := v0.New(tc.app, tc.eng)
	r := gin.New()
	r.GET("/v0/projects/:projectId/repos", c.ReposHandle)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	url := "ws" + srv.URL[len("http"):] + "/v0/projects/p1/repos"
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	c.WaitReposRegistered()

	// A sibling project's repo must be filtered out; only p1's child repo arrives.
	tc.app.Hub.BroadcastRepo(dto.RepoDTO{ID: "r1", ProjectID: "p2", Name: "skip"})
	tc.app.Hub.BroadcastRepo(dto.RepoDTO{ID: "r1", ProjectID: "p1", Name: "keep"})

	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(msg, &got))
	assert.Equal(t, "r1", got["id"])
	assert.Equal(t, "p1", got["projectId"])
	assert.Equal(t, "keep", got["name"])
}

// TestV0_PushLSP_ReachesFilteredClient dials the flat chat-scoped mount
// (spec §8 step 6 retired editor/LSP's .../workspaces/:wsId/lsp/ws twin
// entirely): editor/LSP's OWNED-bucket key is the chat id itself
// (handlers.Handlers.lspOwnerID), so a diagnostics event is keyed by "chat-1"
// rather than by the workspace it resolves to.
func TestV0_PushLSP_ReachesFilteredClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tc := newApp(t)
	seedWorkspace(t, tc, "w1")
	tc.app.Usecases.Worktree = stubChatWorktreeResolver{
		chatToWs:   map[string]string{"chat-1": "w1"},
		workspaces: tc.app.Repositories.Workspace,
	}
	c := v0.New(tc.app, tc.eng)
	r := gin.New()
	c.Register(r.Group("/v0"))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	url := "ws" + srv.URL[len("http"):] + "/v0/chats/chat-1/lsp/ws"
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	c.WaitLSPRegistered()

	// An event for a different chat must be filtered out; the matching one
	// must arrive.
	c.PushLSP(lspdomain.DiagnosticsEvent{WsID: "other-chat", Diagnostics: []lspdomain.Diagnostic{{Message: "skip"}}})
	c.PushLSP(lspdomain.DiagnosticsEvent{WsID: "chat-1", Diagnostics: []lspdomain.Diagnostic{{Message: "boom"}}})

	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(msg, &got))
	assert.Equal(t, "chat-1", got["wsId"])
	diags, _ := got["diagnostics"].([]any)
	require.Len(t, diags, 1)
}

// TestV0_PushGit_ChatFanout_IsolatesUnrelatedWorkspace dials the flat
// chat-scoped mount (spec §8 step 6 retired git's .../workspaces/:wsId/git/
// status twin entirely): a push for a workspace no chat resolves to (or a
// DIFFERENT chat's worktree) must never reach this subscriber, which
// gitDef's Required chatId membership filter is what now guarantees.
func TestV0_PushGit_ChatFanout_IsolatesUnrelatedWorkspace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tc := newApp(t)
	seedWorkspace(t, tc, "A")
	tc.app.Usecases.Worktree = stubChatWorktreeResolver{
		chatToWs:   map[string]string{"chat-1": "A"},
		wsToChats:  map[string][]string{"A": {"chat-1"}},
		workspaces: tc.app.Repositories.Workspace,
	}
	c := v0.New(tc.app, tc.eng)
	r := gin.New()
	c.Register(r.Group("/v0"))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	url := "ws" + srv.URL[len("http"):] + "/v0/chats/chat-1/git/status"
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	c.WaitGitRegistered()

	// A push for workspace B (no chat resolves to it here) must be filtered
	// out; only chat-1's own worktree status (A) arrives.
	tc.app.Hub.BroadcastGit("B", gitdomain.GitStatus{Branch: "branch-B"})
	tc.app.Hub.BroadcastGit("A", gitdomain.GitStatus{Branch: "branch-A"})

	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	var got gitdomain.GitStatus
	require.NoError(t, json.Unmarshal(msg, &got))
	assert.Equal(t, "branch-A", got.Branch)
}

func TestV0_PushFile_ReachesFilteredClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tc := newApp(t)
	seedWorkspace(t, tc, "w1")
	// files' own repo-scoped .../workspaces/:wsId/files/ws mount is gone (spec
	// §8 step 6); the flat chat prefix is what's left, fanned out by chat id
	// (wsToChats) the same way git's chatId filter works.
	tc.app.Usecases.Worktree = stubChatWorktreeResolver{
		chatToWs:   map[string]string{"chat-1": "w1"},
		wsToChats:  map[string][]string{"w1": {"chat-1"}},
		workspaces: tc.app.Repositories.Workspace,
	}
	c := v0.New(tc.app, tc.eng)
	r := gin.New()
	c.Register(r.Group("/v0"))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	url := "ws" + srv.URL[len("http"):] + "/v0/chats/chat-1/files/ws"
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	c.WaitFilesRegistered()

	tc.app.Hub.BroadcastFile(domain.FileChangeEvent{WsID: "other", Path: "skip.go"})
	tc.app.Hub.BroadcastFile(domain.FileChangeEvent{WsID: "w1", Path: "a.go"})

	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	var got domain.FileChangeEvent
	require.NoError(t, json.Unmarshal(msg, &got))
	assert.Equal(t, "a.go", got.Path)
}
