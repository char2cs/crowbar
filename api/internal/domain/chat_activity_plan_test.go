package domain_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestActivityTurn_LegacyJSONWithoutPlanDecodesUnchanged(t *testing.T) {
	const legacy = `{
		"id":"turn-1",
		"chatId":"chat-1",
		"seq":3,
		"role":"assistant",
		"text":"done",
		"startedAt":"2026-08-17T09:00:00Z"
	}`

	var turn domain.ActivityTurn
	require.NoError(t, json.Unmarshal([]byte(legacy), &turn))
	assert.Equal(t, "turn-1", turn.ID)
	assert.Equal(t, "done", turn.Text)
	assert.Empty(t, turn.Plan)
	assert.Nil(t, turn.PlanUpdatedAt)
}
