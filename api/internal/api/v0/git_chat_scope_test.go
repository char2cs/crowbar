package v0

import (
	"context"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/ws"
	"github.com/char2cs/crowbar/api/internal/app"
	chatrepo "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	workspacerepo "github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// This file proves git's chat-scoped stream end to end over the real v0
// Container, its real routes and real WebSocket connections. chatScopeEnv
// stands in only the chat ROWS; placementEnv runs the real chat and node
// repositories, so a binding follows a real placement write.

// fanoutChatRows is the container's ChatLister stood in for by raw rows with
// real ParentID edges: every ancestry decision is the resolver's own.
type fanoutChatRows []domain.Chat

func (r fanoutChatRows) ListChats(
	_ context.Context,
) ([]domain.Chat, error) {
	return r, nil
}

// batchImportedChats: chat-a owns ws-a, chat-b sits beside it, chat-c hangs
// under a folder, and chat-z owns an unrelated ws-z.
func batchImportedChats() fanoutChatRows {
	return fanoutChatRows{
		{ID: "chat-a", Type: domain.ChatTypeChat, WorkspaceID: "ws-a"},
		{ID: "chat-b", Type: domain.ChatTypeChat, ParentID: "chat-a"},
		{ID: "folder-f", Type: domain.ChatTypeFolder, ParentID: "chat-a"},
		{ID: "chat-c", Type: domain.ChatTypeChat, ParentID: "folder-f"},
		{ID: "chat-z", Type: domain.ChatTypeChat, WorkspaceID: "ws-z"},
	}
}

// rowsWorktreeResolver runs the REAL worktree package over stand-in rows. It is
// a pointer because the route middleware captures it at Register while the
// streams read the container field; mu guards rows against the broadcaster's
// own goroutines. reads counts every chat-forest read it serves.
type rowsWorktreeResolver struct {
	mu         sync.RWMutex
	rows       fanoutChatRows
	workspaces worktree.WorkspaceReader
	reads      int
}

func (r *rowsWorktreeResolver) snapshot() fanoutChatRows {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reads++
	return r.rows
}

func (r *rowsWorktreeResolver) readCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.reads
}

func (r *rowsWorktreeResolver) Resolve(
	ctx context.Context,
	chatID string,
) (domain.Workspace, error) {
	return worktree.Resolve(
		ctx,
		chatID,
		worktree.NewChatTreeAncestryReader(r.snapshot(), nil, nil),
		r.workspaces,
	)
}

func (r *rowsWorktreeResolver) WorkspacesForChats(
	ctx context.Context,
	chatIDs []string,
) (map[string]string, error) {
	return worktree.WorkspacesForChats(ctx, chatIDs, r.snapshot(), nil, nil)
}

func newRowsResolver(
	a *app.Container,
) *rowsWorktreeResolver {
	return &rowsWorktreeResolver{
		rows:       batchImportedChats(),
		workspaces: a.Repositories.Workspace,
	}
}

// chatScopeEnv stands up the whole real v0 surface over the stand-in rows. The
// resolver is installed BEFORE Register, which is when the chat group's
// resolveChatWorktree middleware captures it.
func chatScopeEnv(
	t *testing.T,
) (*Container, *httptest.Server) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	a, eng := newAppAndEngine(t)
	seedRepoRow(t, a, "p1", "r1")
	// Not git repositories, so no snapshot frame precedes the pushed ones:
	// these tests prove isolation by frame ORDER, without a timeout.
	seedWorkspaceAt(t, a, "ws-a", t.TempDir())
	seedWorkspaceAt(t, a, "ws-z", t.TempDir())
	a.Usecases.Worktree = newRowsResolver(a)

	c := New(a, eng)
	t.Cleanup(c.Close)
	r := gin.New()
	c.Register(r.Group("/v0"))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return c, srv
}

// seedWorkspaceAt creates a workspace row under p1/r1 at worktreePath and waits
// for its projection, so the route's scope guard sees it before a client dials.
func seedWorkspaceAt(
	t *testing.T,
	a *app.Container,
	id string,
	worktreePath string,
) {
	t.Helper()
	_, err := a.Repositories.Workspace.Create(
		context.Background(),
		workspacerepo.CreateInput{
			ID:           id,
			ProjectID:    "p1",
			RepoID:       "r1",
			WorktreePath: worktreePath,
			Provisioning: domain.WorkspaceProvisioned,
		},
		time.Unix(1, 0).UTC(),
	)
	require.NoError(t, err)
	a.Repositories.WaitQuiescent()
}

func seedRepoRow(
	t *testing.T,
	a *app.Container,
	projectID string,
	repoID string,
) {
	t.Helper()
	require.NoError(t, a.GORM.Repositories.Save(
		context.Background(),
		domain.Repository{ID: repoID, ProjectID: projectID, Name: repoID, Path: t.TempDir()},
	))
}

const workspaceGitRoute = "/v0/projects/p1/repos/r1/workspaces/"

// TestGitFanout_OnePushReachesEveryChatHoldingTheWorktree: three chats at three
// chat-scoped URLs over one worktree all receive one push; chat-z's first
// frame is its own ws-z push, so the ws-a frame never reached it.
func TestGitFanout_OnePushReachesEveryChatHoldingTheWorktree(t *testing.T) {
	c, srv := chatScopeEnv(t)

	owner := dialWSAt(t, srv, "/v0/chats/chat-a/git/status")
	sibling := dialWSAt(t, srv, "/v0/chats/chat-b/git/status")
	underFolder := dialWSAt(t, srv, "/v0/chats/chat-c/git/status")
	unrelated := dialWSAt(t, srv, "/v0/chats/chat-z/git/status")
	c.git.WaitNRegistered(4)

	c.PushGit("ws-a", gitdomain.GitStatus{Branch: "feature/a"})
	c.PushGit("ws-z", gitdomain.GitStatus{Branch: "feature/z"})

	assert.Equal(t, "feature/a", readJSON(t, owner)["branch"])
	assert.Equal(t, "feature/a", readJSON(t, sibling)["branch"],
		"a sibling chat on the same worktree shares its git state")
	assert.Equal(t, "feature/a", readJSON(t, underFolder)["branch"],
		"a chat filed under a folder still resolves through it to the worktree")
	assert.Equal(t, "feature/z", readJSON(t, unrelated)["branch"],
		"chat-z holds ws-z: the ws-a frame must never have reached it")
}

// TestGitCoexistence_TheWorkspaceScopedRouteIsGone: the retired
// /workspaces/:wsId/git/status mount refuses the upgrade outright.
func TestGitCoexistence_TheWorkspaceScopedRouteIsGone(t *testing.T) {
	_, srv := chatScopeEnv(t)

	url := "ws" + srv.URL[len("http"):] + workspaceGitRoute + "ws-a/git/status"
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if conn != nil {
		_ = conn.Close()
	}
	require.Error(t, err, "the old workspace-scoped git mount must no longer upgrade")
}

// TestGitDef_AChatWithNoWorktreeMatchesNothing: a chat whose ancestry owns no
// worktree binds to nothing, so neither a real workspace's status nor one
// naming no workspace reaches it.
func TestGitDef_AChatWithNoWorktreeMatchesNothing(t *testing.T) {
	a := newAppForSnapshot(t)
	a.Usecases.Worktree = &rowsWorktreeResolver{
		rows:       fanoutChatRows{{ID: "orphan", Type: domain.ChatTypeChat}},
		workspaces: a.Repositories.Workspace,
	}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequestWithContext(t.Context(), "GET", "/v0/chats/orphan/git/status", nil)
	ctx.Params = gin.Params{{Key: "chatId", Value: "orphan"}}

	matches, _ := ws.BuildPredicate(ctx, gitDef(a))

	assert.False(t, matches(gitdomain.GitStatusEvent{WsID: "ws-a"}))
	assert.False(t, matches(gitdomain.GitStatusEvent{}))
}

// TestGitSnapshotOnSubscribe_ResolvesABareChatID: on the chat route the
// snapshot scope is a bare CHAT id, resolved to the worktree behind it.
func TestGitSnapshotOnSubscribe_ResolvesABareChatID(t *testing.T) {
	a := newAppForSnapshot(t)
	seedWorkspace(t, a, "ws-a", "p1", "r1", "feature/a", "")
	a.Usecases.Worktree = newRowsResolver(a)

	got := gitSnapshot(a)("chat-c")

	require.Len(t, got, 1, "a chat under a folder still replays the worktree above it")
	assert.Equal(t, "ws-a", got[0].WsID)
}

// TestGitSnapshotOnSubscribe_AChatWithNoWorktreeReplaysNothing pins the
// degradation: no worktree in the ancestry replays nothing, not another one.
func TestGitSnapshotOnSubscribe_AChatWithNoWorktreeReplaysNothing(t *testing.T) {
	a := newAppForSnapshot(t)
	seedWorkspace(t, a, "ws-a", "p1", "r1", "feature/a", "")
	a.Usecases.Worktree = &rowsWorktreeResolver{
		rows: fanoutChatRows{
			{ID: "orphan", Type: domain.ChatTypeChat},
		},
		workspaces: a.Repositories.Workspace,
	}

	assert.Empty(t, gitSnapshot(a)("orphan"))
}

// TestGitStreamScopeKey_AnUnboundClientRefcountsItsPathWorkspace: ScopeKey
// answers only a client bound to no chat; a chat client is refcounted by its
// binding (TestRegression_TheFileWatcherFollowsAChatToItsNewWorktree).
func TestGitStreamScopeKey_AnUnboundClientRefcountsItsPathWorkspace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a, _ := newAppAndEngine(t)
	def := withOriginSyncLifecycle(withWatcherLifecycle(gitDef(a), a), a)
	require.NotNil(t, def.ScopeKey)

	viaPathParam, _ := gin.CreateTestContext(httptest.NewRecorder())
	viaPathParam.Request = httptest.NewRequestWithContext(t.Context(), "GET", workspaceGitRoute+"ws-direct/git/status", nil)
	viaPathParam.Params = gin.Params{{Key: "wsId", Value: "ws-direct"}}

	assert.Equal(t, "ws-direct", def.ScopeKey(viaPathParam),
		"a bound :wsId path param must still resolve directly")
}

// placementEnv stands up the real v0 surface over the REAL chat→worktree
// resolver, chat and node repositories: owner-a owns ws-a (not a git repo, so
// no snapshot frame on connect) and owner-b owns ws-b (a git repo on main).
func placementEnv(
	t *testing.T,
) (*Container, *httptest.Server, *app.Container) {
	t.Helper()
	return placementEnvAt(t, t.TempDir())
}

// placementEnvAt is placementEnv with ws-a's worktree at dirA.
func placementEnvAt(
	t *testing.T,
	dirA string,
) (*Container, *httptest.Server, *app.Container) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	a, eng := newAppAndEngine(t)
	seedRepoRow(t, a, "p1", "r1")
	seedWorkspaceAt(t, a, "ws-a", dirA)
	seedWorkspaceAt(t, a, "ws-b", initCleanGitRepo(t))
	createRealChat(t, a, "owner-a", "ws-a", "")
	createRealChat(t, a, "owner-b", "ws-b", "")

	c := New(a, eng)
	t.Cleanup(c.Close)
	r := gin.New()
	c.Register(r.Group("/v0"))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return c, srv, a
}

// createRealChat writes a chat through the real chat repository, owning
// workspaceID or filed under parentID, and waits for its projections.
func createRealChat(
	t *testing.T,
	a *app.Container,
	chatID string,
	workspaceID string,
	parentID string,
) {
	t.Helper()
	ctx := context.Background()
	_, err := a.Repositories.AgentChat.Create(ctx, chatrepo.CreateInput{
		ID: chatID, Type: domain.ChatTypeChat, WorkspaceID: workspaceID, Now: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)
	if parentID != "" {
		_, err = a.Repositories.AgentChat.SetPlacement(ctx, chatID, parentID, 0)
		require.NoError(t, err)
	}
	a.Repositories.WaitQuiescent()
}

// TestGitBinding_FollowsAChatToItsNewWorktree: a chat moved onto another
// worktree by a real placement write is re-pointed on the SAME connection,
// replayed the new worktree's status, and no longer sent the old one's.
func TestGitBinding_FollowsAChatToItsNewWorktree(t *testing.T) {
	c, srv, a := placementEnv(t)
	createRealChat(t, a, "chat-x", "", "owner-a")
	conn := dialWSAt(t, srv, "/v0/chats/chat-x/git/status")
	c.git.WaitNRegistered(1)

	c.PushGit("ws-a", gitdomain.GitStatus{Branch: "on-a"})
	assert.Equal(t, "on-a", readJSON(t, conn)["branch"])

	_, err := a.Repositories.AgentChat.SetWorkspace(context.Background(), "chat-x", "ws-b")
	require.NoError(t, err)
	assert.Equal(t, "main", readJSON(t, conn)["branch"],
		"the rebind replays the new worktree's status on the same connection")

	c.PushGit("ws-a", gitdomain.GitStatus{Branch: "stale-a"})
	c.PushGit("ws-b", gitdomain.GitStatus{Branch: "on-b"})
	assert.Equal(t, "on-b", readJSON(t, conn)["branch"],
		"the old worktree's status no longer reaches the moved chat")
}

// TestGitBinding_FollowsAFolderDraggedToAnotherOwner: the chat itself never
// moves; the folder it is filed under is re-parented by a node placement.
func TestGitBinding_FollowsAFolderDraggedToAnotherOwner(t *testing.T) {
	c, srv, a := placementEnv(t)
	ctx := context.Background()
	require.NoError(t, a.GORM.Folders.Save(ctx, domain.Folder{ID: "folder-f", Name: "notes"}))
	_, err := a.Repositories.Node.Create(ctx, "folder-f", domain.NodeKindFolder, "owner-a", 0)
	require.NoError(t, err)
	createRealChat(t, a, "chat-x", "", "folder-f")
	conn := dialWSAt(t, srv, "/v0/chats/chat-x/git/status")
	c.git.WaitNRegistered(1)

	c.PushGit("ws-a", gitdomain.GitStatus{Branch: "on-a"})
	assert.Equal(t, "on-a", readJSON(t, conn)["branch"])

	require.NoError(t, a.Repositories.Node.SetPlacement(ctx, "folder-f", "owner-b", 0))
	assert.Equal(t, "main", readJSON(t, conn)["branch"])

	c.PushGit("ws-a", gitdomain.GitStatus{Branch: "stale-a"})
	c.PushGit("ws-b", gitdomain.GitStatus{Branch: "on-b"})
	assert.Equal(t, "on-b", readJSON(t, conn)["branch"])
}

// TestRegression_GitForkAfterConnectReachesEveryChatOnTheWorktree: push-time
// fan-out existed for a chat forked onto a worktree after a sibling's stream
// opened; per-connection binding must still reach both.
func TestRegression_GitForkAfterConnectReachesEveryChatOnTheWorktree(t *testing.T) {
	c, srv, a := placementEnv(t)
	established := dialWSAt(t, srv, "/v0/chats/owner-a/git/status")
	c.git.WaitNRegistered(1)

	createRealChat(t, a, "chat-new", "", "owner-a")
	newcomer := dialWSAt(t, srv, "/v0/chats/chat-new/git/status")
	c.git.WaitNRegistered(1)

	c.PushGit("ws-a", gitdomain.GitStatus{Branch: "after-the-fork"})

	assert.Equal(t, "after-the-fork", readJSON(t, newcomer)["branch"])
	assert.Equal(t, "after-the-fork", readJSON(t, established)["branch"])
}
