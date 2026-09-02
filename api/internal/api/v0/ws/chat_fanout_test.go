package ws

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// sharedEvent stands in for a shared-bucket frame (git status, a file change, a
// review update): the workspace it describes, plus the chat ids the PUSHING
// side resolved for that workspace.
type sharedEvent struct {
	wsID    string
	chatIDs []string
}

func sharedBucketDef() StreamDef[sharedEvent] {
	return StreamDef[sharedEvent]{
		Namespace:     func(e sharedEvent) string { return e.wsID },
		FlatNamespace: true,
		Filters: []FilterDef[sharedEvent]{
			ChatFanoutFilter(func(e sharedEvent) []string { return e.chatIDs }),
		},
	}
}

func sharedWorkspaceEvent() sharedEvent {
	return sharedEvent{wsID: "ws-a", chatIDs: []string{"chat-a", "chat-b", "chat-c"}}
}

// TestChatFanoutFilter_EveryChatSharingTheWorktreeMatchesOnePush is the whole
// mechanism: ONE event, one Serialize, one fan-out pass, and every sibling
// chat's predicate says yes.
func TestChatFanoutFilter_EveryChatSharingTheWorktreeMatchesOnePush(t *testing.T) {
	event := sharedWorkspaceEvent()

	for _, chatID := range []string{"chat-a", "chat-b", "chat-c"} {
		p := BuildPredicate(chatScopeCtx(gin.Params{{Key: "chatId", Value: chatID}}), sharedBucketDef())
		assert.True(t, p(event), "%s shares ws-a and must receive its push", chatID)
	}
}

// TestChatFanoutFilter_AChatOutsideTheResolvedSetIsRefused: chat-z resolves to
// another worktree, so a write to ws-a is none of its business.
func TestChatFanoutFilter_AChatOutsideTheResolvedSetIsRefused(t *testing.T) {
	p := BuildPredicate(chatScopeCtx(gin.Params{{Key: "chatId", Value: "chat-z"}}), sharedBucketDef())

	assert.False(t, p(sharedWorkspaceEvent()))
}

// TestChatFanoutFilter_AnEmptyResolvedSetReachesNobody pins that a push for a
// workspace nobody currently resolves to is delivered to nobody — never to
// everybody, the way an inactive filter would.
func TestChatFanoutFilter_AnEmptyResolvedSetReachesNobody(t *testing.T) {
	p := BuildPredicate(chatScopeCtx(gin.Params{{Key: "chatId", Value: "chat-a"}}), sharedBucketDef())

	assert.False(t, p(sharedEvent{wsID: "ws-nobody", chatIDs: nil}))
	assert.False(t, p(sharedEvent{wsID: "ws-nobody", chatIDs: []string{}}))
}

// TestChatFanoutFilter_AClientCarryingNoChatIDIsRefusedNotOverSubscribed is the
// trap closed. resolveFilterValue answers "" for a client whose request names
// the filter's Param nowhere, and collectFilters used to DROP such a filter —
// turning it into a no-op that matches every event on the stream. Required
// makes the same client match nothing instead.
func TestChatFanoutFilter_AClientCarryingNoChatIDIsRefusedNotOverSubscribed(t *testing.T) {
	p := BuildPredicate(ctxWith("", ""), sharedBucketDef())

	assert.False(t, p(sharedWorkspaceEvent()))
	assert.False(t, p(sharedEvent{wsID: "ws-other", chatIDs: []string{"chat-q"}}))
}

// TestBuildPredicate_AWsIdFilterHandsAChatScopedClientEveryWorkspace is the
// trap itself, pinned as the reason ChatFanoutFilter exists. A chat-scoped
// route binds no :wsId, so the wsId Filter every shared-bucket stream carries
// TODAY resolves empty for such a client and goes inactive — leaving it
// subscribed to every workspace on the daemon rather than to none. This is why
// re-keying those routes cannot simply reuse the existing Filter.
func TestBuildPredicate_AWsIdFilterHandsAChatScopedClientEveryWorkspace(t *testing.T) {
	p := BuildPredicate(chatScopeCtx(gin.Params{{Key: "chatId", Value: "chat-a"}}), flatDef())

	assert.True(t, p(row{name: "A"}))
	assert.True(t, p(row{name: "B"}), "the wsId filter is inactive, so every workspace matches")
}

// TestChatFanoutFilter_ResolvesTheChatIDFromAQueryParamToo keeps the
// non-nested caller (?chatId=) working, the same way resolveFilterValue already
// serves the dual-served workspace routes from either shape.
func TestChatFanoutFilter_ResolvesTheChatIDFromAQueryParamToo(t *testing.T) {
	p := BuildPredicate(ctxWith("chatId=chat-b", ""), sharedBucketDef())

	assert.True(t, p(sharedWorkspaceEvent()))
	assert.False(t, p(sharedEvent{wsID: "ws-other", chatIDs: []string{"chat-z"}}))
}

// TestChatFanoutFilter_ASharedStreamMustStayFlatNamespace pins the second half
// of the contract: the events are namespaced by WORKSPACE while a chat-scoped
// client's namespace scope is its bare chat id, so a hierarchical stream would
// prefix-match "chat-a" against "ws-a" and drop every frame — a silent total
// blackout rather than an over-subscription.
func TestChatFanoutFilter_ASharedStreamMustStayFlatNamespace(t *testing.T) {
	hierarchical := sharedBucketDef()
	hierarchical.FlatNamespace = false

	p := BuildPredicate(chatScopeCtx(gin.Params{{Key: "chatId", Value: "chat-a"}}), hierarchical)

	assert.False(t, p(sharedWorkspaceEvent()))
}

// TestChatFanoutFilter_MembershipIsExactNotAPrefix guards the cheap wrong
// implementation (a joined string plus strings.Contains): chat-a1 is not
// chat-a.
func TestChatFanoutFilter_MembershipIsExactNotAPrefix(t *testing.T) {
	p := BuildPredicate(chatScopeCtx(gin.Params{{Key: "chatId", Value: "chat-a"}}), sharedBucketDef())

	assert.False(t, p(sharedEvent{wsID: "ws-a", chatIDs: []string{"chat-a1", "chat-ab"}}))
}
