package agentjournal_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter/store/agentjournal"
)

var jnow = time.Now()

func requireSettled(t *testing.T, j agentjournal.PromptRequests, dir, requestID string) {
	t.Helper()
	retired, err := j.Settle(dir, requestID, jnow)
	require.NoError(t, err)
	require.True(t, retired)
}

func journal(t *testing.T) (agentjournal.PromptRequests, string) {
	t.Helper()
	j := agentjournal.NewPromptRequests()
	dir := j.Dir(t.TempDir(), "chat-1")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	return j, dir
}

func TestJournal_BeginRecordsADispatchingIntent(t *testing.T) {
	j, dir := journal(t)

	record, existing, err := j.Begin(dir, "req-1", "hash", "claude", "runner-out", "runner-new", jnow)

	require.NoError(t, err)
	assert.False(t, existing)
	assert.Equal(t, agentjournal.PromptStateDispatching, record.State)
	assert.Equal(t, "runner-new", record.RunnerID)
	assert.Equal(t, "runner-out", record.OutgoingRunnerID)
}

func TestJournal_BeginIsDurableBeforeItReturns(t *testing.T) {
	j, dir := journal(t)

	_, _, err := j.Begin(dir, "req-1", "hash", "claude", "out", "new", jnow)
	require.NoError(t, err)

	found, ok, err := agentjournal.ReadPromptRequest(dir, "req-1")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, agentjournal.PromptStateDispatching, found.State)
}

func TestJournal_RejectsAReusedRequestIDWithDifferentText(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.Begin(dir, "req-1", "hash-a", "claude", "out", "new", jnow)
	require.NoError(t, err)

	_, _, err = j.Begin(dir, "req-1", "hash-b", "claude", "out", "new2", jnow)

	assert.ErrorIs(t, err, agentjournal.ErrPromptRequestIDConflict)
}

func TestJournal_OnlyAFailedRecordFallsThroughToARetry(t *testing.T) {
	testCases := []struct {
		name     string
		state    string
		existing bool
	}{
		{"dispatching", agentjournal.PromptStateDispatching, true},
		{"spawned", agentjournal.PromptStateSpawned, true},
		{"accepted", agentjournal.PromptStateAccepted, true},
		{"uncertain", agentjournal.PromptStateUncertain, true},
		{"failed", agentjournal.PromptStateFailed, false},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			j, dir := journal(t)
			require.NoError(t, agentjournal.WritePromptRequest(dir, agentjournal.PromptRequest{
				RequestID: "req-1", TextHash: "hash", State: tc.state,
				ProviderID: "claude", CreatedAt: jnow, UpdatedAt: jnow,
			}))

			_, existing, err := j.Begin(dir, "req-1", "hash", "claude", "out", "new", jnow)

			require.NoError(t, err)
			assert.Equal(t, tc.existing, existing)
		})
	}
}

func TestJournal_RejectsAnUnknownStoredState(t *testing.T) {
	j, dir := journal(t)
	require.NoError(t, agentjournal.WritePromptRequest(dir, agentjournal.PromptRequest{
		RequestID: "req-1", TextHash: "hash", State: "teleported", CreatedAt: jnow,
	}))

	_, _, err := j.Begin(dir, "req-1", "hash", "claude", "out", "new", jnow)

	assert.Error(t, err)
}

func TestJournal_RefusesASecondRequestWhileOneIsSpawned(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.Begin(dir, "req-1", "hash-a", "claude", "out", "new", jnow)
	require.NoError(t, err)
	_, err = j.MarkSpawned(dir, "req-1", "hash-a", "runner", "session", jnow)
	require.NoError(t, err)

	_, _, err = j.Begin(dir, "req-2", "hash-b", "claude", "out", "new2", jnow)

	assert.ErrorIs(t, err, agentjournal.ErrPromptBusy)
}

func TestJournal_ADispatchingRecordIsOrphanedByDefinition(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.Begin(dir, "req-1", "hash-a", "claude", "out", "new", jnow)
	require.NoError(t, err)
	require.Equal(t, agentjournal.PromptStateDispatching, mustRecord(t, dir, "req-1").State)

	pending, err := j.HasPendingDelivery(dir)

	require.NoError(t, err)
	assert.False(t, pending, "nothing is in flight; an outcome is merely unknown")
	assert.Equal(t, agentjournal.PromptStateUncertain, mustRecord(t, dir, "req-1").State)
}

func TestJournal_LookupReturnsTheOriginalResultForARetry(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.Begin(dir, "req-1", "hash", "claude", "out", "new", jnow)
	require.NoError(t, err)
	_, err = j.MarkSpawned(dir, "req-1", "hash", "runner-1", "session-1", jnow)
	require.NoError(t, err)

	found, ok, err := j.Lookup(dir, "req-1", "hash")

	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, agentjournal.PromptStateSpawned, found.State)
	assert.Equal(t, "runner-1", found.RunnerID)
	assert.Equal(t, "session-1", found.TerminalSessionID)
}

func TestJournal_LookupOfAnUnknownRequestIsAbsentNotAnError(t *testing.T) {
	j, dir := journal(t)

	_, ok, err := j.Lookup(dir, "never-seen", "hash")

	require.NoError(t, err)
	assert.False(t, ok)
}

func TestJournal_LookupRejectsAReusedIDWithDifferentText(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.Begin(dir, "req-1", "hash-a", "claude", "out", "new", jnow)
	require.NoError(t, err)

	_, _, err = j.Lookup(dir, "req-1", "hash-b")

	assert.ErrorIs(t, err, agentjournal.ErrPromptRequestIDConflict)
}

func TestJournal_StateTransitionsAreRecorded(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.Begin(dir, "req-1", "hash", "claude", "out", "new", jnow)
	require.NoError(t, err)
	_, err = j.MarkSpawned(dir, "req-1", "hash", "runner-1", "session-1", jnow)
	require.NoError(t, err)

	require.NoError(t, j.ConfirmAccepted(dir, "runner-1", "claude", "hash", jnow))
	found, ok, err := agentjournal.ReadPromptRequest(dir, "req-1")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, agentjournal.PromptStateAccepted, found.State)
}

func TestJournal_MarkFailedDispatchAllowsASameIDRetry(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.Begin(dir, "req-1", "hash", "claude", "out", "new", jnow)
	require.NoError(t, err)

	require.NoError(t, j.MarkFailedDispatch(dir, "req-1", jnow))

	_, existing, err := j.Begin(dir, "req-1", "hash", "claude", "out", "new2", jnow)
	require.NoError(t, err)
	assert.False(t, existing, "a proven pre-spawn failure is safe to retry")
}

func TestJournal_MarkUncertainLeavesTheOutcomeUnknown(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.Begin(dir, "req-1", "hash", "claude", "out", "new", jnow)
	require.NoError(t, err)

	require.NoError(t, j.MarkUncertain(dir, "req-1", jnow))

	found, ok, err := agentjournal.ReadPromptRequest(dir, "req-1")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, agentjournal.PromptStateUncertain, found.State)
}

func TestJournal_MarkAcceptedByRequestIsIdempotent(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.Begin(dir, "req-1", "hash", "claude", "out", "new", jnow)
	require.NoError(t, err)

	_, err = j.MarkAccepted(dir, "req-1", jnow)
	require.NoError(t, err)
	assert.Equal(t, agentjournal.PromptStateAccepted, mustRecord(t, dir, "req-1").State)

	_, err = j.MarkAccepted(dir, "req-1", jnow)
	require.NoError(t, err)
	assert.Equal(t, agentjournal.PromptStateAccepted, mustRecord(t, dir, "req-1").State)
}

func TestJournal_MarkAcceptedByRequestOnAnUnknownRequestIsReported(t *testing.T) {
	j, dir := journal(t)

	_, err := j.MarkAccepted(dir, "never-seen", jnow)

	assert.Error(t, err, "accepting a request nobody journalled is a caller bug, not a state")
}

func TestJournal_ReportsWhetherADeliveryIsPending(t *testing.T) {
	j, dir := journal(t)

	pending, err := j.HasPendingDelivery(dir)
	require.NoError(t, err)
	assert.False(t, pending, "an empty journal has nothing in flight")

	_, _, err = j.Begin(dir, "req-1", "hash", "claude", "out", "new", jnow)
	require.NoError(t, err)
	_, err = j.MarkSpawned(dir, "req-1", "hash", "runner", "session", jnow)
	require.NoError(t, err)

	pending, err = j.HasPendingDelivery(dir)
	require.NoError(t, err)
	assert.True(t, pending, "a spawned delivery is in flight until its hook confirms it")
}

func TestJournal_ActiveForRunnerFindsTheRequestARunnerIsDelivering(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.Begin(dir, "req-1", "hash", "claude", "out", "runner-new", jnow)
	require.NoError(t, err)

	found, ok, err := j.ActiveForRunner(dir, "runner-new", "claude")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "req-1", found.RequestID)

	_, ok, err = j.ActiveForRunner(dir, "someone-else", "claude")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestJournal_RecoversOrphanedDispatchesToUncertain(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.Begin(dir, "req-1", "hash", "claude", "out", "new", jnow)
	require.NoError(t, err)

	require.NoError(t, j.RecoverOrphanedDispatches(dir, jnow.Add(time.Hour)))

	assert.Equal(t, agentjournal.PromptStateUncertain, mustRecord(t, dir, "req-1").State)
}

func TestJournal_ReadsTolerateAMissingDirectory(t *testing.T) {
	j, _ := journal(t)
	missing := filepath.Join(t.TempDir(), "never-created")

	_, ok, err := j.Lookup(missing, "req-1", "hash")
	require.NoError(t, err)
	assert.False(t, ok)

	pending, err := j.HasPendingDelivery(missing)
	require.NoError(t, err)
	assert.False(t, pending)
}

func TestJournal_RefusesToAnswerFromAJournalItCannotFullyRead(t *testing.T) {
	j, dir := journal(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "corrupt.json"), []byte("{not json"), 0o600))

	_, _, err := j.Begin(dir, "req-1", "hash", "claude", "out", "new", jnow)

	assert.Error(t, err)
}

func TestJournal_PrunesSettledRecordsAndKeepsUnsettledOnes(t *testing.T) {
	_, dir := journal(t)
	old := jnow.Add(-90 * 24 * time.Hour)
	require.NoError(t, agentjournal.WritePromptRequest(dir, agentjournal.PromptRequest{
		RequestID: "settled", State: agentjournal.PromptStateAccepted, CreatedAt: old, UpdatedAt: old,
	}))
	require.NoError(t, agentjournal.WritePromptRequest(dir, agentjournal.PromptRequest{
		RequestID: "unsettled", State: agentjournal.PromptStateUncertain, CreatedAt: old, UpdatedAt: old,
	}))

	agentjournal.PrunePromptRequests(dir, jnow)

	_, settledStillThere, err := agentjournal.ReadPromptRequest(dir, "settled")
	require.NoError(t, err)
	assert.False(t, settledStillThere, "a long-settled record has nothing left to answer")

	_, unsettledStillThere, err := agentjournal.ReadPromptRequest(dir, "unsettled")
	require.NoError(t, err)
	assert.True(t, unsettledStillThere, "an unresolved outcome must never be forgotten")
}

func TestPromptTextHash_IsStableAndDistinguishing(t *testing.T) {
	assert.Equal(t, agentjournal.PromptTextHash("same"), agentjournal.PromptTextHash("same"))
	assert.NotEqual(t, agentjournal.PromptTextHash("a"), agentjournal.PromptTextHash("b"))
	assert.NotEmpty(t, agentjournal.PromptTextHash(""))
}

func mustRecord(t *testing.T, dir, id string) agentjournal.PromptRequest {
	t.Helper()
	record, ok, err := agentjournal.ReadPromptRequest(dir, id)
	require.NoError(t, err)
	require.True(t, ok)
	return record
}

func TestRegression_SettledDeliveryStopsBlockingTheChat(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.Begin(dir, "req-1", "hash", "claude", "out", "new", jnow)
	require.NoError(t, err)
	_, err = j.MarkSpawned(dir, "req-1", "hash", "new", "pty", jnow)
	require.NoError(t, err)

	pending, err := j.HasPendingDelivery(dir)
	require.NoError(t, err)
	require.True(t, pending, "a spawned delivery blocks, which is the point of it")
	_, _, err = j.Begin(dir, "req-2", "other", "claude", "out", "new2", jnow)
	require.ErrorIs(t, err, agentjournal.ErrPromptBusy)

	requireSettled(t, j, dir, "req-1")

	pending, err = j.HasPendingDelivery(dir)
	require.NoError(t, err)
	assert.False(t, pending)
	_, _, err = j.Begin(dir, "req-2", "other", "claude", "out", "new2", jnow)
	assert.NoError(t, err, "the next prompt must be accepted once nothing is owed")
}

func TestJournal_ASettledRequestIsNeverDeliveredTwice(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.Begin(dir, "req-1", "hash", "claude", "out", "new", jnow)
	require.NoError(t, err)
	_, err = j.MarkSpawned(dir, "req-1", "hash", "new", "pty", jnow)
	require.NoError(t, err)
	requireSettled(t, j, dir, "req-1")

	record, existing, err := j.Begin(dir, "req-1", "hash", "claude", "out", "new", jnow)

	require.NoError(t, err)
	assert.True(t, existing)
	assert.Equal(t, agentjournal.PromptStateSettled, record.State)
}

func TestJournal_AnAcknowledgementUpgradesASettledRecord(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.Begin(dir, "req-1", "hash", "claude", "out", "new", jnow)
	require.NoError(t, err)
	_, err = j.MarkSpawned(dir, "req-1", "hash", "new", "pty", jnow)
	require.NoError(t, err)
	requireSettled(t, j, dir, "req-1")

	require.NoError(t, j.ConfirmAccepted(dir, "new", "claude", "hash", jnow))

	found, ok, err := agentjournal.ReadPromptRequest(dir, "req-1")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, agentjournal.PromptStateAccepted, found.State)
}

func TestJournal_SettleOnlyRetiresASpawnedRecord(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(j agentjournal.PromptRequests, dir string)
		want string
	}{
		{
			name: "dispatching has not reached a process",
			set:  func(agentjournal.PromptRequests, string) {},
			want: agentjournal.PromptStateDispatching,
		},
		{
			name: "accepted is a provider's own word",
			set: func(j agentjournal.PromptRequests, dir string) {
				_, err := j.MarkAccepted(dir, "req-1", jnow)
				require.NoError(t, err)
			},
			want: agentjournal.PromptStateAccepted,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			j, dir := journal(t)
			_, _, err := j.Begin(dir, "req-1", "hash", "claude", "out", "new", jnow)
			require.NoError(t, err)
			tc.set(j, dir)

			retired, err := j.Settle(dir, "req-1", jnow)

			require.NoError(t, err)
			assert.False(t, retired, "only a spawned record is retired")
			found, ok, err := agentjournal.ReadPromptRequest(dir, "req-1")
			require.NoError(t, err)
			require.True(t, ok)
			assert.Equal(t, tc.want, found.State)
		})
	}
}
