package agent_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIngestHookDelivery_DuplicatePOSTMutatesLedgerOnce(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	userDelivery := uuid.NewString()
	userPayload := mustJSON(t, map[string]any{"prompt": "exactly once"})

	for range 2 {
		require.NoError(t, f.usecase.IngestHookDelivery(
			f.ctx, "ws1", userDelivery, runnerID, "codex", "user_prompt", userPayload,
		))
	}
	stopDelivery := uuid.NewString()
	stopPayload := mustJSON(t, map[string]any{"last_assistant_message": "one reply"})
	for range 2 {
		require.NoError(t, f.usecase.IngestHookDelivery(
			f.ctx, "ws1", stopDelivery, runnerID, "codex", "turn_stop", stopPayload,
		))
	}
	f.wait()

	page, err := f.usecase.ReadMessages(f.ctx, chatID, 0, 0, 100)
	require.NoError(t, err)
	require.Len(t, page.Items, 2)
	assert.Equal(t, "exactly once", page.Items[0].Text)
	assert.Equal(t, "one reply", page.Items[1].Text)
	assert.False(t, f.chat(t, chatID).Working)
}

func TestIngestHookDelivery_RejectsUUIDReuseWithDifferentPayload(t *testing.T) {
	f := newFixture(t)
	_, runnerID := f.spawn(t, "codex")
	deliveryID := uuid.NewString()
	require.NoError(t, f.usecase.IngestHookDelivery(
		f.ctx, "ws1", deliveryID, runnerID, "codex", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "first"}),
	))

	err := f.usecase.IngestHookDelivery(
		f.ctx, "ws1", deliveryID, runnerID, "codex", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "different"}),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "different payload")
}
