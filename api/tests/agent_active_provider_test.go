//go:build integration

package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegression_AgentChatActiveProviderID proves both GET .../agent/chats (list)
// and GET .../agent/chats/:id (detail) carry activeProviderId derived from the
// active segment, so the FE row glyph resolves with no extra fetch.
func TestRegression_AgentChatActiveProviderID(t *testing.T) {
	h := newHarness(t)
	writeStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	chatID := createAgentChat(t, h, imported) // spawns provider "stub"

	var list []struct {
		ID               string `json:"id"`
		ActiveProviderID string `json:"activeProviderId"`
	}
	h.get(wsBase(imported)+"/agent/chats", &list)
	require.Len(t, list, 1)
	assert.Equal(t, chatID, list[0].ID)
	assert.Equal(t, "stub", list[0].ActiveProviderID)

	var detail struct {
		ActiveProviderID string `json:"activeProviderId"`
	}
	h.get(wsBase(imported)+"/agent/chats/"+chatID, &detail)
	assert.Equal(t, "stub", detail.ActiveProviderID)
}
