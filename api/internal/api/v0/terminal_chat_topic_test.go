package v0

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/api/v0/ws"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/engine"
)

// subscriberOnChat builds the gin context a client gets when it upgrades
// GET /v0/chats/:chatId/terminals — the ONLY path param that route binds.
func subscriberOnChat(chatID string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v0/chats/"+chatID+"/terminals", nil)
	c.Params = gin.Params{{Key: "chatId", Value: chatID}}
	return c
}

// TestRegression_TerminalTopicDoesNotLeakAcrossChats is the broadcast half of
// the workspace→chat re-key, and it exercises the compiled PREDICATE a real
// subscriber gets — not just the StreamDef's field extractors, which would pass
// even if the predicate ignored them.
//
// THE TRAP: terminals used to be a hierarchical stream, scoped by the
// projectId/repoId/wsId path params its old route nested under. The flat chat
// route binds NONE of those three, so a client's hierarchical scope prefix
// comes out empty — and an empty prefix matches EVERY namespace, which would
// quietly turn this feed into a firehose handing every subscriber every other
// chat's frames.
//
// MEASURED, not assumed: deleting the chatId Filter makes this test fail;
// deleting FlatNamespace alone does NOT, because ws.clientScope falls back to
// the bare chatId and prefix-matching a flat chat namespace against it scopes
// correctly too. The two guards are independent, and the Filter is the one this
// test pins. FlatNamespace's own consequence is covered by the snapshot test
// below, which is the path that reads the scope string directly.
func TestRegression_TerminalTopicDoesNotLeakAcrossChats(t *testing.T) {
	def := terminalsDef(nil, nil)
	predicate := ws.BuildPredicate(subscriberOnChat("chat-a"), def)

	own := dto.TerminalSessionDTO{ID: "sess-a", ChatID: "chat-a", Status: "active"}
	sibling := dto.TerminalSessionDTO{ID: "sess-b", ChatID: "chat-b", Status: "active"}

	assert.True(t, predicate(own), "a chat must receive its own session frames")
	assert.False(t, predicate(sibling),
		"a chat must NOT receive a sibling chat's session frames, even one sharing its worktree")
}

// TestRegression_TerminalTopicScopesEveryChatToItself walks several chats
// through the same predicate construction, so a predicate that happened to
// hard-code one id, or that matched on something other than the chat, cannot
// pass by luck.
func TestRegression_TerminalTopicScopesEveryChatToItself(t *testing.T) {
	def := terminalsDef(nil, nil)
	chats := []string{"chat-a", "chat-b", "chat-c"}

	for _, subscriber := range chats {
		predicate := ws.BuildPredicate(subscriberOnChat(subscriber), def)
		for _, owner := range chats {
			frame := dto.TerminalSessionDTO{ID: "sess-" + owner, ChatID: owner}
			assert.Equal(t, subscriber == owner, predicate(frame),
				"subscriber %s vs frame owned by %s", subscriber, owner)
		}
	}
}

// TestTerminalSessionDTO_CarriesNoWorkspaceIdentity guards spec §6's rejected
// option — "keep workspaceId as a public, read-only field" — at the wire.
//
// A workspace id on the frame would let a consumer name a resource independent
// of any chat, which is the exact thing law 1 exists to prevent, and it would
// also re-open the possibility of a subscriber filtering by workspace and
// getting a sibling's sessions back.
func TestTerminalSessionDTO_CarriesNoWorkspaceIdentity(t *testing.T) {
	def := terminalsDef(nil, nil)

	payload, err := def.Serialize(dto.TerminalSessionDTO{ID: "sess-1", ChatID: "chat-1"})
	require.NoError(t, err)

	body := string(payload)
	assert.Contains(t, body, `"chatId":"chat-1"`)
	assert.NotContains(t, body, "workspaceId")
	assert.NotContains(t, body, "projectId")
	assert.NotContains(t, body, "repoId")
}

// TestRegression_TerminalSnapshotOnSubscribeIsChatScoped covers the OTHER half
// of a subscription: the snapshot-on-subscribe replay, which does not go
// through the predicate at all. It is handed ws.clientScope's scope string
// directly, so it is the path that depends on that function resolving a flat
// chat route to a bare chat id.
//
// A leak here would be invisible to the predicate tests above: a client would
// pass its own filter on every LIVE frame and still be handed every other
// chat's sessions in the initial replay.
func TestRegression_TerminalSnapshotOnSubscribeIsChatScoped(t *testing.T) {
	eng := engineterminal.New()
	t.Cleanup(eng.Shutdown)

	// One shared worktree, two sibling chats — the case the re-key exists for.
	shared := t.TempDir()
	sidA, err := eng.Create(context.Background(), "chat-a", shared, nil)
	require.NoError(t, err)
	sidB, err := eng.Create(context.Background(), "chat-b", shared, nil)
	require.NoError(t, err)

	snapshot := terminalsSnapshot(nil, &engine.Container{Terminal: eng})
	require.NotNil(t, snapshot)

	ownedByA := snapshot("chat-a")
	require.Len(t, ownedByA, 1, "chat-a's replay must hold exactly its own session")
	assert.Equal(t, sidA, ownedByA[0].ID)
	assert.Equal(t, "chat-a", ownedByA[0].ChatID)

	ownedByB := snapshot("chat-b")
	require.Len(t, ownedByB, 1, "chat-b's replay must hold exactly its own session")
	assert.Equal(t, sidB, ownedByB[0].ID)

	// An unknown chat gets nothing — never a whole-registry scan.
	assert.Empty(t, snapshot("chat-unknown"))
	assert.Empty(t, snapshot(""))
}
