package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

var jnow = time.Now()

func requireSettled(t *testing.T, j *promptJournal, dir, requestID string) {
	t.Helper()
	retired, err := j.settle(dir, requestID, jnow)
	require.NoError(t, err)
	require.True(t, retired)
}

func journal(t *testing.T) (*promptJournal, string) {
	t.Helper()
	dir := promptJournalDir(t.TempDir())
	require.NoError(t, os.MkdirAll(dir, 0o700))
	return newPromptJournal(), dir
}

func TestJournal_BeginRecordsADispatchingIntent(t *testing.T) {
	j, dir := journal(t)

	record, existing, err := j.begin(dir, "req-1", "hash", "claude", "runner-out", "runner-new", jnow)

	require.NoError(t, err)
	assert.False(t, existing)
	assert.Equal(t, promptStateDispatching, record.State)
	assert.Equal(t, "runner-new", record.RunnerID)
	assert.Equal(t, "runner-out", record.OutgoingRunnerID)
}

func TestJournal_BeginIsDurableBeforeItReturns(t *testing.T) {
	j, dir := journal(t)

	_, _, err := j.begin(dir, "req-1", "hash", "claude", "out", "new", jnow)
	require.NoError(t, err)

	found, ok, err := readPromptRecord(dir, "req-1")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, promptStateDispatching, found.State)
}

func TestJournal_RejectsAReusedRequestIDWithDifferentText(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.begin(dir, "req-1", "hash-a", "claude", "out", "new", jnow)
	require.NoError(t, err)

	_, _, err = j.begin(dir, "req-1", "hash-b", "claude", "out", "new2", jnow)

	assert.ErrorIs(t, err, ErrPromptRequestIDConflict)
}

func TestJournal_OnlyAFailedRecordFallsThroughToARetry(t *testing.T) {
	testCases := []struct {
		name     string
		state    string
		existing bool
	}{
		{"dispatching", promptStateDispatching, true},
		{"spawned", promptStateSpawned, true},
		{"accepted", promptStateAccepted, true},
		{"uncertain", promptStateUncertain, true},
		{"failed", promptStateFailed, false},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			j, dir := journal(t)
			require.NoError(t, j.write(dir, promptRequestRecord{
				RequestID: "req-1", TextHash: "hash", State: tc.state,
				ProviderID: "claude", CreatedAt: jnow, UpdatedAt: jnow,
			}))

			_, existing, err := j.begin(dir, "req-1", "hash", "claude", "out", "new", jnow)

			require.NoError(t, err)
			assert.Equal(t, tc.existing, existing)
		})
	}
}

func TestJournal_RejectsAnUnknownStoredState(t *testing.T) {
	j, dir := journal(t)
	require.NoError(t, j.write(dir, promptRequestRecord{
		RequestID: "req-1", TextHash: "hash", State: "teleported", CreatedAt: jnow,
	}))

	_, _, err := j.begin(dir, "req-1", "hash", "claude", "out", "new", jnow)

	assert.Error(t, err)
}

func TestJournal_RefusesASecondRequestWhileOneIsSpawned(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.begin(dir, "req-1", "hash-a", "claude", "out", "new", jnow)
	require.NoError(t, err)
	_, err = j.markSpawned(dir, "req-1", "hash-a", spawnResult("runner", "session"), jnow)
	require.NoError(t, err)

	_, _, err = j.begin(dir, "req-2", "hash-b", "claude", "out", "new2", jnow)

	assert.ErrorIs(t, err, ErrPromptBusy)
}

func TestJournal_ADispatchingRecordIsOrphanedByDefinition(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.begin(dir, "req-1", "hash-a", "claude", "out", "new", jnow)
	require.NoError(t, err)
	require.Equal(t, promptStateDispatching, mustRecord(t, dir, "req-1").State)

	pending, err := j.hasPendingDelivery(dir)

	require.NoError(t, err)
	assert.False(t, pending, "nothing is in flight; an outcome is merely unknown")
	assert.Equal(t, promptStateUncertain, mustRecord(t, dir, "req-1").State)
}

func TestJournal_LookupReturnsTheOriginalResultForARetry(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.begin(dir, "req-1", "hash", "claude", "out", "new", jnow)
	require.NoError(t, err)
	_, err = j.markSpawned(dir, "req-1", "hash", spawnResult("runner-1", "session-1"), jnow)
	require.NoError(t, err)

	found, ok, err := j.lookup(dir, "req-1", "hash")

	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, promptStateSpawned, found.State)
	assert.Equal(t, "runner-1", found.RunnerID)
	assert.Equal(t, "session-1", found.TerminalSessionID)
	assert.Equal(t, "runner-1", found.result().RunnerID)
}

func TestJournal_LookupOfAnUnknownRequestIsAbsentNotAnError(t *testing.T) {
	j, dir := journal(t)

	_, ok, err := j.lookup(dir, "never-seen", "hash")

	require.NoError(t, err)
	assert.False(t, ok)
}

func TestJournal_LookupRejectsAReusedIDWithDifferentText(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.begin(dir, "req-1", "hash-a", "claude", "out", "new", jnow)
	require.NoError(t, err)

	_, _, err = j.lookup(dir, "req-1", "hash-b")

	assert.ErrorIs(t, err, ErrPromptRequestIDConflict)
}

func TestJournal_StateTransitionsAreRecorded(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.begin(dir, "req-1", "hash", "claude", "out", "new", jnow)
	require.NoError(t, err)
	_, err = j.markSpawned(dir, "req-1", "hash", spawnResult("runner-1", "session-1"), jnow)
	require.NoError(t, err)

	require.NoError(t, j.confirmAccepted(dir, "runner-1", "claude", "hash", jnow))
	found, ok, err := readPromptRecord(dir, "req-1")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, promptStateAccepted, found.State)
}

func TestJournal_MarkFailedDispatchAllowsASameIDRetry(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.begin(dir, "req-1", "hash", "claude", "out", "new", jnow)
	require.NoError(t, err)

	require.NoError(t, j.markFailedDispatch(dir, "req-1", jnow))

	_, existing, err := j.begin(dir, "req-1", "hash", "claude", "out", "new2", jnow)
	require.NoError(t, err)
	assert.False(t, existing, "a proven pre-spawn failure is safe to retry")
}

func TestJournal_MarkUncertainLeavesTheOutcomeUnknown(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.begin(dir, "req-1", "hash", "claude", "out", "new", jnow)
	require.NoError(t, err)

	require.NoError(t, j.markUncertain(dir, "req-1", jnow))

	found, ok, err := readPromptRecord(dir, "req-1")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, promptStateUncertain, found.State)
}

func TestJournal_MarkAcceptedByRequestIsIdempotent(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.begin(dir, "req-1", "hash", "claude", "out", "new", jnow)
	require.NoError(t, err)

	_, err = j.markAcceptedByRequest(dir, "req-1", jnow)
	require.NoError(t, err)
	assert.Equal(t, promptStateAccepted, mustRecord(t, dir, "req-1").State)

	_, err = j.markAcceptedByRequest(dir, "req-1", jnow)
	require.NoError(t, err)
	assert.Equal(t, promptStateAccepted, mustRecord(t, dir, "req-1").State)
}

func TestJournal_MarkAcceptedByRequestOnAnUnknownRequestIsReported(t *testing.T) {
	j, dir := journal(t)

	_, err := j.markAcceptedByRequest(dir, "never-seen", jnow)

	assert.Error(t, err, "accepting a request nobody journalled is a caller bug, not a state")
}

func TestJournal_ReportsWhetherADeliveryIsPending(t *testing.T) {
	j, dir := journal(t)

	pending, err := j.hasPendingDelivery(dir)
	require.NoError(t, err)
	assert.False(t, pending, "an empty journal has nothing in flight")

	_, _, err = j.begin(dir, "req-1", "hash", "claude", "out", "new", jnow)
	require.NoError(t, err)
	_, err = j.markSpawned(dir, "req-1", "hash", spawnResult("runner", "session"), jnow)
	require.NoError(t, err)

	pending, err = j.hasPendingDelivery(dir)
	require.NoError(t, err)
	assert.True(t, pending, "a spawned delivery is in flight until its hook confirms it")
}

func TestJournal_ActiveForRunnerFindsTheRequestARunnerIsDelivering(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.begin(dir, "req-1", "hash", "claude", "out", "runner-new", jnow)
	require.NoError(t, err)

	found, ok, err := j.activeForRunner(dir, "runner-new", "claude")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "req-1", found.RequestID)

	_, ok, err = j.activeForRunner(dir, "someone-else", "claude")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestJournal_RecoversOrphanedDispatchesToUncertain(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.begin(dir, "req-1", "hash", "claude", "out", "new", jnow)
	require.NoError(t, err)

	require.NoError(t, j.recoverOrphanedDispatches(dir, jnow.Add(time.Hour)))

	assert.Equal(t, promptStateUncertain, mustRecord(t, dir, "req-1").State)
}

func TestJournal_ReadsTolerateAMissingDirectory(t *testing.T) {
	j, _ := journal(t)
	missing := filepath.Join(t.TempDir(), "never-created")

	_, ok, err := j.lookup(missing, "req-1", "hash")
	require.NoError(t, err)
	assert.False(t, ok)

	pending, err := j.hasPendingDelivery(missing)
	require.NoError(t, err)
	assert.False(t, pending)
}

func TestJournal_RefusesToAnswerFromAJournalItCannotFullyRead(t *testing.T) {
	j, dir := journal(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "corrupt.json"), []byte("{not json"), 0o600))

	_, _, err := j.begin(dir, "req-1", "hash", "claude", "out", "new", jnow)

	assert.Error(t, err)
}

func TestJournal_PrunesSettledRecordsAndKeepsUnsettledOnes(t *testing.T) {
	j, dir := journal(t)
	old := jnow.Add(-90 * 24 * time.Hour)
	require.NoError(t, j.write(dir, promptRequestRecord{
		RequestID: "settled", State: promptStateAccepted, CreatedAt: old, UpdatedAt: old,
	}))
	require.NoError(t, j.write(dir, promptRequestRecord{
		RequestID: "unsettled", State: promptStateUncertain, CreatedAt: old, UpdatedAt: old,
	}))

	prunePromptRecords(dir, jnow)

	_, settledStillThere, err := readPromptRecord(dir, "settled")
	require.NoError(t, err)
	assert.False(t, settledStillThere, "a long-settled record has nothing left to answer")

	_, unsettledStillThere, err := readPromptRecord(dir, "unsettled")
	require.NoError(t, err)
	assert.True(t, unsettledStillThere, "an unresolved outcome must never be forgotten")
}

func TestPromptTextHash_IsStableAndDistinguishing(t *testing.T) {
	assert.Equal(t, promptTextHash("same"), promptTextHash("same"))
	assert.NotEqual(t, promptTextHash("a"), promptTextHash("b"))
	assert.NotEmpty(t, promptTextHash(""))
}

func spawnResult(runnerID, sessionID string) domain.AgentPromptSubmission {
	return domain.AgentPromptSubmission{RunnerID: runnerID, TerminalSessionID: sessionID}
}

func mustRecord(t *testing.T, dir, id string) promptRequestRecord {
	t.Helper()
	record, ok, err := readPromptRecord(dir, id)
	require.NoError(t, err)
	require.True(t, ok)
	return record
}

func TestRegression_SettledDeliveryStopsBlockingTheChat(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.begin(dir, "req-1", "hash", "claude", "out", "new", jnow)
	require.NoError(t, err)
	_, err = j.markSpawned(dir, "req-1", "hash",
		domain.AgentPromptSubmission{RunnerID: "new", TerminalSessionID: "pty"}, jnow)
	require.NoError(t, err)

	pending, err := j.hasPendingDelivery(dir)
	require.NoError(t, err)
	require.True(t, pending, "a spawned delivery blocks, which is the point of it")
	_, _, err = j.begin(dir, "req-2", "other", "claude", "out", "new2", jnow)
	require.ErrorIs(t, err, ErrPromptBusy)

	requireSettled(t, j, dir, "req-1")

	pending, err = j.hasPendingDelivery(dir)
	require.NoError(t, err)
	assert.False(t, pending)
	_, _, err = j.begin(dir, "req-2", "other", "claude", "out", "new2", jnow)
	assert.NoError(t, err, "the next prompt must be accepted once nothing is owed")
}

func TestJournal_ASettledRequestIsNeverDeliveredTwice(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.begin(dir, "req-1", "hash", "claude", "out", "new", jnow)
	require.NoError(t, err)
	_, err = j.markSpawned(dir, "req-1", "hash",
		domain.AgentPromptSubmission{RunnerID: "new", TerminalSessionID: "pty"}, jnow)
	require.NoError(t, err)
	requireSettled(t, j, dir, "req-1")

	record, existing, err := j.begin(dir, "req-1", "hash", "claude", "out", "new", jnow)

	require.NoError(t, err)
	assert.True(t, existing)
	assert.Equal(t, promptStateSettled, record.State)
}

func TestJournal_AnAcknowledgementUpgradesASettledRecord(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.begin(dir, "req-1", "hash", "claude", "out", "new", jnow)
	require.NoError(t, err)
	_, err = j.markSpawned(dir, "req-1", "hash",
		domain.AgentPromptSubmission{RunnerID: "new", TerminalSessionID: "pty"}, jnow)
	require.NoError(t, err)
	requireSettled(t, j, dir, "req-1")

	require.NoError(t, j.confirmAccepted(dir, "new", "claude", "hash", jnow))

	found, ok, err := readPromptRecord(dir, "req-1")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, promptStateAccepted, found.State)
}

func TestJournal_SettleOnlyRetiresASpawnedRecord(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(j *promptJournal, dir string)
		want string
	}{
		{
			name: "dispatching has not reached a process",
			set:  func(*promptJournal, string) {},
			want: promptStateDispatching,
		},
		{
			name: "accepted is a provider's own word",
			set: func(j *promptJournal, dir string) {
				_, err := j.markAcceptedByRequest(dir, "req-1", jnow)
				require.NoError(t, err)
			},
			want: promptStateAccepted,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			j, dir := journal(t)
			_, _, err := j.begin(dir, "req-1", "hash", "claude", "out", "new", jnow)
			require.NoError(t, err)
			tc.set(j, dir)

			retired, err := j.settle(dir, "req-1", jnow)

			require.NoError(t, err)
			assert.False(t, retired, "only a spawned record is retired")
			found, ok, err := readPromptRecord(dir, "req-1")
			require.NoError(t, err)
			require.True(t, ok)
			assert.Equal(t, tc.want, found.State)
		})
	}
}
