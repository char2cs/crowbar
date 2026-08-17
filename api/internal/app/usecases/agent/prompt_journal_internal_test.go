package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

// The prompt journal is the at-most-once record that stands between a user
// pressing send and a destructive process replacement. It is reached directly
// here because it IS a state machine over files: driving it through a spawn would
// test the spawn, and the states that matter most are the ones a crash leaves
// behind, which no successful spawn ever produces.

// The clock is REAL, not a fixed instant: the journal's own recovery treats a
// dispatch older than its grace period as orphaned, so a hard-coded past time
// would silently recover every record before the test could assert on it.
var jnow = time.Now()

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

// The intent must be on disk BEFORE anything destructive happens, so a crash in
// between is recoverable rather than ambiguous.
func TestJournal_BeginIsDurableBeforeItReturns(t *testing.T) {
	j, dir := journal(t)

	_, _, err := j.begin(dir, "req-1", "hash", "claude", "out", "new", jnow)
	require.NoError(t, err)

	found, ok, err := readPromptRecord(dir, "req-1")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, promptStateDispatching, found.State)
}

// The same id with DIFFERENT text is a client bug, not a retry: answering it with
// the first attempt's result would report success for a message never sent.
func TestJournal_RejectsAReusedRequestIDWithDifferentText(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.begin(dir, "req-1", "hash-a", "claude", "out", "new", jnow)
	require.NoError(t, err)

	_, _, err = j.begin(dir, "req-1", "hash-b", "claude", "out", "new2", jnow)

	assert.ErrorIs(t, err, ErrPromptRequestIDConflict)
}

// Only a failure proven to precede the replacement process may retry: any other
// state means a process may exist, and a second one would deliver twice.
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

// A request whose CLI is already up blocks the next one: the chat has one CLI,
// and starting a second delivery would replace a process mid-answer.
func TestJournal_RefusesASecondRequestWhileOneIsSpawned(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.begin(dir, "req-1", "hash-a", "claude", "out", "new", jnow)
	require.NoError(t, err)
	_, err = j.markSpawned(dir, "req-1", "hash-a", spawnResult("runner", "session"), jnow)
	require.NoError(t, err)

	_, _, err = j.begin(dir, "req-2", "hash-b", "claude", "out", "new2", jnow)

	assert.ErrorIs(t, err, ErrPromptBusy)
}

// A record still reading `dispatching` when anything else touches the journal is
// by definition ORPHANED: begin and its outcome are written inside one call under
// the chat gate, so nobody legitimately observes the intermediate state. It is
// recovered to `uncertain` rather than retried, because a replacement process may
// already exist and a blind retry would deliver the prompt twice.
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

// A dispatch left mid-flight by a crash cannot be retried blindly: the process
// may have existed. Recovery moves it to a state that says exactly that.
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

// An unreadable record fails the whole scan rather than being skipped, and that
// is deliberate: a journal it cannot fully read cannot answer "has this prompt
// already been delivered", and guessing there means delivering twice. Records are
// written temp-then-rename, so a partial one is not a state the writer produces.
func TestJournal_RefusesToAnswerFromAJournalItCannotFullyRead(t *testing.T) {
	j, dir := journal(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "corrupt.json"), []byte("{not json"), 0o600))

	_, _, err := j.begin(dir, "req-1", "hash", "claude", "out", "new", jnow)

	assert.Error(t, err)
}

// The journal is unbounded otherwise: one file per prompt, for the life of a chat.
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

func spawnResult(runnerID, sessionID string) dto.PromptSubmissionDTO {
	return dto.PromptSubmissionDTO{RunnerID: runnerID, TerminalSessionID: sessionID}
}

func mustRecord(t *testing.T, dir, id string) promptRequestRecord {
	t.Helper()
	record, ok, err := readPromptRecord(dir, id)
	require.NoError(t, err)
	require.True(t, ok)
	return record
}
