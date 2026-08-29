//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegression_AgentChatActiveProviderID proves both GET .../chats (list)
// and GET .../chats/:id (detail) carry activeProviderId derived from the
// active segment, so the FE row glyph resolves with no extra fetch.
//
// It spawns LIVESTUB (`cat`), not stub (`true`), and that is load-bearing.
// activeProviderId is derived from the LIVE runner's provider; a `true` runner exits in
// microseconds and its exit projection deletes the runner row, so between the spawn and
// the GET the field would flip to "" and the test flakes (~1 run in 30). `cat` holds its
// PTY open, so the runner row survives the read and the assertion is deterministic.
func TestRegression_AgentChatActiveProviderID(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)

	var created struct {
		ID string `json:"id"`
	}
	h.post(repoBase(imported)+"/chats",
		map[string]string{"provider": "livestub", "workspaceId": imported.workspaceID},
		http.StatusCreated, &created)
	require.NotEmpty(t, created.ID, "create must respond with the new chat's id")
	// Join the reactors so the spawned runner has actually landed before the read (a plain
	// projection drain returns the moment the placement goroutine is spawned, before the
	// runner row exists — exactly when activeProviderId reads "").
	h.QuiesceReactors()
	chatID := created.ID

	var list []struct {
		ID               string `json:"id"`
		ActiveProviderID string `json:"activeProviderId"`
	}
	h.get(repoBase(imported)+"/chats", &list)
	require.Len(t, list, 1)
	assert.Equal(t, chatID, list[0].ID)
	assert.Equal(t, "livestub", list[0].ActiveProviderID)

	var detail struct {
		ActiveProviderID string `json:"activeProviderId"`
	}
	h.get(repoBase(imported)+"/chats/"+chatID, &detail)
	assert.Equal(t, "livestub", detail.ActiveProviderID)
}
