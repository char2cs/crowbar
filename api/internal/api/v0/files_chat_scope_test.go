package v0

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// This file proves files' move onto /v0/chats/:chatId end to end over the REAL
// delivery path — the real v0 Container, its real route registration, the real
// filesDef predicate compiled for each connecting client, the real
// worktree.ChatsForWorkspace fan-out, and real WebSocket connections. Only the
// chat ROWS are stood in for (chatScopeEnv, git_chat_scope_test.go): every
// ancestry and folder-crossing decision is still made by the resolver under
// test.
//
// files is the LAST of spec §4.2's shared bucket, and shares the whole bucket's
// premise: the worktree holds one tree, so a change made through ONE chat is
// news for every sibling chat holding that worktree, not only the one whose
// route wrote it.
//
// One thing here differs from git and is worth naming: filesDef carries NO
// snapshot-on-subscribe (defs_test.go pins that), so there is no replay half to
// prove — a chat-scoped subscriber's first frame is simply the next change, and
// every assertion below reads a live push.

const workspaceFilesRoute = "/v0/projects/p1/repos/r1/workspaces/"

// fileChange is the event the watcher dispatcher hands PushFile. It deliberately
// carries NO ChatIDs: resolving the fan-out set is PushFile's own job, and a
// test that pre-stamped it would prove the broadcaster works and nothing about
// the wiring this step adds.
func fileChange(
	wsID string,
	path string,
) domain.FileChangeEvent {
	return domain.FileChangeEvent{Type: domain.FileChangeModified, WsID: wsID, Path: path}
}

// TestFilesFanout_OnePushReachesEveryChatHoldingTheWorktree is the scenario the
// step exists for, over the real container: three chats subscribed at three
// DIFFERENT chat-scoped URLs, one worktree behind them, one push.
//
// The set is NOT handed to the push — Container.PushFile resolves it itself,
// the way the watcher dispatcher calls it in production. A fan-out that
// silently resolved nothing would leave every client below empty-handed, and
// their file trees frozen at whatever they last fetched.
//
// The unrelated chat's isolation is proven without a timeout: it is sent a
// second, ws-z frame after the ws-a one, and its first read must be that second
// frame. A leaked ws-a frame would arrive first — Push delivers in call order
// into each client's own buffered channel — and fail the assertion.
func TestFilesFanout_OnePushReachesEveryChatHoldingTheWorktree(t *testing.T) {
	c, srv, _ := chatScopeEnv(t)

	owner := dialWSAt(t, srv, "/v0/chats/chat-a/files/ws")
	sibling := dialWSAt(t, srv, "/v0/chats/chat-b/files/ws")
	underFolder := dialWSAt(t, srv, "/v0/chats/chat-c/files/ws")
	unrelated := dialWSAt(t, srv, "/v0/chats/chat-z/files/ws")
	c.files.WaitNRegistered(4)

	c.PushFile(fileChange("ws-a", "a.go"))
	c.PushFile(fileChange("ws-z", "z.go"))

	assert.Equal(t, "a.go", readJSON(t, owner)["path"])
	assert.Equal(t, "a.go", readJSON(t, sibling)["path"],
		"a sibling chat on the same worktree shares its file tree")
	assert.Equal(t, "a.go", readJSON(t, underFolder)["path"],
		"a chat filed under a folder still resolves through it to the worktree")
	assert.Equal(t, "z.go", readJSON(t, unrelated)["path"],
		"chat-z holds ws-z: the ws-a frame must never have reached it")
}

// TestFilesCoexistence_TheWorkspaceScopedRouteIsUnchanged is this step's
// regression bar, and the reason NEITHER of filesDef's filters is Required.
//
// The workspace-scoped route is deliberately still mounted (spec §8 step 6
// retires it, not this step), and the HOME route reaches the same broadcaster
// through a :wsId its own middleware injects. Neither can ever bind a :chatId —
// so a Required chatId filter would compile their predicates to "match nothing"
// and silently freeze the file tree for every one of their clients.
func TestFilesCoexistence_TheWorkspaceScopedRouteIsUnchanged(t *testing.T) {
	c, srv, _ := chatScopeEnv(t)

	legacy := dialWSAt(t, srv, workspaceFilesRoute+"ws-a/files/ws")
	c.files.WaitNRegistered(1)

	c.PushFile(fileChange("ws-z", "z.go"))
	c.PushFile(fileChange("ws-a", "a.go"))

	assert.Equal(t, "a.go", readJSON(t, legacy)["path"],
		"a wsId-scoped client must still receive its own workspace, and only it")
}

// TestFilesCoexistence_OneBroadcasterServesBothRoutesFromOnePush pins the shape
// the live mounts actually have: ONE Broadcaster, ONE StreamDef, compiled once
// at construction. The routes are not separate streams that could drift — they
// are ways of naming a client's scope on the same stream, and each client is
// held to whichever param its own request bound.
func TestFilesCoexistence_OneBroadcasterServesBothRoutesFromOnePush(t *testing.T) {
	c, srv, _ := chatScopeEnv(t)

	viaChat := dialWSAt(t, srv, "/v0/chats/chat-b/files/ws")
	viaWorkspace := dialWSAt(t, srv, workspaceFilesRoute+"ws-a/files/ws")
	c.files.WaitNRegistered(2)

	c.PushFile(fileChange("ws-a", "a.go"))

	assert.Equal(t, "a.go", readJSON(t, viaChat)["path"])
	assert.Equal(t, "a.go", readJSON(t, viaWorkspace)["path"])
}

// TestFilesCoexistence_NeitherRouteSeesTheOthersUnrelatedTraffic walks both
// mounts past a workspace neither of them holds, so a filter that had gone
// INACTIVE — the real failure mode here, since an unresolvable filter is
// dropped rather than denied — would show up as a firehose rather than as
// silence.
func TestFilesCoexistence_NeitherRouteSeesTheOthersUnrelatedTraffic(t *testing.T) {
	c, srv, _ := chatScopeEnv(t)

	chatOnA := dialWSAt(t, srv, "/v0/chats/chat-a/files/ws")
	legacyOnA := dialWSAt(t, srv, workspaceFilesRoute+"ws-a/files/ws")
	c.files.WaitNRegistered(2)

	// ws-z first: neither client holds it, so neither may receive it. Both must
	// read the ws-a frame that follows as their FIRST frame.
	c.PushFile(fileChange("ws-z", "z.go"))
	c.PushFile(fileChange("ws-a", "a.go"))

	assert.Equal(t, "a.go", readJSON(t, chatOnA)["path"],
		"the chat-scoped client must not have been handed another worktree's changes")
	assert.Equal(t, "a.go", readJSON(t, legacyOnA)["path"],
		"the workspace-scoped client must not have been handed another worktree's changes")
}

// TestFilesFanout_AChatForkedAfterAClientConnectedWidensTheSet is why the
// resolved set rides on the EVENT rather than being baked into a client's
// predicate at connect time.
//
// chat-a subscribes while the worktree has three chats on it. A fourth is then
// forked onto the same worktree and subscribes too, and ONE push reaches both —
// including chat-a, whose predicate was compiled before chat-new existed and
// which never reconnected. A set resolved at connect time could not have done
// that: it would have had to be either chat-a's set (missing chat-new) or
// chat-new's (compiled too late for chat-a).
func TestFilesFanout_AChatForkedAfterAClientConnectedWidensTheSet(t *testing.T) {
	c, srv, resolver := chatScopeEnv(t)

	established := dialWSAt(t, srv, "/v0/chats/chat-a/files/ws")
	c.files.WaitNRegistered(1)

	resolver.fork("chat-new", "chat-a")
	newcomer := dialWSAt(t, srv, "/v0/chats/chat-new/files/ws")
	c.files.WaitNRegistered(1)

	c.PushFile(fileChange("ws-a", "after-the-fork.go"))

	assert.Equal(t, "after-the-fork.go", readJSON(t, newcomer)["path"],
		"a chat forked onto the worktree after this stream opened must receive its changes")
	assert.Equal(t, "after-the-fork.go", readJSON(t, established)["path"],
		"and the chat that was already listening must not have been dropped to make room")
}

// TestFilesPush_CarriesTheFanoutSetResolvedAtPushTime asserts the wiring this
// step adds at its own seam, so a failure points at PushFile rather than at
// whichever client stopped receiving frames.
//
// It also pins the degradation: a workspace no chat points at fans out to
// nobody rather than erroring, which leaves the wsId-scoped subscribers on that
// same event untouched.
func TestFilesPush_CarriesTheFanoutSetResolvedAtPushTime(t *testing.T) {
	a := newAppForSnapshot(t)
	a.Usecases.Worktree = newRowsResolver(a)
	c := New(a, nil)

	assert.Equal(t, []string{"chat-a", "chat-b", "chat-c"}, c.chatsHolding(context.Background(), "ws-a"))
	assert.Empty(t, c.chatsHolding(context.Background(), "ws-nobody-holds"),
		"a workspace no chat points at fans out to nobody, and is not an error")
}

// TestFilesWireShape_TheFanoutSetIsNeverSerialized is a guard the delivery
// tests above cannot give. This topic serializes the WHOLE event — unlike git's,
// which serializes only its embedded Status — so the chat roster is kept off the
// wire by a json tag alone, and dropping that tag would leak every sibling
// chat's id to every file-tree client on every keystroke-driven save.
func TestFilesWireShape_TheFanoutSetIsNeverSerialized(t *testing.T) {
	data, err := filesDef().Serialize(domain.FileChangeEvent{
		Type:    domain.FileChangeModified,
		WsID:    "ws-a",
		Path:    "a.go",
		ChatIDs: []string{"chat-a", "chat-b", "chat-c"},
	})
	require.NoError(t, err)

	assert.Contains(t, string(data), "a.go", "the change itself is the payload")
	assert.NotContains(t, string(data), "chat-a",
		"the fan-out set is routing, not payload: a file-tree client is never handed a chat roster")
}
