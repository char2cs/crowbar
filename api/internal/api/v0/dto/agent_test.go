package dto_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// The chat aggregate still carries no process facts — a chat does not own the CLI
// talking to it. The runner facts on the DTO are DERIVED at read time from the runner
// projections and handed in as a ChatRuntime; the chat itself supplies only identity,
// title and creation time.
func TestAgentChatDTOFrom_CarriesIdentityAndTitle(t *testing.T) {
	created := time.Unix(1, 0).UTC()
	got := dto.AgentChatDTOFrom(domain.AgentChat{
		ID:          "c1",
		WorkspaceID: "ws1",
		Title:       "a title",
		CreatedAt:   created,
	}, dto.ChatRuntime{})

	assert.Equal(t, "c1", got.ID)
	assert.Equal(t, "ws1", got.WorkspaceID)
	assert.Equal(t, "a title", got.Title)
	assert.Equal(t, created, got.CreatedAt)
}

// TestAgentChatDTOFrom_LiveRunnerIsTheLivenessAnswer proves the live join: the runner
// placed on the chat supplies liveRunnerId (which IS the liveness answer — no status
// field exists to disagree with it), its PTY, and the provider. Its provider outranks
// the history, whose last entry can still name the OUTGOING vendor mid-switch.
func TestAgentChatDTOFrom_LiveRunnerIsTheLivenessAnswer(t *testing.T) {
	got := dto.AgentChatDTOFrom(domain.AgentChat{ID: "c1"}, dto.ChatRuntime{
		LiveRunner: &domain.AgentRunner{
			ID:              "run-1",
			ProviderID:      "vendor-b",
			TerminalSession: "term-1",
			CurrentChatID:   "c1",
		},
		Conversations: []domain.ChatConversation{{ChatID: "c1", ProviderID: "vendor-a"}},
	})

	assert.Equal(t, "run-1", got.LiveRunnerID)
	assert.Equal(t, "term-1", got.TerminalSessionID)
	assert.Equal(t, "vendor-b", got.ActiveProviderID)
}

// TestAgentChatDTOFrom_DormantFallsBackToLastConversation proves the fallback that keeps
// a dormant chat legible: no live runner means no runner id and no PTY, but
// activeProviderId still resolves — off the LAST conversation (history is oldest-first)
// — so the provider dropdown shows the right vendor and Resume knows who to bring back.
func TestAgentChatDTOFrom_DormantFallsBackToLastConversation(t *testing.T) {
	got := dto.AgentChatDTOFrom(domain.AgentChat{ID: "c1"}, dto.ChatRuntime{
		Conversations: []domain.ChatConversation{
			{ChatID: "c1", ProviderID: "vendor-a", FirstSeenAt: time.Unix(1, 0).UTC()},
			{ChatID: "c1", ProviderID: "vendor-b", FirstSeenAt: time.Unix(2, 0).UTC()},
		},
	})

	assert.Empty(t, got.LiveRunnerID)
	assert.Empty(t, got.TerminalSessionID)
	assert.Equal(t, "vendor-b", got.ActiveProviderID)
}

// TestAgentChatDTOFrom_NeverRanIsAllEmpty proves a chat no runner has ever been placed
// on derives to empty strings everywhere rather than erroring or inventing a provider.
func TestAgentChatDTOFrom_NeverRanIsAllEmpty(t *testing.T) {
	got := dto.AgentChatDTOFrom(domain.AgentChat{ID: "c1"}, dto.ChatRuntime{})

	assert.Empty(t, got.LiveRunnerID)
	assert.Empty(t, got.TerminalSessionID)
	assert.Empty(t, got.ActiveProviderID)
}

// TestAgentChatDTOList_IsNonNil keeps the envelope carrying [] rather than null, and
// proves each row derives from ITS OWN runtime: the map is keyed by chat id, and a chat
// missing from it (in neither runner projection) reads as dormant with no history.
func TestAgentChatDTOList_IsNonNil(t *testing.T) {
	got := dto.AgentChatDTOList(nil, nil)
	require.NotNil(t, got)
	assert.Empty(t, got)

	got = dto.AgentChatDTOList(
		[]domain.AgentChat{{ID: "c1"}, {ID: "c2"}},
		map[string]dto.ChatRuntime{
			"c1": {LiveRunner: &domain.AgentRunner{ID: "run-1", ProviderID: "vendor-a", TerminalSession: "term-1"}},
		},
	)
	require.Len(t, got, 2)
	assert.Equal(t, "c1", got[0].ID)
	assert.Equal(t, "run-1", got[0].LiveRunnerID)
	assert.Equal(t, "term-1", got[0].TerminalSessionID)
	assert.Equal(t, "vendor-a", got[0].ActiveProviderID)

	assert.Equal(t, "c2", got[1].ID)
	assert.Empty(t, got[1].LiveRunnerID, "a chat in neither projection is dormant, not an error")
	assert.Empty(t, got[1].ActiveProviderID)
}

// TestAgentChatDetailDTOFrom_CarriesConversations proves the detail shape returns the
// chat's append-only conversation history (the successor of the deleted `segments`)
// alongside the same derived runner facts the list rows carry.
func TestAgentChatDetailDTOFrom_CarriesConversations(t *testing.T) {
	convs := []domain.ChatConversation{
		{ChatID: "c1", ProviderID: "vendor-a", SessionID: "sess-1", FirstSeenAt: time.Unix(1, 0).UTC()},
		{ChatID: "c1", ProviderID: "vendor-b", SessionID: "sess-2", FirstSeenAt: time.Unix(2, 0).UTC()},
	}
	got := dto.AgentChatDetailDTOFrom(
		domain.AgentChat{ID: "c1", WorkspaceID: "ws1"},
		dto.ChatRuntime{
			LiveRunner:    &domain.AgentRunner{ID: "run-1", ProviderID: "vendor-b", TerminalSession: "term-1"},
			Conversations: convs,
		},
	)

	assert.Equal(t, "c1", got.ID)
	assert.Equal(t, "run-1", got.LiveRunnerID)
	assert.Equal(t, "term-1", got.TerminalSessionID)
	assert.Equal(t, convs, got.Conversations)
}

// TestAgentChatDetailDTOFrom_ConversationsNeverNull keeps the envelope carrying [] on a
// chat that has hosted no conversation yet, so the FE can map over it unguarded.
func TestAgentChatDetailDTOFrom_ConversationsNeverNull(t *testing.T) {
	got := dto.AgentChatDetailDTOFrom(domain.AgentChat{ID: "c1"}, dto.ChatRuntime{})

	require.NotNil(t, got.Conversations)
	assert.Empty(t, got.Conversations)

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"conversations":[]`)
	assert.NotContains(t, string(raw), `"segments"`)
}
