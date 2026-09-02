package v0

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/ws"
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// This file proves the shared-bucket fan-out (spec §7.4) end to end over the
// REAL machinery both sides: worktree.ChatsForWorkspace resolving the set, and
// ws.Broadcaster/BuildPredicate delivering against it over real WebSocket
// connections. No route is re-keyed yet — git/review/files/search/identity stay
// workspace-scoped until the next step — so the stream below is a stand-in for
// the shape they take when they move, not a copy of any of them.

// sharedWorktreeEvent is the shape a shared-bucket event takes once re-keyed:
// the workspace it describes, plus the chat ids resolved for that workspace at
// PUSH time. ChatIDs is routing, not payload, so it stays off the wire — the
// same way gitdomain.GitStatusEvent serializes only its embedded Status.
type sharedWorktreeEvent struct {
	WsID    string   `json:"-"`
	ChatIDs []string `json:"-"`
	Branch  string   `json:"branch"`
}

func sharedWorktreeDef() ws.StreamDef[sharedWorktreeEvent] {
	return ws.StreamDef[sharedWorktreeEvent]{
		Namespace:     func(e sharedWorktreeEvent) string { return e.WsID },
		Serialize:     func(e sharedWorktreeEvent) ([]byte, error) { return json.Marshal(e) },
		FlatNamespace: true,
		Filters: []ws.FilterDef[sharedWorktreeEvent]{
			ws.ChatFanoutFilter(func(e sharedWorktreeEvent) []string { return e.ChatIDs }),
		},
	}
}

// fanoutChatRows is the container's ChatLister (usecases/chat.ListChats) stood
// in for by raw rows carrying real ParentID edges — every ancestry and
// folder-crossing decision is left to the resolver under test.
type fanoutChatRows []domain.Chat

func (r fanoutChatRows) ListChats(
	_ context.Context,
) ([]domain.Chat, error) {
	return r, nil
}

// batchImportedChats is the normal post-Step-1 shape: a batch import creates
// chat-a owning the worktree, chat-b beside it, and chat-c filed under a
// folder — all three on ONE workspace — while chat-z owns an unrelated one.
func batchImportedChats() fanoutChatRows {
	return fanoutChatRows{
		{ID: "chat-a", Type: domain.ChatTypeChat, WorkspaceID: "ws-a"},
		{ID: "chat-b", Type: domain.ChatTypeChat, ParentID: "chat-a"},
		{ID: "folder-f", Type: domain.ChatTypeFolder, ParentID: "chat-a"},
		{ID: "chat-c", Type: domain.ChatTypeChat, ParentID: "folder-f"},
		{ID: "chat-z", Type: domain.ChatTypeChat, WorkspaceID: "ws-z"},
	}
}

func serveSharedWorktreeStream(
	t *testing.T,
) (*ws.Broadcaster[sharedWorktreeEvent], *httptest.Server) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	b := ws.NewBroadcaster(sharedWorktreeDef())
	r := gin.New()
	r.GET("/v0/chats/:chatId/git/status", b.Handle)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return b, srv
}

// TestSharedWorktreeFanout_OnePushReachesEverySiblingChatAndNoOther is the
// scenario the whole step exists for: three chats subscribed under three
// DIFFERENT chat-scoped URLs, one worktree behind them, one Push.
//
// The unrelated chat's isolation is proven without a timeout: it is sent a
// second, ws-z frame after the ws-a one, and its first read must be that second
// frame. A leaked ws-a frame would arrive first — Push delivers in call order
// into each client's own buffered channel — and fail the assertion.
func TestSharedWorktreeFanout_OnePushReachesEverySiblingChatAndNoOther(t *testing.T) {
	rows := batchImportedChats()
	shared, err := worktree.ChatsForWorkspace(context.Background(), "ws-a", rows)
	require.NoError(t, err)
	require.Equal(t, []string{"chat-a", "chat-b", "chat-c"}, shared)

	b, srv := serveSharedWorktreeStream(t)
	owner := dialWSAt(t, srv, "/v0/chats/chat-a/git/status")
	sibling := dialWSAt(t, srv, "/v0/chats/chat-b/git/status")
	underFolder := dialWSAt(t, srv, "/v0/chats/chat-c/git/status")
	unrelated := dialWSAt(t, srv, "/v0/chats/chat-z/git/status")
	b.WaitNRegistered(4)

	b.Push(sharedWorktreeEvent{WsID: "ws-a", ChatIDs: shared, Branch: "feature/a"})

	elsewhere, err := worktree.ChatsForWorkspace(context.Background(), "ws-z", rows)
	require.NoError(t, err)
	b.Push(sharedWorktreeEvent{WsID: "ws-z", ChatIDs: elsewhere, Branch: "feature/z"})

	assert.Equal(t, "feature/a", readJSON(t, owner)["branch"])
	assert.Equal(t, "feature/a", readJSON(t, sibling)["branch"])
	assert.Equal(t, "feature/a", readJSON(t, underFolder)["branch"])
	assert.Equal(
		t,
		"feature/z",
		readJSON(t, unrelated)["branch"],
		"chat-z resolves to ws-z: the ws-a frame must never have reached it",
	)
}

// TestSharedWorktreeFanout_AChatCreatedAfterAClientConnectedStillReceives is
// the reason the resolved set rides on the EVENT rather than being baked into a
// client's predicate at connect time.
//
// chat-new dials before it exists in the forest at all. The first push cannot
// name it and must not reach it; once the row exists, the next push resolves a
// set that includes it and it receives that frame — over the same connection,
// with no reconnect and no re-registration.
func TestSharedWorktreeFanout_AChatCreatedAfterAClientConnectedStillReceives(t *testing.T) {
	rows := batchImportedChats()

	b, srv := serveSharedWorktreeStream(t)
	newcomer := dialWSAt(t, srv, "/v0/chats/chat-new/git/status")
	b.WaitNRegistered(1)

	before, err := worktree.ChatsForWorkspace(context.Background(), "ws-a", rows)
	require.NoError(t, err)
	require.NotContains(t, before, "chat-new")
	b.Push(sharedWorktreeEvent{WsID: "ws-a", ChatIDs: before, Branch: "before-the-fork"})

	rows = append(rows, domain.Chat{ID: "chat-new", Type: domain.ChatTypeChat, ParentID: "chat-a"})
	after, err := worktree.ChatsForWorkspace(context.Background(), "ws-a", rows)
	require.NoError(t, err)
	require.Contains(t, after, "chat-new")
	b.Push(sharedWorktreeEvent{WsID: "ws-a", ChatIDs: after, Branch: "after-the-fork"})

	assert.Equal(
		t,
		"after-the-fork",
		readJSON(t, newcomer)["branch"],
		"the pre-existence frame must not have reached a chat the resolver could not name",
	)
}
