package dto_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestChatDTOFrom(
	t *testing.T,
) {
	now := time.Unix(1, 0).UTC()
	got := dto.ChatDTOFrom(domain.Chat{
		ID:        "c1",
		WsID:      "w1",
		Title:     "title",
		ParentID:  "c0",
		Status:    domain.ChatStatusAgentRunning,
		Type:      domain.ChatTypeChat,
		CreatedAt: now,
	})
	assert.Equal(t, "c1", got.ID)
	assert.Equal(t, "w1", got.WsID)
	assert.Equal(t, "title", got.Title)
	assert.Equal(t, "c0", got.ParentID)
	assert.Equal(t, domain.ChatStatusAgentRunning, got.Status)
	assert.Equal(t, now, got.CreatedAt)
}

func TestChatDTOListEmptyNonNil(
	t *testing.T,
) {
	got := dto.ChatDTOList(nil)
	require.NotNil(t, got)
	assert.Len(t, got, 0)
}

func TestChatDTOList(
	t *testing.T,
) {
	got := dto.ChatDTOList([]domain.Chat{
		{ID: "c1"},
		{ID: "c2"},
	})
	require.Len(t, got, 2)
	assert.Equal(t, "c1", got[0].ID)
	assert.Equal(t, "c2", got[1].ID)
}
