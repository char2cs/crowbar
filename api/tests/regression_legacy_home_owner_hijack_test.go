//go:build integration

package tests

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A project home from before the chat-first mint (or one resolveHome
// provisioned lazily, which mints no owner at all) has real user conversations
// and no row that records ownership. ResolveOwningChat's legacy fallback then
// elects the EARLIEST conversation as the home's owner: GET /home names it,
// the home chat list marks it as owning the home worktree, and the sidebar
// (rows-from-home.ts) hides it as the home's ghost owner. ensureOwner never
// mints a real owner because the heuristic already answered.
func TestRegression_LegacyHomeWithoutOwnerDoesNotHijackAUserChat(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	p := imported.projectID
	homePath := "/v0/projects/" + p + "/home"

	var home struct {
		ID           string `json:"id"`
		OwningChatID string `json:"owningChatId"`
	}
	h.get(homePath, &home)
	require.NotEmpty(t, home.OwningChatID)

	// The user's conversations, titled the way a turn's derived title would.
	c1 := createHomeChatStub(t, h, p)
	c2 := createHomeChatStub(t, h, p)
	h.raw(http.MethodPost, homePath+"/chats/"+c1+"/rename", map[string]string{"title": "plan the release"}, http.StatusAccepted).Body.Close()
	h.raw(http.MethodPost, homePath+"/chats/"+c2+"/rename", map[string]string{"title": "triage bugs"}, http.StatusAccepted).Body.Close()
	h.Quiesce()

	// The legacy shape: the home's owner row never existed.
	ctx := context.Background()
	require.NoError(t, h.app.Usecases.AgentChat.PurgeChat(ctx, home.OwningChatID))
	h.Quiesce()

	var rows []agentChatDTO
	h.get(homePath+"/chats", &rows)
	for _, r := range rows {
		if r.ID != c1 && r.ID != c2 {
			continue
		}
		if assert.NotNil(t, r.Worktree, "home chat %s must carry the home worktree", r.Title) {
			assert.NotEqual(t, r.ID, r.Worktree.OwningChatID,
				"user conversation %q must not be served as the home's owner", r.Title)
		}
	}

	var after struct {
		OwningChatID string `json:"owningChatId"`
	}
	h.get(homePath, &after)
	assert.NotEqual(t, c1, after.OwningChatID, "GET /home must not name a user conversation as the owner")
	assert.NotEqual(t, c2, after.OwningChatID, "GET /home must not name a user conversation as the owner")
}
