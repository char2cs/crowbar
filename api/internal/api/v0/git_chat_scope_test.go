package v0

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"
	"github.com/char2cs/crowbar/api/internal/app"
	workspacerepo "github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine"
)

// This file proves git's move onto /v0/chats/:chatId end to end over the REAL
// delivery path — the real v0 Container, its real route registration, the real
// gitDef predicate compiled for each connecting client, the real
// worktree.ChatsForWorkspace fan-out, and real WebSocket connections. Only the
// chat ROWS are stood in for, exactly as chat_fanout_test.go stands them in:
// every ancestry and folder-crossing decision is still made by the resolver
// under test.
//
// The scenario it keeps returning to is the one this whole step exists for and
// the one Step 1 made ordinary rather than exotic: several chats over ONE
// worktree. git is spec §4.2's shared bucket — the worktree answers once — so a
// status is news for every chat holding it, not only the one whose route
// triggered the write.

// rowsWorktreeResolver is the container's WorktreeResolver over stand-in chat
// rows: both directions run the REAL worktree package (Resolve and its inverse,
// ChatsForWorkspace) against real workspace rows from the real repository. It
// substitutes the chat FOREST, never the resolution.
//
// It is a POINTER so its forest can grow mid-test. The chat group's
// resolveChatWorktree middleware captures the resolver VALUE when Register runs,
// while PushGit reads the container field on every call — so a test that
// reassigned the field would leave the middleware resolving against the old
// forest and the fan-out against the new one. One shared pointer is what keeps
// the two agreeing, exactly as the single container-built resolver does in
// production.
type rowsWorktreeResolver struct {
	rows       fanoutChatRows
	workspaces worktree.WorkspaceReader
}

func (r *rowsWorktreeResolver) Resolve(
	ctx context.Context,
	chatID string,
) (domain.Workspace, error) {
	return worktree.Resolve(
		ctx,
		chatID,
		worktree.NewChatTreeAncestryReader(r.rows),
		r.workspaces,
	)
}

func (r *rowsWorktreeResolver) ChatsForWorkspace(
	ctx context.Context,
	workspaceID string,
) ([]string, error) {
	return worktree.ChatsForWorkspace(ctx, workspaceID, r.rows)
}

// fork adds a chat to the forest, the way a fork onto an existing worktree
// does: a new row whose parent already owns one.
func (r *rowsWorktreeResolver) fork(
	chatID string,
	parentID string,
) {
	r.rows = append(r.rows, domain.Chat{
		ID:       chatID,
		Type:     domain.ChatTypeChat,
		ParentID: parentID,
	})
}

// newRowsResolver builds the resolver over the batch-import chat shape:
// chat-a owns ws-a, chat-b sits beside it, chat-c hangs under a folder, and
// chat-z owns an unrelated ws-z.
func newRowsResolver(
	a *app.Container,
) *rowsWorktreeResolver {
	return &rowsWorktreeResolver{
		rows:       batchImportedChats(),
		workspaces: a.Repositories.Workspace,
	}
}

// chatScopeEnv stands up the real v0 surface — real routes, real middleware,
// real scope guards, real broadcasters — over that chat shape. It registers the
// WHOLE container, not just git, so every chat-scoped group's tests share one
// env and one chat forest (files_chat_scope_test.go is the other caller).
//
// The resolver is installed BEFORE Register, because that is when the chat
// group's resolveChatWorktree middleware captures it.
func chatScopeEnv(
	t *testing.T,
) (*Container, *httptest.Server, *rowsWorktreeResolver) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	a, eng := newAppAndEngine(t)
	seedRepoRow(t, a, "p1", "r1")
	// A worktree path that is not a git repository, so the snapshot-on-subscribe
	// resolves nothing and the FIRST frame each client below reads is the one
	// the test pushed. Frame ORDER is how these tests prove isolation without a
	// timeout, so a replay frame arriving ahead of the pushed one would not just
	// add noise — it would read as the wrong answer.
	seedWorkspaceAt(t, a, "ws-a", t.TempDir())
	seedWorkspaceAt(t, a, "ws-z", t.TempDir())
	resolver := newRowsResolver(a)
	a.Usecases.Worktree = resolver

	c := New(a, eng)
	r := gin.New()
	c.Register(r.Group("/v0"))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return c, srv, resolver
}

// newAppAndEngine builds the same real app container newAppForSnapshot does,
// and hands back the ENGINE with it: Container.Register reaches into
// c.eng.Provider/Git/LSP as it wires the other endpoint groups, so the full
// route registration these tests exercise cannot run against a nil one.
func newAppAndEngine(
	t *testing.T,
) (*app.Container, *engine.Container) {
	t.Helper()
	ctx := context.Background()
	eng, err := engine.New(ctx)
	require.NoError(t, err)
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })
	a, err := app.New(ctx, eng, adapters)
	require.NoError(t, err)
	return a, eng
}

// seedWorkspaceAt creates a workspace row under p1/r1 at a given worktree path,
// and waits for the async store projection the same way seedWorkspace does, so
// the route's own scope guard can see the row before a client dials.
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

// TestGitFanout_OnePushReachesEveryChatHoldingTheWorktree is the scenario the
// step exists for, over the real container: three chats subscribed at three
// DIFFERENT chat-scoped URLs, one worktree behind them, one push.
//
// The set is NOT handed to the push — Container.PushGit resolves it itself, the
// way the watcher dispatcher calls it in production. A fan-out that silently
// resolved nothing would leave every client below empty-handed.
//
// The unrelated chat's isolation is proven without a timeout: it is sent a
// second, ws-z frame after the ws-a one, and its first read must be that second
// frame. A leaked ws-a frame would arrive first — Push delivers in call order
// into each client's own buffered channel — and fail the assertion.
func TestGitFanout_OnePushReachesEveryChatHoldingTheWorktree(t *testing.T) {
	c, srv, _ := chatScopeEnv(t)

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

// TestGitCoexistence_TheWorkspaceScopedRouteIsGone proves spec §8 step 6's
// deletion is real over the REAL delivery path: a WebSocket upgrade attempt on
// the old /workspaces/:wsId/git/status mount fails outright — the route no
// longer exists to upgrade — rather than connecting and silently seeing
// nothing (the failure mode gitDef's Required chatId filter would produce if
// the mount were somehow still reachable).
func TestGitCoexistence_TheWorkspaceScopedRouteIsGone(t *testing.T) {
	_, srv, _ := chatScopeEnv(t)

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

// TestGitFanout_AChatForkedAfterAClientConnectedWidensTheSet is why the
// resolved set rides on the EVENT rather than being baked into a client's
// predicate at connect time.
//
// chat-a subscribes while the worktree has three chats on it. A fourth is then
// forked onto the same worktree and subscribes too, and ONE push reaches both —
// including chat-a, whose predicate was compiled before chat-new existed and
// which never reconnected. A set resolved at connect time could not have done
// that: it would have had to be either chat-a's set (missing chat-new) or
// chat-new's (compiled too late for chat-a).
//
// The stricter edge — a client subscribed for a chat that does not exist YET —
// is not reachable through the real route, and correctly so: resolveChatWorktree
// refuses the upgrade for a chat it cannot resolve. That case is pinned at the
// predicate level in ws/chat_fanout_test.go instead.
func TestGitFanout_AChatForkedAfterAClientConnectedWidensTheSet(t *testing.T) {
	c, srv, resolver := chatScopeEnv(t)

	established := dialWSAt(t, srv, "/v0/chats/chat-a/git/status")
	c.git.WaitNRegistered(1)

	resolver.fork("chat-new", "chat-a")
	newcomer := dialWSAt(t, srv, "/v0/chats/chat-new/git/status")
	c.git.WaitNRegistered(1)

	c.PushGit("ws-a", gitdomain.GitStatus{Branch: "after-the-fork"})

	assert.Equal(t, "after-the-fork", readJSON(t, newcomer)["branch"],
		"a chat forked onto the worktree after this stream opened must receive its state")
	assert.Equal(t, "after-the-fork", readJSON(t, established)["branch"],
		"and the chat that was already listening must not have been dropped to make room")
}

// TestGitSnapshotOnSubscribe_ResolvesABareChatID covers the OTHER half of a
// subscription: the snapshot replay, which never goes through the predicate
// that produced it. It is handed ws.clientScope's scope string directly, and on
// the chat route that string is a bare CHAT id — so this is the path that
// depends on resolving one.
//
// A break here is invisible to every live-frame test above: the connection
// opens, every later push arrives correctly, and the panel is simply blank
// until something in the worktree happens to change.
func TestGitSnapshotOnSubscribe_ResolvesABareChatID(t *testing.T) {
	a := newAppForSnapshot(t)
	seedWorkspace(t, a, "ws-a", "p1", "r1", "feature/a", "")
	a.Usecases.Worktree = newRowsResolver(a)

	got := gitSnapshot(a)("chat-c")

	require.Len(t, got, 1, "a chat under a folder still replays the worktree above it")
	assert.Equal(t, "ws-a", got[0].WsID)
	assert.Contains(t, got[0].ChatIDs, "chat-c",
		"a replay frame the subscriber's own predicate rejects is a blank panel")
	assert.Equal(t, []string{"chat-a", "chat-b", "chat-c"}, got[0].ChatIDs,
		"the replay carries the same fan-out set a live push would")
}

// TestGitSnapshotOnSubscribe_AChatWithNoWorktreeReplaysNothing pins the
// degradation: a chat whose ancestry owns no worktree cannot name a workspace,
// so it replays nothing rather than falling back to some other one.
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

// TestGitStreamScopeKey_AChatSubscriberRefcountsTheResolvedWorkspace pins the
// quietest thing in this step, and the one whose absence would look like
// nothing at all.
//
// The git stream's ScopeKey does not scope delivery — the filters do that. It
// names the workspace the lazy per-scope RESOURCES are refcounted by: the file
// watcher that produces git-status pushes in the first place, and the
// protected-branch origin sync. A chat-scoped client binds no :wsId, so before
// this step that scope resolved to the empty string and neither resource was
// ever acquired for the workspace it was watching.
//
// The result would have been a subscription that connects, replays its
// snapshot, passes every delivery test in this file, and then never receives
// another frame — because nothing was left watching the worktree to produce
// one. It is asserted through the SAME wrapper chain New builds, so the wiring
// is covered and not just the function.
func TestGitStreamScopeKey_AChatSubscriberRefcountsTheResolvedWorkspace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a, _ := newAppAndEngine(t)
	def := withOriginSyncLifecycle(withWatcherLifecycle(gitDef(a), a), a)
	require.NotNil(t, def.ScopeKey)

	viaChat, _ := gin.CreateTestContext(httptest.NewRecorder())
	viaChat.Request = httptest.NewRequest("GET", "/v0/chats/chat-a/git/status", nil)
	viaChat.Params = gin.Params{{Key: "chatId", Value: "chat-a"}}
	reqscope.SetWorkspace(viaChat, domain.Workspace{ID: "ws-a"})

	assert.Equal(t, "ws-a", def.ScopeKey(viaChat),
		"the watcher must be refcounted against the resolved worktree, not against nothing")

	// scopeWsID itself is shared, general-purpose infrastructure (files' home
	// mount still binds a real :wsId param, see files_chat_scope_test.go) even
	// though git's own :wsId-bound route is gone (spec §8 step 6); this proves
	// its path-param branch still resolves correctly wherever it IS bound.
	viaPathParam, _ := gin.CreateTestContext(httptest.NewRecorder())
	viaPathParam.Request = httptest.NewRequest("GET", workspaceGitRoute+"ws-direct/git/status", nil)
	viaPathParam.Params = gin.Params{{Key: "wsId", Value: "ws-direct"}}

	assert.Equal(t, "ws-direct", def.ScopeKey(viaPathParam),
		"a bound :wsId path param must still resolve directly")
}

// TestGitPush_CarriesTheFanoutSetResolvedAtPushTime asserts the wiring this
// step adds at its own seam, so a failure points at PushGit rather than at
// whichever client stopped receiving frames.
func TestGitPush_CarriesTheFanoutSetResolvedAtPushTime(t *testing.T) {
	a := newAppForSnapshot(t)
	a.Usecases.Worktree = newRowsResolver(a)
	c := New(a, nil)

	assert.Equal(t, []string{"chat-a", "chat-b", "chat-c"}, c.chatsHolding(context.Background(), "ws-a"))
	assert.Equal(t, []string{"chat-z"}, c.chatsHolding(context.Background(), "ws-z"))
	assert.Empty(t, c.chatsHolding(context.Background(), "ws-nobody-holds"),
		"a workspace no chat points at fans out to nobody, and is not an error")
}
