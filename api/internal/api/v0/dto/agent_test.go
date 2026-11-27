package dto_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/engine/agents"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestAgentChatDTOFrom_CarriesIdentityAndTitle(t *testing.T) {
	created := time.Unix(1, 0).UTC()
	got := dto.AgentChatDTOFrom(domain.Chat{
		ID:          "c1",
		WorkspaceID: "ws1",
		Title:       "a title",
		CreatedAt:   created,
	}, dto.ChatRuntime{}, nil)

	assert.Equal(t, "c1", got.ID)
	assert.Equal(t, "ws1", got.WorkspaceID)
	assert.Equal(t, "a title", got.Title)
	assert.Equal(t, created, got.CreatedAt)
}

func TestAgentChatDTOFrom_LiveRunnerIsTheLivenessAnswer(t *testing.T) {
	got := dto.AgentChatDTOFrom(domain.Chat{ID: "c1"}, dto.ChatRuntime{
		LiveRunner: &agents.Runner{
			ID:              "run-1",
			ProviderID:      "vendor-b",
			TerminalSession: "term-1",
			CurrentChatID:   "c1",
		},
		Conversations: []agents.ChatConversation{{ChatID: "c1", ProviderID: "vendor-a"}},
	}, nil)

	assert.Equal(t, "run-1", got.LiveRunnerID)
	assert.Equal(t, "term-1", got.TerminalSessionID)
	assert.Equal(t, "vendor-b", got.ActiveProviderID)
}

func TestAgentChatDTOFrom_NeverRanIsAllEmpty(t *testing.T) {
	got := dto.AgentChatDTOFrom(domain.Chat{ID: "c1"}, dto.ChatRuntime{}, nil)

	assert.Empty(t, got.LiveRunnerID)
	assert.Empty(t, got.TerminalSessionID)
	assert.Empty(t, got.ActiveProviderID)
}

func TestAgentChatDTOFrom_CarriesTheRowType(t *testing.T) {
	got := dto.AgentChatDTOFrom(domain.Chat{ID: "c1", Type: domain.ChatTypeBranch},
		dto.ChatRuntime{}, nil)

	assert.Equal(t, domain.ChatTypeBranch, got.Type)

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"type":"branch"`)
}

func TestAgentChatDTOList_IsNonNil(t *testing.T) {
	got := dto.AgentChatDTOList(nil, nil, nil)
	require.NotNil(t, got)
	assert.Empty(t, got)

	got = dto.AgentChatDTOList(
		[]domain.Chat{{ID: "c1"}, {ID: "c2"}},
		map[string]dto.ChatRuntime{
			"c1": {LiveRunner: &agents.Runner{ID: "run-1", ProviderID: "vendor-a", TerminalSession: "term-1"}},
		},
		nil,
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

func TestAgentChatDetailDTOFrom_CarriesConversations(t *testing.T) {
	convs := []agents.ChatConversation{
		{ChatID: "c1", ProviderID: "vendor-a", SessionID: "sess-1", FirstSeenAt: time.Unix(1, 0).UTC()},
		{ChatID: "c1", ProviderID: "vendor-b", SessionID: "sess-2", FirstSeenAt: time.Unix(2, 0).UTC()},
	}
	got := dto.AgentChatDetailDTOFrom(
		domain.Chat{ID: "c1", WorkspaceID: "ws1"},
		dto.ChatRuntime{
			LiveRunner:    &agents.Runner{ID: "run-1", ProviderID: "vendor-b", TerminalSession: "term-1"},
			Conversations: convs,
		},
		nil,
	)

	assert.Equal(t, "c1", got.ID)
	assert.Equal(t, "run-1", got.LiveRunnerID)
	assert.Equal(t, "term-1", got.TerminalSessionID)
	assert.Equal(t, convs, got.Conversations)
}

func TestAgentChatDetailDTOFrom_ConversationsNeverNull(t *testing.T) {
	got := dto.AgentChatDetailDTOFrom(domain.Chat{ID: "c1"}, dto.ChatRuntime{}, nil)

	require.NotNil(t, got.Conversations)
	assert.Empty(t, got.Conversations)

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"conversations":[]`)
	assert.NotContains(t, string(raw), `"segments"`)
}

func TestAgentChatDTOFrom_CarriesTheStickySelection(t *testing.T) {
	got := dto.AgentChatDTOFrom(domain.Chat{ID: "c1", Model: "opus", Effort: "high"},
		dto.ChatRuntime{}, nil)

	assert.Equal(t, "opus", got.Model)
	assert.Equal(t, "high", got.Effort)
}

func TestAgentChatDTOFrom_UnselectedChatOmitsTheSelection(t *testing.T) {
	got := dto.AgentChatDTOFrom(domain.Chat{ID: "c1"}, dto.ChatRuntime{}, nil)

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), `"model"`)
	assert.NotContains(t, string(raw), `"effort"`)
}

func TestTerminalWaitDTOFrom_NotWaitingIsNil(t *testing.T) {
	got := dto.TerminalWaitDTOFrom(domain.AgentTerminalWait{})

	assert.Nil(t, got)
}

func TestTerminalWaitDTOFrom_WaitingCarriesKind(t *testing.T) {
	got := dto.TerminalWaitDTOFrom(domain.AgentTerminalWait{
		Waiting: true,
		Kind:    domain.AgentTerminalWaitTrust,
	})

	require.NotNil(t, got)
	assert.Equal(t, domain.AgentTerminalWaitTrust, got.Kind)
}

func TestAgentChatDTOFrom_TerminalWaitOmittedWhenNotWaiting(t *testing.T) {
	got := dto.AgentChatDTOFrom(domain.Chat{ID: "c1"}, dto.ChatRuntime{}, nil)

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), `"terminalWait"`)
}

func TestAgentChatDTOFrom_TerminalWaitCarriesKindWhenWaiting(t *testing.T) {
	got := dto.AgentChatDTOFrom(domain.Chat{ID: "c1"}, dto.ChatRuntime{
		TerminalWait: domain.AgentTerminalWait{
			Waiting: true,
			Kind:    domain.AgentTerminalWaitTrust,
		},
	}, nil)

	require.NotNil(t, got.TerminalWait)
	assert.Equal(t, domain.AgentTerminalWaitTrust, got.TerminalWait.Kind)

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"terminalWait":{"kind":"workspace_trust"}`)
}

func TestAgentChatDTOFrom_UnidentifiedTerminalWaitMarshalsAsEmptyObject(t *testing.T) {
	got := dto.AgentChatDTOFrom(domain.Chat{ID: "c1"}, dto.ChatRuntime{
		TerminalWait: domain.AgentTerminalWait{Waiting: true},
	}, nil)

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"terminalWait":{}`)
}

func TestRegression_AgentChatDTOFrom_BornOnAProviderThatRecordedNothingIsStillNamed(t *testing.T) {
	got := dto.AgentChatDTOFrom(
		domain.Chat{ID: "c1", ProviderID: "codex"}, dto.ChatRuntime{}, nil)

	assert.Empty(t, got.LiveRunnerID, "the chat must be dormant for this to prove anything")
	assert.Equal(t, "codex", got.ActiveProviderID,
		"a dormant chat that recorded nothing still knows what it runs; \"\" is what let the UI guess")
}

func TestAgentChatDTOFrom_CarriesTheConversationAndOmitsAnUnboundOne(t *testing.T) {
	chat := domain.Chat{ID: "c1", ProviderID: "claude"}
	for _, state := range []ledger.ConversationState{ledger.ConversationBound, ledger.ConversationLost} {
		got := dto.AgentChatDTOFrom(chat, dto.ChatRuntime{Conversation: ledger.Conversation{
			Provider: "claude", SessionID: "S", State: state,
		}}, nil)
		assert.Equal(t, &dto.AgentConversationDTO{ProviderID: "claude", SessionID: "S", State: string(state)}, got.Conversation)
	}

	raw, err := json.Marshal(dto.AgentChatDTOFrom(chat, dto.ChatRuntime{}, nil))
	require.NoError(t, err)
	assert.NotContains(t, string(raw), `"conversation"`)
}

func TestAgentChatDTOFrom_TheReportedLaunchOutranksTheRequestedOne(t *testing.T) {
	got := dto.AgentChatDTOFrom(domain.Chat{ID: "c1"}, dto.ChatRuntime{LiveRunner: &agents.Runner{
		ID: "run-1", LaunchModel: "opus", ReportedModel: "claude-opus-4", LaunchEffort: "low",
	}}, nil)

	assert.Equal(t, "claude-opus-4", got.LaunchModel)
	assert.Equal(t, "low", got.LaunchEffort, "an unreported half falls back to the launch")
}

func TestTerminalWaitDTOFrom_AnswersExposeIDAndLabelOnly(t *testing.T) {
	got := dto.TerminalWaitDTOFrom(domain.AgentTerminalWait{
		Waiting: true,
		Answers: []domain.AgentTerminalAnswer{{ID: "accept", Label: "Trust workspace", Keys: "\r"}},
	})

	require.NotNil(t, got)
	assert.Equal(t, []dto.AgentTerminalAnswerDTO{{ID: "accept", Label: "Trust workspace"}}, got.Answers)
}

func TestAgentChatDTOFrom_CarriesOnlyAPinnedPermissionLevel(t *testing.T) {
	pinned := dto.AgentChatDTOFrom(domain.Chat{ID: "c1", PermissionLevel: "trusted", PermissionLevelExplicit: true},
		dto.ChatRuntime{}, nil)
	assert.Equal(t, "trusted", pinned.PermissionLevel)

	inherited := dto.AgentChatDTOFrom(domain.Chat{ID: "c2", PermissionLevel: "guarded"}, dto.ChatRuntime{}, nil)
	assert.Empty(t, inherited.PermissionLevel)
}
