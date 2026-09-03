package agentjournal_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter/store/agentjournal"
)

// asFile creates dir's parent normally but plants a plain FILE where a
// directory is expected, so os.ReadDir/os.Stat on it fails with something
// other than os.ErrNotExist — the real-error branches distinct from the
// already-well-tested "journal not created yet" paths.
func asFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))
	return path
}

func TestJournal_LookupDowngradesADispatchingRecordAndPersistsIt(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.Begin(dir, "req-1", "hash", "claude", "out", "new", jnow)
	require.NoError(t, err)

	record, found, err := j.Lookup(dir, "req-1", "hash")

	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, agentjournal.PromptStateUncertain, record.State)

	onDisk, ok, err := agentjournal.ReadPromptRequest(dir, "req-1")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, agentjournal.PromptStateUncertain, onDisk.State, "the downgrade must be durable, not just returned")
}

func TestJournal_LookupPropagatesADurabilityFaultDuringDowngrade(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "chat-1", "prompt-requests")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	writer := agentjournal.NewPromptRequests()
	_, _, err := writer.Begin(dir, "req-1", "hash", "claude", "out", "new", jnow)
	require.NoError(t, err)

	sentinel := errors.New("simulated fsync failure")
	faulty := agentjournal.NewPromptRequests(agentjournal.WithDirSync(func(string) error { return sentinel }))

	_, _, err = faulty.Lookup(dir, "req-1", "hash")

	require.ErrorIs(t, err, sentinel)
}

func TestJournal_Begin_SurfacesACorruptExistingRecord(t *testing.T) {
	j, dir := journal(t)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "req-1.json"), 0o700))

	_, _, err := j.Begin(dir, "req-1", "hash", "claude", "out", "new", jnow)

	require.Error(t, err)
}

func TestJournal_Begin_PropagatesADurabilityFaultOnTheNewRecord(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "chat-1", "prompt-requests")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	sentinel := errors.New("simulated fsync failure")
	faulty := agentjournal.NewPromptRequests(agentjournal.WithDirSync(func(string) error { return sentinel }))

	_, _, err := faulty.Begin(dir, "req-1", "hash", "claude", "out", "new", jnow)

	require.ErrorIs(t, err, sentinel)
	// The rename itself is not rolled back on a sync failure (writeRecord
	// commits the rename before syncing the directory) — the caller sees the
	// error and may retry, but the content is genuinely on disk already.
	record, found, readErr := agentjournal.ReadPromptRequest(dir, "req-1")
	require.NoError(t, readErr)
	require.True(t, found)
	assert.Equal(t, agentjournal.PromptStateDispatching, record.State)
}

func TestJournal_Begin_SurfacesAnUnstattableJournalDir(t *testing.T) {
	// ensureDir's os.Stat fails with something other than ErrNotExist when a
	// path component ABOVE the target is a plain file rather than a directory
	// (ENOTDIR) — a real corruption/misconfiguration case distinct from "not
	// created yet".
	blocker := asFile(t)
	j := agentjournal.NewPromptRequests()
	dir := filepath.Join(blocker, "prompt-requests")

	_, _, err := j.Begin(dir, "req-1", "hash", "claude", "out", "new", jnow)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "stat dir")
}

func TestJournal_Begin_SurfacesAMkdirAllFailureOnAReadOnlyParent(t *testing.T) {
	parent := t.TempDir()
	readOnlyParent := filepath.Join(parent, "readonly")
	require.NoError(t, os.Mkdir(readOnlyParent, 0o500)) // r-x: Stat can traverse it, MkdirAll cannot write into it

	j := agentjournal.NewPromptRequests()
	dir := filepath.Join(readOnlyParent, "prompt-requests")

	_, _, err := j.Begin(dir, "req-1", "hash", "claude", "out", "new", jnow)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "mkdir")
}

func TestJournal_ConfirmAccepted_NonExistentDirectoryIsNotAnError(t *testing.T) {
	j := agentjournal.NewPromptRequests()
	dir := filepath.Join(t.TempDir(), "never-created")

	err := j.ConfirmAccepted(dir, "runner-1", "claude", "hash", jnow)

	require.NoError(t, err)
}

func TestJournal_ConfirmAccepted_SurfacesARealReadFailure(t *testing.T) {
	j := agentjournal.NewPromptRequests()

	err := j.ConfirmAccepted(asFile(t), "runner-1", "claude", "hash", jnow)

	require.Error(t, err)
}

func TestJournal_ConfirmAccepted_SkipsNonMatchingRecordsAndAcceptsTheRightOne(t *testing.T) {
	// Only one delivery is ever ACTIVE per chat directory (Begin enforces that),
	// so two simultaneously-live records for different providers is a state no
	// caller reaches through the interface — planted directly here, the same
	// way export_test.go's own helpers exist to reach states no caller can.
	j, dir := journal(t)
	require.NoError(t, agentjournal.WriteRecord(dir, "other-provider.json", agentjournal.PromptRequest{
		RequestID:  "other-provider",
		TextHash:   "hash",
		State:      agentjournal.PromptStateSpawned,
		ProviderID: "openai",
		RunnerID:   "new-1",
		CreatedAt:  jnow,
		UpdatedAt:  jnow,
	}, ".prompt-request-*", func(string) error { return nil }))

	require.NoError(t, agentjournal.WriteRecord(dir, "req-1.json", agentjournal.PromptRequest{
		RequestID:        "req-1",
		TextHash:         "hash",
		State:            agentjournal.PromptStateDispatching,
		ProviderID:       "claude",
		OutgoingRunnerID: "out",
		RunnerID:         "new-2",
		CreatedAt:        jnow,
		UpdatedAt:        jnow,
	}, ".prompt-request-*", func(string) error { return nil }))

	err := j.ConfirmAccepted(dir, "new-2", "claude", "hash", jnow)
	require.NoError(t, err)

	accepted, found, err := agentjournal.ReadPromptRequest(dir, "req-1")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, agentjournal.PromptStateAccepted, accepted.State)

	untouched, found, err := agentjournal.ReadPromptRequest(dir, "other-provider")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, agentjournal.PromptStateSpawned, untouched.State, "a non-matching record must never be mutated")
}

func TestJournal_ConfirmAccepted_NoMatchIsNotAnError(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.Begin(dir, "req-1", "hash-a", "claude", "out", "new", jnow)
	require.NoError(t, err)

	// Different text hash: nothing in the journal is acknowledgeable.
	err = j.ConfirmAccepted(dir, "new", "claude", "hash-b", jnow)

	require.NoError(t, err)
	record, _, err := agentjournal.ReadPromptRequest(dir, "req-1")
	require.NoError(t, err)
	assert.Equal(t, agentjournal.PromptStateDispatching, record.State)
}

func TestJournal_ConfirmAccepted_IgnoresARecordInATerminalNonAcknowledgeableState(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.Begin(dir, "req-1", "hash", "claude", "out", "new", jnow)
	require.NoError(t, err)
	require.NoError(t, j.MarkFailedDispatch(dir, "req-1", jnow))

	// A FAILED record must never be resurrected into Accepted by an echo.
	err = j.ConfirmAccepted(dir, "new", "claude", "hash", jnow)
	require.NoError(t, err)

	record, _, err := agentjournal.ReadPromptRequest(dir, "req-1")
	require.NoError(t, err)
	assert.Equal(t, agentjournal.PromptStateFailed, record.State)
}

func TestJournal_ConfirmAccepted_NeverLetsTheOutgoingRunnerAcknowledgeItsOwnHandoff(t *testing.T) {
	// A record's RunnerID is empty until MarkSpawned assigns the REPLACEMENT
	// runner's identity. Before that, the OLD (outgoing) runner's hook firing
	// for the same text/provider must not be mistaken for the new runner's
	// acceptance — that would falsely accept a prompt the new process hasn't
	// even spawned yet.
	j, dir := journal(t)
	_, _, err := j.Begin(dir, "req-1", "hash", "claude", "outgoing-runner", "new-runner", jnow)
	require.NoError(t, err)

	err = j.ConfirmAccepted(dir, "outgoing-runner", "claude", "hash", jnow)
	require.NoError(t, err)

	record, _, err := agentjournal.ReadPromptRequest(dir, "req-1")
	require.NoError(t, err)
	assert.Equal(t, agentjournal.PromptStateDispatching, record.State, "the outgoing runner's echo must not accept the handoff")
}

func TestJournal_MarkSpawned_SurfacesACorruptExistingRecord(t *testing.T) {
	j, dir := journal(t)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "req-1.json"), 0o700))

	_, err := j.MarkSpawned(dir, "req-1", "hash", "runner-1", "term-1", jnow)

	require.Error(t, err)
}

func TestJournal_MarkSpawned_PropagatesADurabilityFault(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "chat-1", "prompt-requests")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	writer := agentjournal.NewPromptRequests()
	_, _, err := writer.Begin(dir, "req-1", "hash", "claude", "out", "new", jnow)
	require.NoError(t, err)

	sentinel := errors.New("simulated fsync failure")
	faulty := agentjournal.NewPromptRequests(agentjournal.WithDirSync(func(string) error { return sentinel }))

	_, err = faulty.MarkSpawned(dir, "req-1", "hash", "runner-1", "term-1", jnow)

	require.ErrorIs(t, err, sentinel)
}

func TestJournal_MarkFailedDispatch_UnknownIDIsNotAnError(t *testing.T) {
	j, dir := journal(t)

	err := j.MarkFailedDispatch(dir, "never-begun", jnow)

	require.NoError(t, err)
}

func TestJournal_MarkFailedDispatch_LeavesASpawnedRecordAlone(t *testing.T) {
	// Only a proven PRE-spawn failure may retry through the same request id
	// (reusablePromptRequest's PromptStateFailed case). Once a record has moved
	// past dispatching, MarkFailedDispatch must be a no-op — mislabeling a
	// spawned delivery as "failed" would let a caller wrongly re-dispatch it.
	j, dir := journal(t)
	_, _, err := j.Begin(dir, "req-1", "hash", "claude", "out", "new", jnow)
	require.NoError(t, err)
	_, err = j.MarkSpawned(dir, "req-1", "hash", "new", "term-1", jnow)
	require.NoError(t, err)

	err = j.MarkFailedDispatch(dir, "req-1", jnow)
	require.NoError(t, err)

	record, _, err := agentjournal.ReadPromptRequest(dir, "req-1")
	require.NoError(t, err)
	assert.Equal(t, agentjournal.PromptStateSpawned, record.State)
}

func TestJournal_MarkAccepted_UnknownIDIsAnError(t *testing.T) {
	j, dir := journal(t)

	_, err := j.MarkAccepted(dir, "never-begun", jnow)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestJournal_Settle_UnknownIDIsNotAnError(t *testing.T) {
	j, dir := journal(t)

	retired, err := j.Settle(dir, "never-begun", jnow)

	require.NoError(t, err)
	assert.False(t, retired)
}

func TestJournal_Settle_PropagatesADurabilityFault(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "chat-1", "prompt-requests")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	writer := agentjournal.NewPromptRequests()
	_, _, err := writer.Begin(dir, "req-1", "hash", "claude", "out", "new", jnow)
	require.NoError(t, err)
	_, err = writer.MarkSpawned(dir, "req-1", "hash", "new", "term-1", jnow)
	require.NoError(t, err)

	sentinel := errors.New("simulated fsync failure")
	faulty := agentjournal.NewPromptRequests(agentjournal.WithDirSync(func(string) error { return sentinel }))

	_, err = faulty.Settle(dir, "req-1", jnow)

	require.ErrorIs(t, err, sentinel)
}

func TestJournal_ActiveForRunner_SurfacesARealReadFailure(t *testing.T) {
	j := agentjournal.NewPromptRequests()

	_, found, err := j.ActiveForRunner(asFile(t), "runner-1", "claude")

	require.Error(t, err)
	assert.False(t, found)
}

func TestJournal_ActiveForRunner_SkipsARecordForADifferentProvider(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.Begin(dir, "other-provider", "hash", "openai", "out", "runner-1", jnow)
	require.NoError(t, err)
	_, err = j.MarkSpawned(dir, "other-provider", "hash", "runner-1", "term", jnow)
	require.NoError(t, err)

	_, found, err := j.ActiveForRunner(dir, "runner-1", "claude")

	require.NoError(t, err)
	assert.False(t, found, "a same-runner record for a DIFFERENT provider must not match")
}

func TestJournal_HasPendingDelivery_SurfacesARealReadFailureDuringRecovery(t *testing.T) {
	j := agentjournal.NewPromptRequests()

	_, err := j.HasPendingDelivery(asFile(t))

	require.Error(t, err)
}

func TestJournal_ActiveDelivery_SurfacesARealReadFailure(t *testing.T) {
	j := agentjournal.NewPromptRequests()

	_, found, err := j.ActiveDelivery(asFile(t))

	require.Error(t, err)
	assert.False(t, found)
}

func TestJournal_RecoverOrphanedDispatches_PropagatesADurabilityFault(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "chat-1", "prompt-requests")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	writer := agentjournal.NewPromptRequests()
	_, _, err := writer.Begin(dir, "req-1", "hash", "claude", "out", "new", jnow)
	require.NoError(t, err)

	sentinel := errors.New("simulated fsync failure")
	faulty := agentjournal.NewPromptRequests(agentjournal.WithDirSync(func(string) error { return sentinel }))

	err = faulty.RecoverOrphanedDispatches(dir, jnow)

	require.ErrorIs(t, err, sentinel)
}

func TestJournal_IgnoresJunkEntriesAlongsideRealRecords(t *testing.T) {
	// The journal directory is meant to hold only "<id>.json" record files, but
	// readPromptRequests walks os.ReadDir's raw entries: a stray subdirectory or
	// a non-json leftover (e.g. an orphaned temp file from an interrupted write)
	// must be skipped rather than fed to the JSON decoder.
	j, dir := journal(t)
	_, _, err := j.Begin(dir, "req-1", "hash", "claude", "out", "new", jnow)
	require.NoError(t, err)

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "a-subdirectory"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "leftover.tmp"), []byte("not json"), 0o600))

	record, found, err := j.ActiveForRunner(dir, "new", "claude")

	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "req-1", record.RequestID)
}

func TestPrunePromptRequests_IgnoresAnUnreadableDirectory(t *testing.T) {
	agentjournal.PrunePromptRequests(filepath.Join(t.TempDir(), "does-not-exist"), jnow)
	// No panic, no error return to check (fire-and-forget maintenance called
	// from inside MarkSpawned's own lock) — that it degrades quietly IS the
	// contract this exercises.
}

func TestPrunePromptRequests_RemovesOnlyTerminalRecordsPastTheirAge(t *testing.T) {
	j, dir := journal(t)
	old := jnow.Add(-2 * 31 * 24 * time.Hour) // well past the 30-day retention window

	// A terminal (accepted) record old enough to prune...
	_, _, err := j.Begin(dir, "old-accepted", "hash", "claude", "out", "r1", old)
	require.NoError(t, err)
	require.NoError(t, j.ConfirmAccepted(dir, "r1", "claude", "hash", old))

	// ...alongside a SPAWNED record of the same age, which must survive pruning
	// regardless of age: an in-flight delivery is never pruned.
	_, _, err = j.Begin(dir, "old-spawned", "hash", "claude", "out", "r2", old)
	require.NoError(t, err)
	_, err = j.MarkSpawned(dir, "old-spawned", "hash", "r2", "term", old)
	require.NoError(t, err)

	agentjournal.PrunePromptRequests(dir, jnow)

	_, found, err := agentjournal.ReadPromptRequest(dir, "old-accepted")
	require.NoError(t, err)
	assert.False(t, found, "a terminal record past the retention window must be pruned")

	_, found, err = agentjournal.ReadPromptRequest(dir, "old-spawned")
	require.NoError(t, err)
	assert.True(t, found, "an in-flight (spawned) record must never be pruned regardless of age")
}

// TestJournal_Begin_FirstEverDirectorySyncsItsParent proves ensureDir's
// missing-directory tail: when the journal directory does not exist yet,
// MkdirAll creates it and the PARENT must then be fsynced so the new directory
// entry itself survives a crash — distinct from every other Begin test here,
// which pre-creates the directory via journal(t) and so never reaches this
// branch at all.
func TestJournal_Begin_FirstEverDirectorySyncsItsParent(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "prompt-requests")

	var synced []string
	requests := agentjournal.NewPromptRequests(agentjournal.WithDirSync(func(d string) error {
		synced = append(synced, d)
		return nil
	}))

	_, _, err := requests.Begin(dir, "req-1", "hash", "claude", "out", "new", jnow)
	require.NoError(t, err)

	assert.Contains(t, synced, parent,
		"creating the journal directory for the first time must fsync its PARENT, not just the directory itself")
}

func TestJournal_MarkSpawned_ErrorsWhenRequestNeverBegan(t *testing.T) {
	j, dir := journal(t)

	_, err := j.MarkSpawned(dir, "never-begun", "hash", "runner-1", "term-1", jnow)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "dispatch record disappeared")
}

func TestJournal_MarkSpawned_ErrorsWhenTextHashDiffersFromTheDispatch(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.Begin(dir, "req-1", "hash-a", "claude", "out", "new", jnow)
	require.NoError(t, err)

	_, err = j.MarkSpawned(dir, "req-1", "hash-b", "runner-1", "term-1", jnow)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "dispatch record disappeared")
}

// TestJournal_ConfirmAccepted_RejectsTheOutgoingRunnerBeforeAnyReplacementIsSpawned
// is distinct from TestJournal_ConfirmAccepted_NeverLetsTheOutgoingRunnerAcknowledgeItsOwnHandoff
// above: that test's record was already spawned to a DIFFERENT runner, so
// acknowledgeable's rejection falls through to its final comparison. Here the
// record is still dispatching — RunnerID is genuinely empty, not yet
// assigned — which is the OTHER branch of the same guard: the outgoing
// runner's own hook firing before MarkSpawned ever ran must not be mistaken
// for the still-unspawned replacement's acceptance.
func TestJournal_ConfirmAccepted_RejectsTheOutgoingRunnerBeforeAnyReplacementIsSpawned(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.Begin(dir, "req-1", "hash", "claude", "outgoing-runner", "", jnow)
	require.NoError(t, err)

	err = j.ConfirmAccepted(dir, "outgoing-runner", "claude", "hash", jnow)
	require.NoError(t, err)

	record, found, err := agentjournal.ReadPromptRequest(dir, "req-1")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, agentjournal.PromptStateDispatching, record.State,
		"the outgoing runner must not acknowledge a request before any replacement has even spawned")
}

func TestJournal_MarkUncertain_UnknownIDIsNotAnError(t *testing.T) {
	j, dir := journal(t)

	err := j.MarkUncertain(dir, "never-begun", jnow)

	assert.NoError(t, err, "marking an unknown request uncertain must be a silent no-op")
}

func TestJournal_MarkUncertain_LeavesAnAlreadyAcceptedRecordAlone(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.Begin(dir, "req-1", "hash", "claude", "out", "new", jnow)
	require.NoError(t, err)
	_, err = j.MarkAccepted(dir, "req-1", jnow)
	require.NoError(t, err)

	require.NoError(t, j.MarkUncertain(dir, "req-1", jnow))

	record, found, err := agentjournal.ReadPromptRequest(dir, "req-1")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, agentjournal.PromptStateAccepted, record.State,
		"MarkUncertain must never downgrade an outcome the provider already settled")
}

func TestJournal_MarkAccepted_SurfacesACorruptRecord(t *testing.T) {
	j, dir := journal(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "req-1.json"), []byte("{not json"), 0o600))

	_, err := j.MarkAccepted(dir, "req-1", jnow)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode")
}

func TestJournal_ActiveForRunner_NonExistentDirectoryIsNotAnError(t *testing.T) {
	j := agentjournal.NewPromptRequests()
	dir := filepath.Join(t.TempDir(), "never-created")

	got, found, err := j.ActiveForRunner(dir, "runner-1", "claude")

	require.NoError(t, err)
	assert.False(t, found)
	assert.Zero(t, got)
}

func TestJournal_ActiveForRunner_SkipsATerminalRecord(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.Begin(dir, "req-1", "hash", "claude", "out", "runner-1", jnow)
	require.NoError(t, err)
	_, err = j.MarkAccepted(dir, "req-1", jnow)
	require.NoError(t, err)

	_, found, err := j.ActiveForRunner(dir, "runner-1", "claude")

	require.NoError(t, err)
	assert.False(t, found, "an accepted (settled) record must never be reported as the runner's active delivery")
}

func TestJournal_ActiveDelivery_NonExistentDirectoryIsNotAnError(t *testing.T) {
	j := agentjournal.NewPromptRequests()
	dir := filepath.Join(t.TempDir(), "never-created")

	got, found, err := j.ActiveDelivery(dir)

	require.NoError(t, err)
	assert.False(t, found)
	assert.Zero(t, got)
}

func TestJournal_ActiveDelivery_ReturnsTheInFlightRecord(t *testing.T) {
	j, dir := journal(t)
	_, _, err := j.Begin(dir, "req-1", "hash", "claude", "out", "runner-1", jnow)
	require.NoError(t, err)

	got, found, err := j.ActiveDelivery(dir)

	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "req-1", got.RequestID)
}
