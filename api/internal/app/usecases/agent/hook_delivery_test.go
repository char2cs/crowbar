package agent_test

import (
	"strings"
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

func TestRegression_IngestHookDelivery_ARetriedDeliveryIDRunsItsEffectsOnce(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	deliveryID := uuid.NewString()
	payload := mustJSON(t, map[string]any{"last_assistant_message": "the reply"})

	for range 3 {
		require.NoError(t, f.usecase.IngestHookDelivery(
			f.ctx, "", deliveryID, runnerID, "claude", "turn_stop", payload))
	}
	f.wait()

	turns, err := f.activity.Turns(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	require.Len(t, turns, 1)
	assert.Equal(t, "the reply", turns[0].Text)
}

func TestIngestHookDelivery_DistinctDeliveryIDsAreDistinctTurns(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	payload := mustJSON(t, map[string]any{"last_assistant_message": "same words"})

	for range 2 {
		require.NoError(t, f.usecase.IngestHookDelivery(
			f.ctx, "", uuid.NewString(), runnerID, "claude", "turn_stop", payload))
	}
	f.wait()

	turns, err := f.activity.Turns(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	assert.Len(t, turns, 2)
}

func TestIngestHookDelivery_RefusesADeliveryIDThatIsNotACanonicalUUID(t *testing.T) {
	f := newFixture(t)
	_, runnerID := f.spawn(t, "claude")

	for _, id := range []string{"", "not-a-uuid", "  " + uuid.NewString(), strings.ToUpper(uuid.NewString())} {
		err := f.usecase.IngestHookDelivery(
			f.ctx, "", id, runnerID, "claude", "turn_stop", mustJSON(t, map[string]any{}))
		assert.Error(t, err, "delivery id %q", id)
	}
}

func TestIngestHookDelivery_AnUnknownRunnerIsDropped(t *testing.T) {
	f := newFixture(t)

	err := f.usecase.IngestHookDelivery(f.ctx, "", uuid.NewString(), uuid.NewString(),
		"claude", "turn_stop", mustJSON(t, map[string]any{"last_assistant_message": "x"}))

	assert.NoError(t, err)
}

func TestIngestHookDelivery_UsesTheRouteScopeWhenTheRunnerIsNotYetPersisted(t *testing.T) {
	f := newFixture(t)

	err := f.usecase.IngestHookDelivery(f.ctx, "ws1", uuid.NewString(), uuid.NewString(),
		"claude", "session_start", mustJSON(t, map[string]any{"session_id": "s1"}))

	assert.NoError(t, err)
}
