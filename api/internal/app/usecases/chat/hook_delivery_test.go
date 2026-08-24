package chat_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
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
			f.ctx, "", deliveryID, runnerID, "claude", "turn_stop", payload,
		))
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
			f.ctx, "", uuid.NewString(), runnerID, "claude", "turn_stop", payload,
		))
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
			f.ctx, "", id, runnerID, "claude", "turn_stop", mustJSON(t, map[string]any{}),
		)
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

func TestRegression_IngestHookDelivery_TheJournalIsBoundedInMemoryAndOnDisk(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	total := 10 * agentusecase.HookDeliveryPruneEvery
	guarded := total / 2
	ids := make([]string, 0, total)

	for i := range total {
		id := uuid.NewString()
		ids = append(ids, id)
		event, payload := boundedJournalDelivery(t, i, guarded, total-1)
		require.NoError(t, f.usecase.IngestHookDelivery(
			f.ctx, "", id, runnerID, "claude", event, payload,
		))
	}
	f.wait()

	assert.LessOrEqual(t, agentusecase.HookDeliveryMarkerCount(f.usecase.TurnUsecase),
		agentusecase.HookDeliveryCompletedMax, "the in-memory completion map must be capped")
	dir := filepath.Join(f.ws.chatsDir, agentusecase.HookDeliveryDirName, runnerID)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(entries), agentusecase.HookDeliveryJournalMax,
		"the on-disk runner journal must be capped")

	replayed := ids[guarded]
	require.False(t, agentusecase.HookDeliveryMarked(f.usecase.TurnUsecase, replayed),
		"the guarded delivery must have been evicted from memory, or the replay proves nothing")
	require.FileExists(t, filepath.Join(dir, replayed+".json"),
		"the guarded delivery must still be on disk, or there is nothing left to answer the replay")

	before, err := f.activity.Turns(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	require.Len(t, before, 2)
	require.Equal(t, "the guarded reply", before[0].Text)
	_, guardedPayload := boundedJournalDelivery(t, guarded, guarded, total-1)
	require.NoError(t, f.usecase.IngestHookDelivery(
		f.ctx, "", replayed, runnerID, "claude", "turn_stop", guardedPayload,
	))
	f.wait()

	after, err := f.activity.Turns(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, before, after,
		"a delivery evicted from memory is still done on disk: nothing may be appended or resequenced")
}

func boundedJournalDelivery(
	t *testing.T,
	index, guarded, last int,
) (event string, payload []byte) {
	t.Helper()
	if index == guarded {
		return "turn_stop", mustJSON(t, map[string]any{"last_assistant_message": "the guarded reply"})
	}
	if index == last {
		return "turn_stop", mustJSON(t, map[string]any{"last_assistant_message": "the later reply"})
	}
	return "not_a_declared_event", mustJSON(t, map[string]any{"filler": index})
}

func TestRegression_IngestHookDelivery_AFailedCompletionDoesNotRelocateTheTurnOnReplay(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	firstID := uuid.NewString()
	firstPayload := mustJSON(t, map[string]any{"last_assistant_message": "the first reply"})
	var syncs atomic.Int64
	agentusecase.SetHookDeliveryDirSync(f.usecase.TurnUsecase, func(string) error {
		if syncs.Add(1) != 2 {
			return nil
		}
		return errors.New("injected hook delivery dir fsync failure")
	})

	require.NoError(t, f.usecase.IngestHookDelivery(
		f.ctx, "", firstID, runnerID, "claude", "turn_stop", firstPayload,
	))
	f.wait()
	require.Equal(t, int64(2), syncs.Load(), "the fault must have landed on the completion write")
	require.False(t, agentusecase.HookDeliveryMarked(f.usecase.TurnUsecase, firstID),
		"a completion whose durable write failed must not be marked done in memory")

	require.NoError(t, f.usecase.IngestHookDelivery(
		f.ctx, "", uuid.NewString(), runnerID, "claude", "turn_stop",
		mustJSON(t, map[string]any{"last_assistant_message": "the second reply"}),
	))
	f.wait()
	before, err := f.activity.Turns(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	require.Len(t, before, 2)
	require.Equal(t, "the first reply", before[0].Text)

	require.NoError(t, f.usecase.IngestHookDelivery(
		f.ctx, "", firstID, runnerID, "claude", "turn_stop", firstPayload,
	),
		"a delivery whose effects already committed is done, however its marker fared")
	f.wait()

	after, err := f.activity.Turns(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, before, after,
		"replaying the turn would bump its Seq and relocate it to the end of the log")
	replayErr := f.usecase.IngestHookDelivery(
		f.ctx, "", firstID, runnerID, "claude", "turn_stop",
		mustJSON(t, map[string]any{"last_assistant_message": "a different reply"}),
	)
	require.Error(t, replayErr)
	assert.Contains(t, replayErr.Error(), "different payload")
}

func TestRegression_IngestHookDelivery_AnIdleRunnerDirectoryIsReaped(t *testing.T) {
	f := newFixture(t)
	_, runnerID := f.spawn(t, "claude")
	root := filepath.Join(f.ws.chatsDir, agentusecase.HookDeliveryDirName)
	stale := filepath.Join(root, uuid.NewString())
	require.NoError(t, os.MkdirAll(stale, 0o700))
	idle := time.Now().Add(-2 * agentusecase.HookDeliveryJournalMaxAge)
	require.NoError(t, os.Chtimes(stale, idle, idle))

	for i := range agentusecase.HookDeliveryPruneEvery {
		require.NoError(t, f.usecase.IngestHookDelivery(
			f.ctx, "", uuid.NewString(), runnerID, "claude", "not_a_declared_event",
			mustJSON(t, map[string]any{"filler": i}),
		))
	}
	f.wait()

	assert.NoDirExists(t, stale, "a runner directory silent past the max age must be reaped whole")
	assert.DirExists(t, filepath.Join(root, runnerID), "the live runner's directory must survive")
}

func TestRegression_IngestHookDelivery_PruningNeverRemovesAnInFlightRecord(t *testing.T) {
	f := newFixture(t)
	_, runnerID := f.spawn(t, "claude")
	dir := filepath.Join(f.ws.chatsDir, agentusecase.HookDeliveryDirName, runnerID)
	require.NoError(t, os.MkdirAll(dir, 0o700))
	inFlight := make([]string, 0, agentusecase.HookDeliveryJournalMax)
	stale := time.Now().Add(-time.Hour)

	for range agentusecase.HookDeliveryJournalMax - agentusecase.HookDeliveryPruneEvery/2 {
		id := uuid.NewString()
		inFlight = append(inFlight, id)
		require.NoError(t, agentusecase.PlantPendingHookDelivery(dir, id, stale))
	}
	for i := range agentusecase.HookDeliveryPruneEvery {
		require.NoError(t, f.usecase.IngestHookDelivery(
			f.ctx, "", uuid.NewString(), runnerID, "claude", "not_a_declared_event",
			mustJSON(t, map[string]any{"filler": i}),
		))
	}
	f.wait()

	for _, id := range inFlight {
		require.FileExists(t, filepath.Join(dir, id+".json"),
			"an in-flight delivery is the one thing the prune may never take")
	}
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(entries), agentusecase.HookDeliveryJournalMax)
}
