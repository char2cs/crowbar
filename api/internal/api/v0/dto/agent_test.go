package dto_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// The chat DTO carries no process facts any more. activeSegmentId/activeProviderId were
// derived from segments embedded in the chat aggregate, and there are none: a chat does
// not own the CLI talking to it. The runner pointed at the chat (a live row that exists
// exactly while its PTY does) and the chat's conversation history answer those questions
// now, and the DTO task later in this series joins them back on as liveRunnerId /
// terminalSessionId / activeProviderId.
func TestAgentChatDTOFrom_CarriesIdentityAndTitle(t *testing.T) {
	created := time.Unix(1, 0).UTC()
	got := dto.AgentChatDTOFrom(domain.AgentChat{
		ID:          "c1",
		WorkspaceID: "ws1",
		Title:       "a title",
		CreatedAt:   created,
	})

	assert.Equal(t, "c1", got.ID)
	assert.Equal(t, "ws1", got.WorkspaceID)
	assert.Equal(t, "a title", got.Title)
	assert.Equal(t, created, got.CreatedAt)
}

// TestAgentChatDTOList_IsNonNil keeps the envelope carrying [] rather than null.
func TestAgentChatDTOList_IsNonNil(t *testing.T) {
	got := dto.AgentChatDTOList(nil)
	require.NotNil(t, got)
	assert.Empty(t, got)

	got = dto.AgentChatDTOList([]domain.AgentChat{{ID: "c1"}, {ID: "c2"}})
	require.Len(t, got, 2)
	assert.Equal(t, "c1", got[0].ID)
}
