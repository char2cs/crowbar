package v0

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"
	"github.com/char2cs/crowbar/api/internal/domain"
	lspdomain "github.com/char2cs/crowbar/api/internal/domain/lsp"
)

// This file proves editor/LSP's move onto /v0/chats/:chatId end to end over
// the REAL delivery path — the real v0 Container, its real route
// registration, the real lspDef predicate compiled for each connecting
// client — reusing chatScopeEnv (git_chat_scope_test.go), the same real chat
// forest every chat-scoped group's tests share (chat-a and chat-b both hold
// ws-a; chat-z holds the unrelated ws-z).
//
// LSP diverges from git's shared bucket here, and that divergence is the
// whole point of this file: git's dual mount answers the SAME worktree
// identically through either route (TestGitCoexistence_
// OneBroadcasterServesBothRoutesFromOnePush), and a sibling chat sharing a
// worktree receives the SAME status (TestGitFanout_
// OnePushReachesEveryChatHoldingTheWorktree). Editor/LSP is spec §4.2's
// OWNED bucket instead: two chats holding the same worktree never share a
// session, so neither a sibling chat's diagnostics NOR the legacy
// workspace-scoped route's diagnostics ever reach a chat-scoped subscriber.

// TestLSPDelivery_ChatScopedClientReceivesItsOwnSessionDiagnostics is the
// baseline this file's isolation tests are contrasted against: a plain push
// under a chat's own key reaches that chat's subscriber.
func TestLSPDelivery_ChatScopedClientReceivesItsOwnSessionDiagnostics(t *testing.T) {
	c, srv, _ := chatScopeEnv(t)

	conn := dialWSAt(t, srv, "/v0/chats/chat-a/lsp/ws")
	c.lsp.WaitNRegistered(1)

	c.lsp.Push(lspdomain.DiagnosticsEvent{
		WsID:        "chat-a",
		Diagnostics: []lspdomain.Diagnostic{{FilePath: "main.go", Message: "oops"}},
	})

	got := readJSON(t, conn)
	assert.Equal(t, "chat-a", got["wsId"])
}

// TestLSPIsolation_ASiblingChatOnTheSameWorktreeDoesNotReceiveAnotherChatsSession
// pins the OWNED-bucket difference from git directly: chat-a and chat-b both
// hold ws-a in chatScopeEnv's own forest, but each opened its OWN LSP
// session (handlers.Handlers.lspOwnerID keys by chat id), so chat-b must
// never see chat-a's diagnostics even though they share a worktree.
//
// chat-b's own diagnostics are pushed FIRST, so the first frame chat-b's
// connection reads must be its own — a leaked chat-a frame would arrive
// first and fail the assertion, proving isolation without a timeout.
func TestLSPIsolation_ASiblingChatOnTheSameWorktreeDoesNotReceiveAnotherChatsSession(t *testing.T) {
	c, srv, _ := chatScopeEnv(t)

	owner := dialWSAt(t, srv, "/v0/chats/chat-a/lsp/ws")
	sibling := dialWSAt(t, srv, "/v0/chats/chat-b/lsp/ws")
	c.lsp.WaitNRegistered(2)

	c.lsp.Push(lspdomain.DiagnosticsEvent{WsID: "chat-b", Diagnostics: []lspdomain.Diagnostic{{FilePath: "b.go"}}})
	c.lsp.Push(lspdomain.DiagnosticsEvent{WsID: "chat-a", Diagnostics: []lspdomain.Diagnostic{{FilePath: "a.go"}}})

	assert.Equal(t, "chat-a", readJSON(t, owner)["wsId"])
	assert.Equal(t, "chat-b", readJSON(t, sibling)["wsId"],
		"a sibling chat sharing this worktree must see only its OWN LSP session, never chat-a's")
}

// TestLSPCoexistence_TheWorkspaceScopedRouteIsUnchanged is this step's
// regression bar, and the reason NEITHER of lspDef's filters is Required: the
// old workspace-scoped route (spec §8 step 6 retires it, not this step) binds
// :wsId and can never bind a :chatId, so a Required chatId filter would
// silently kill it.
func TestLSPCoexistence_TheWorkspaceScopedRouteIsUnchanged(t *testing.T) {
	c, srv, _ := chatScopeEnv(t)

	legacy := dialWSAt(t, srv, workspaceGitRoute+"ws-a/lsp/ws")
	c.lsp.WaitNRegistered(1)

	c.lsp.Push(lspdomain.DiagnosticsEvent{WsID: "ws-z", Diagnostics: []lspdomain.Diagnostic{{FilePath: "z.go"}}})
	c.lsp.Push(lspdomain.DiagnosticsEvent{WsID: "ws-a", Diagnostics: []lspdomain.Diagnostic{{FilePath: "a.go"}}})

	assert.Equal(t, "ws-a", readJSON(t, legacy)["wsId"],
		"a wsId-scoped client must still receive its own workspace, and only it")
}

// TestLSPCoexistence_EachMountAddressesItsOwnSessionKey pins the OWNED-bucket
// divergence from git's dual mount: unlike git/files, where one worktree
// answers identically through either route, editor/LSP's session key ITSELF
// differs by mount (handlers.Handlers.lspOwnerID) — the chat id on the new
// mount, the workspace id on the old one — so a diagnostics push under one
// key is invisible to a subscriber on the other, even though chat-a and the
// legacy ws-a route both resolve to the very same worktree.
func TestLSPCoexistence_EachMountAddressesItsOwnSessionKey(t *testing.T) {
	c, srv, _ := chatScopeEnv(t)

	viaChat := dialWSAt(t, srv, "/v0/chats/chat-a/lsp/ws")
	viaWorkspace := dialWSAt(t, srv, workspaceGitRoute+"ws-a/lsp/ws")
	c.lsp.WaitNRegistered(2)

	c.lsp.Push(lspdomain.DiagnosticsEvent{WsID: "ws-a", Diagnostics: []lspdomain.Diagnostic{{FilePath: "legacy.go"}}})
	c.lsp.Push(lspdomain.DiagnosticsEvent{WsID: "chat-a", Diagnostics: []lspdomain.Diagnostic{{FilePath: "new.go"}}})

	assert.Equal(t, "chat-a", readJSON(t, viaChat)["wsId"],
		"the chat-scoped client must see only its own chat-keyed session")
	assert.Equal(t, "ws-a", readJSON(t, viaWorkspace)["wsId"],
		"the workspace-scoped client must see only the legacy workspace-keyed session")
}

// TestLSPStreamScopeKey_AChatSubscriberRefcountsTheChatNotTheWorkspace pins
// the quietest thing in this step, and the one whose absence would look like
// nothing at all — the mirror image of git's TestGitStreamScopeKey_
// AChatSubscriberRefcountsTheResolvedWorkspace, with the OPPOSITE answer.
//
// The LSP stream's ScopeKey (scopeLSPOwnerID) names the key the LAZY LSP
// lifecycle (LSPManager, app/realtime) refcounts subscribers by; its last-
// unsubscribe edge calls engine.ReleaseWorkspace(ctx, key), which tears down
// every server pool entry whose key has that prefix. If a chat-scoped
// subscriber refcounted the resolved WORKSPACE (git's answer) rather than the
// chat itself, that teardown call would search for servers keyed by the
// workspace id while the actual pool entries editor's handlers created were
// keyed by the chat id (handlers.Handlers.lspOwnerID) — finding nothing, and
// leaking the chat's language-server processes forever.
func TestLSPStreamScopeKey_AChatSubscriberRefcountsTheChatNotTheWorkspace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a, eng := newAppAndEngine(t)
	def := withLSPLifecycle(lspDef(a, eng), a)
	require.NotNil(t, def.ScopeKey)

	viaChat, _ := gin.CreateTestContext(httptest.NewRecorder())
	viaChat.Request = httptest.NewRequest("GET", "/v0/chats/chat-a/lsp/ws", nil)
	viaChat.Params = gin.Params{{Key: "chatId", Value: "chat-a"}}
	reqscope.SetWorkspace(viaChat, domain.Workspace{ID: "ws-a"})

	assert.Equal(t, "chat-a", def.ScopeKey(viaChat),
		"an owned LSP session must be refcounted by the chat that opened it, not by the workspace it resolves to")

	viaWorkspace, _ := gin.CreateTestContext(httptest.NewRecorder())
	viaWorkspace.Request = httptest.NewRequest("GET", workspaceGitRoute+"ws-direct/lsp/ws", nil)
	viaWorkspace.Params = gin.Params{{Key: "wsId", Value: "ws-direct"}}

	assert.Equal(t, "ws-direct", def.ScopeKey(viaWorkspace),
		"the workspace-scoped route keeps naming its own workspace, unchanged")
}
