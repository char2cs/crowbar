//go:build integration

package tests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// A legacy project home (its owner purged, no chats) is served by GET /home
// with no owner, and no read ever mints one — unlike GET .../workspaces —
// so every chat-keyed home surface (terminal tab, ensurePaneChatThenOpen)
// stays unaddressable for the session.
func TestRegression_LegacyHomeIsServedWithAnOwner(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	ctx := context.Background()

	var home struct {
		ID           string `json:"id"`
		OwningChatID string `json:"owningChatId"`
	}
	h.get("/v0/projects/"+imported.projectID+"/home", &home)
	require.NotEmpty(t, home.OwningChatID)
	require.NoError(t, h.app.Usecases.AgentChat.PurgeChat(ctx, home.OwningChatID))
	h.Quiesce()

	var rows []agentChatDTO
	h.get("/v0/projects/"+imported.projectID+"/home/chats", &rows)
	h.Quiesce()

	h.get("/v0/projects/"+imported.projectID+"/home", &home)
	require.NotEmpty(t, home.OwningChatID, "GET /home must mint (or resolve) an owner for a legacy chatless home")
}
