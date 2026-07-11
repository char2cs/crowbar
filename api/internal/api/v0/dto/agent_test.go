package dto_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestAgentChatDTOFrom_DerivesActiveProviderID(
	t *testing.T,
) {
	chat := domain.AgentChat{
		ID:              "c1",
		ActiveSegmentID: "s2",
		Segments: []domain.AgentSegment{
			{ID: "s1", ProviderID: "claude", Status: "ended"},
			{ID: "s2", ProviderID: "codex", Status: "active"},
		},
	}
	got := dto.AgentChatDTOFrom(chat)
	assert.Equal(t, "codex", got.ActiveProviderID)
}

func TestAgentChatDTOFrom_EmptyWhenNoActiveSegment(
	t *testing.T,
) {
	got := dto.AgentChatDTOFrom(domain.AgentChat{ID: "c1", ActiveSegmentID: ""})
	assert.Equal(t, "", got.ActiveProviderID)
}
