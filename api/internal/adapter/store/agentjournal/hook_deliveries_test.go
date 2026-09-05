package agentjournal_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter/store/agentjournal"
)

func TestHookDeliveries_Begin_MkdirAllFailsWhenDirIsBlockedByAFile(t *testing.T) {
	// Dir() derives a path nested under a per-runner directory; if some path
	// component on the way there is already a plain file (not a directory),
	// MkdirAll cannot create the rest of the tree and Begin must surface that
	// instead of panicking or silently treating the delivery as new.
	root := t.TempDir()
	blocker := filepath.Join(root, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("not a directory"), 0o600))

	deliveries := agentjournal.NewHookDeliveries()
	dir := filepath.Join(blocker, "runner-1")

	_, err := deliveries.Begin(dir, "delivery-1", "hash", time.Now())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "mkdir")
}

func TestHookDeliveries_Begin_SurfacesACorruptExistingRecord(t *testing.T) {
	// A delivery id's on-disk record is expected to be a JSON file. If that path
	// is occupied by a directory instead (e.g. filesystem corruption, or a bug
	// upstream that created the wrong kind of entry), the read must fail loudly
	// rather than Begin silently treating it as a brand-new delivery.
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "delivery-1.json"), 0o700))

	deliveries := agentjournal.NewHookDeliveries()

	_, err := deliveries.Begin(dir, "delivery-1", "hash", time.Now())

	require.Error(t, err)
}

func TestHookDeliveries_Complete_SurfacesACorruptExistingRecord(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "delivery-1.json"), 0o700))

	deliveries := agentjournal.NewHookDeliveries()

	err := deliveries.Complete(dir, "delivery-1", "hash", time.Now())

	require.Error(t, err)
}

func TestHookDeliveries_Complete_ErrorsWhenNoPendingRecordExists(t *testing.T) {
	// Complete is only ever reached after a prior Begin recorded the delivery as
	// pending. If that record has vanished (e.g. the journal directory was wiped
	// between the two calls), Complete must not fabricate a completion — it
	// reports the inconsistency instead.
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(dir, 0o700))

	deliveries := agentjournal.NewHookDeliveries()

	err := deliveries.Complete(dir, "never-begun", "hash", time.Now())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "pending record disappeared")
}

func TestHookDeliveries_Complete_PropagatesADurabilityFault(t *testing.T) {
	// Begin with the real (working) journal so a genuine pending record lands on
	// disk, then attempt Complete through a second instance whose syncDir is
	// wired to fail — simulating the fsync-after-rename step losing power. The
	// caller must see that error, not a false completion.
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(dir, 0o700))

	writer := agentjournal.NewHookDeliveries()
	_, err := writer.Begin(dir, "delivery-1", "hash", time.Now())
	require.NoError(t, err)

	faultySync := errors.New("simulated fsync failure")
	reader := agentjournal.NewHookDeliveries(agentjournal.WithDirSync(func(string) error {
		return faultySync
	}))

	err = reader.Complete(dir, "delivery-1", "hash", time.Now())

	require.Error(t, err)
	assert.ErrorIs(t, err, faultySync)
}

// TestHookDeliveryHash_IsStableAndDistinguishesEveryField proves the hash binds
// ALL four fields, not just their concatenation: the null-byte separators exist
// precisely so a boundary shift between two fields (e.g. "ab"+"c" vs "a"+"bc")
// cannot collide the way naive string concatenation would.
func TestHookDeliveryHash_IsStableAndDistinguishesEveryField(t *testing.T) {
	base := agentjournal.HookDeliveryHash("runner-1", "claude", "event-a", []byte("payload"))

	assert.Equal(t, base, agentjournal.HookDeliveryHash("runner-1", "claude", "event-a", []byte("payload")),
		"the same inputs must always hash the same")
	assert.NotEqual(t, base, agentjournal.HookDeliveryHash("runner-2", "claude", "event-a", []byte("payload")),
		"a different runner must change the hash")
	assert.NotEqual(t, base, agentjournal.HookDeliveryHash("runner-1", "openai", "event-a", []byte("payload")),
		"a different provider must change the hash")
	assert.NotEqual(t, base, agentjournal.HookDeliveryHash("runner-1", "claude", "event-b", []byte("payload")),
		"a different event name must change the hash")
	assert.NotEqual(t, base, agentjournal.HookDeliveryHash("runner-1", "claude", "event-a", []byte("different")),
		"a different raw payload must change the hash")
	assert.NotEqual(t,
		agentjournal.HookDeliveryHash("ab", "c", "event", nil),
		agentjournal.HookDeliveryHash("a", "bc", "event", nil),
		"a field-boundary shift must not collide with naive concatenation")
}

func TestHookDeliveries_Dir_JoinsChatsDirAndRunnerID(t *testing.T) {
	deliveries := agentjournal.NewHookDeliveries()

	got := deliveries.Dir("/chats/c1", "runner-7")

	assert.Equal(t, filepath.Join("/chats/c1", agentjournal.HookDeliveriesDirName, "runner-7"), got)
}

func TestHookDeliveries_CompletionMarkers_TracksOnlyCompletedDeliveries(t *testing.T) {
	dir := t.TempDir()
	deliveries := agentjournal.NewHookDeliveries()
	_, err := deliveries.Begin(dir, "still-pending", "hash", time.Now())
	require.NoError(t, err)
	_, err = deliveries.Begin(dir, "done", "hash", time.Now())
	require.NoError(t, err)
	require.NoError(t, deliveries.Complete(dir, "done", "hash", time.Now()))

	markers := deliveries.CompletionMarkers()

	assert.Equal(t, []string{"done"}, markers, "a merely-begun delivery must not be reported as completed")
}

// TestHookDeliveries_Begin_AlreadyCompletedInMemorySameHash_ReturnsTrueWithoutTouchingDisk
// proves the in-memory completion map answers a repeat Begin WITHOUT consulting
// disk: removing the journal directory entirely between Complete and the second
// Begin still yields (true, nil) — if the code had fallen through to disk
// instead, it would have found nothing and treated the delivery as brand new.
func TestHookDeliveries_Begin_AlreadyCompletedInMemorySameHash_ReturnsTrueWithoutTouchingDisk(t *testing.T) {
	dir := t.TempDir()
	deliveries := agentjournal.NewHookDeliveries()
	_, err := deliveries.Begin(dir, "delivery-1", "hash", time.Now())
	require.NoError(t, err)
	require.NoError(t, deliveries.Complete(dir, "delivery-1", "hash", time.Now()))

	require.NoError(t, os.RemoveAll(dir))

	done, err := deliveries.Begin(dir, "delivery-1", "hash", time.Now())

	require.NoError(t, err)
	assert.True(t, done, "an in-memory completion marker must answer without touching disk")
}

func TestHookDeliveries_Begin_AlreadyCompletedInMemoryHashMismatch_ReturnsPayloadMismatch(t *testing.T) {
	dir := t.TempDir()
	deliveries := agentjournal.NewHookDeliveries()
	_, err := deliveries.Begin(dir, "delivery-1", "hash-a", time.Now())
	require.NoError(t, err)
	require.NoError(t, deliveries.Complete(dir, "delivery-1", "hash-a", time.Now()))

	_, err = deliveries.Begin(dir, "delivery-1", "hash-b", time.Now())

	assert.ErrorIs(t, err, agentjournal.ErrHookPayloadMismatch)
}

// TestHookDeliveries_Begin_FoundOnDiskStillPending_ReturnsFalse exercises the
// on-disk lookup branch with a FRESH instance (empty in-memory map), whose
// answer can only come from reading the still-pending record back off disk.
func TestHookDeliveries_Begin_FoundOnDiskStillPending_ReturnsFalse(t *testing.T) {
	dir := t.TempDir()
	writer := agentjournal.NewHookDeliveries()
	_, err := writer.Begin(dir, "delivery-1", "hash", time.Now())
	require.NoError(t, err)

	reader := agentjournal.NewHookDeliveries()
	done, err := reader.Begin(dir, "delivery-1", "hash", time.Now())

	require.NoError(t, err)
	assert.False(t, done, "a still-pending on-disk record must not be reported as already done")
}

func TestHookDeliveries_Begin_FoundOnDiskCompleted_ReturnsTrue(t *testing.T) {
	dir := t.TempDir()
	writer := agentjournal.NewHookDeliveries()
	_, err := writer.Begin(dir, "delivery-1", "hash", time.Now())
	require.NoError(t, err)
	require.NoError(t, writer.Complete(dir, "delivery-1", "hash", time.Now()))

	reader := agentjournal.NewHookDeliveries()
	done, err := reader.Begin(dir, "delivery-1", "hash", time.Now())

	require.NoError(t, err)
	assert.True(t, done, "a completed on-disk record must be found by a fresh instance")
}

func TestHookDeliveries_Begin_FoundOnDiskHashMismatch_ReturnsPayloadMismatch(t *testing.T) {
	dir := t.TempDir()
	writer := agentjournal.NewHookDeliveries()
	_, err := writer.Begin(dir, "delivery-1", "hash-a", time.Now())
	require.NoError(t, err)

	reader := agentjournal.NewHookDeliveries()
	_, err = reader.Begin(dir, "delivery-1", "hash-b", time.Now())

	assert.ErrorIs(t, err, agentjournal.ErrHookPayloadMismatch)
}

// TestHookDeliveries_Complete_EvictsTheOldestMarkerPastTheCap drives the FIFO
// eviction end to end through the public API: past HookDeliveryCompletedMax
// completions, CompletionMarkers stays capped and the oldest entry is the one
// dropped, not an arbitrary one.
func TestHookDeliveries_Complete_EvictsTheOldestMarkerPastTheCap(t *testing.T) {
	dir := t.TempDir()
	deliveries := agentjournal.NewHookDeliveries()

	for i := range agentjournal.HookDeliveryCompletedMax + 1 {
		id := fmt.Sprintf("delivery-%d", i)
		_, err := deliveries.Begin(dir, id, "hash", time.Now())
		require.NoError(t, err)
		require.NoError(t, deliveries.Complete(dir, id, "hash", time.Now()))
	}

	markers := deliveries.CompletionMarkers()

	assert.Len(t, markers, agentjournal.HookDeliveryCompletedMax, "the in-memory FIFO must stay capped")
	assert.NotContains(t, markers, "delivery-0", "the oldest completion must be evicted first")
	assert.Contains(t, markers, fmt.Sprintf("delivery-%d", agentjournal.HookDeliveryCompletedMax),
		"the newest completion must survive")
}
