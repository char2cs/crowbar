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

// The relay retries with the SAME delivery id after a lost response. Replaying
// the effects would append the same turn twice, so the journal answers "already
// done" instead of re-running it.
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

// Two different deliveries are two different facts, even with identical content:
// a user really can say the same thing twice.
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

// The delivery id is Crowbar's own idempotency key, generated before the POST. A
// value that is not one cannot be trusted to dedupe anything.
func TestIngestHookDelivery_RefusesADeliveryIDThatIsNotACanonicalUUID(t *testing.T) {
	f := newFixture(t)
	_, runnerID := f.spawn(t, "claude")

	for _, id := range []string{"", "not-a-uuid", "  " + uuid.NewString(), strings.ToUpper(uuid.NewString())} {
		err := f.usecase.IngestHookDelivery(
			f.ctx, "", id, runnerID, "claude", "turn_stop", mustJSON(t, map[string]any{}))
		assert.Error(t, err, "delivery id %q", id)
	}
}

// A hook from a runner nobody has heard of is dropped, not failed: by the time it
// arrives the CLI has already acted, and failing it would break its turn.
func TestIngestHookDelivery_AnUnknownRunnerIsDropped(t *testing.T) {
	f := newFixture(t)

	err := f.usecase.IngestHookDelivery(f.ctx, "", uuid.NewString(), uuid.NewString(),
		"claude", "turn_stop", mustJSON(t, map[string]any{"last_assistant_message": "x"}))

	assert.NoError(t, err)
}

// The route scope already authorised the workspace, so a hook that fires BEFORE
// its runner row exists can still be journalled durably.
func TestIngestHookDelivery_UsesTheRouteScopeWhenTheRunnerIsNotYetPersisted(t *testing.T) {
	f := newFixture(t)

	err := f.usecase.IngestHookDelivery(f.ctx, "ws1", uuid.NewString(), uuid.NewString(),
		"claude", "session_start", mustJSON(t, map[string]any{"session_id": "s1"}))

	assert.NoError(t, err)
}
