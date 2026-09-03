package agentjournal

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeDirEntry is a minimal fs.DirEntry so appendCompletedHookDelivery can be
// exercised directly with a name that deliberately isn't a real file on disk.
type fakeDirEntry struct {
	name  string
	isDir bool
}

func (f fakeDirEntry) Name() string               { return f.name }
func (f fakeDirEntry) IsDir() bool                { return f.isDir }
func (f fakeDirEntry) Type() fs.FileMode          { return 0 }
func (f fakeDirEntry) Info() (fs.FileInfo, error) { return nil, nil }

func TestAppendCompletedHookDelivery_SkipsEntriesWithoutJSONSuffix(t *testing.T) {
	// The journal directory holds only ".json" records, but pruning walks
	// os.ReadDir's raw entries; a stray non-json file (e.g. a leftover temp file
	// the atomic-write sequence failed to clean up) must be ignored rather than
	// mistaken for a delivery record and fed to readHookDelivery.
	records := appendCompletedHookDelivery(nil, t.TempDir(), "not-a-record.tmp")

	assert.Empty(t, records)
}

func TestReapHookDeliveryRunners_ReapsOldDirectoriesButSkipsFiles(t *testing.T) {
	// A runners root can end up with a stray file alongside the real per-runner
	// directories (e.g. a filesystem artifact). reapHookDeliveryRunner must
	// distinguish the two: only aged DIRECTORIES are removed, a file entry is
	// left alone regardless of how old the reap threshold makes it look.
	root := t.TempDir()

	strayFile := filepath.Join(root, "stray-file")
	require.NoError(t, os.WriteFile(strayFile, []byte("not a runner dir"), 0o600))

	oldRunnerDir := filepath.Join(root, "ancient-runner")
	require.NoError(t, os.MkdirAll(oldRunnerDir, 0o700))
	ancient := time.Now().Add(-2 * HookDeliveryJournalMaxAge)
	require.NoError(t, os.Chtimes(oldRunnerDir, ancient, ancient))

	reapHookDeliveryRunners(root, time.Now())

	assert.FileExists(t, strayFile, "a plain file must never be reaped, no matter its age")
	assert.NoDirExists(t, oldRunnerDir, "a runner directory idle past the max age must be reaped")
}

func TestReapHookDeliveryRunners_DegradesQuietlyWhenRootIsUnreadable(t *testing.T) {
	// Complete's post-commit maintenance calls this on every Nth completion with
	// nowhere to route an error (it is fire-and-forget bookkeeping, not a request
	// path). If the root has vanished between the mkdir and the sweep, it must
	// return without touching the filesystem rather than propagating or panicking.
	missingRoot := filepath.Join(t.TempDir(), "does-not-exist")

	assert.NotPanics(t, func() {
		reapHookDeliveryRunners(missingRoot, time.Now())
	})
	assert.NoDirExists(t, missingRoot, "a missing root must not be created as a side effect of the reap")
}

func TestReapHookDeliveryRunners_LeavesARecentDirectoryAlone(t *testing.T) {
	// The sibling test above only ever proves an ANCIENT directory is reaped; a
	// directory well inside the max age must survive the exact same sweep.
	root := t.TempDir()
	recentRunnerDir := filepath.Join(root, "recent-runner")
	require.NoError(t, os.MkdirAll(recentRunnerDir, 0o700))

	reapHookDeliveryRunners(root, time.Now())

	assert.DirExists(t, recentRunnerDir, "a runner directory well within the max age must never be reaped")
}

func TestHookDeliveries_Complete_CalledTwiceForTheSameDeliveryIsIdempotent(t *testing.T) {
	// Complete has no way to know whether a caller is retrying after a crash
	// between the durable write and its own return, so a second Complete for an
	// already-marked id must not duplicate the in-memory FIFO entry.
	dir := t.TempDir()
	deliveries := newHookDeliveries()
	_, err := deliveries.Begin(dir, "delivery-1", "hash", time.Now())
	require.NoError(t, err)
	require.NoError(t, deliveries.Complete(dir, "delivery-1", "hash", time.Now()))

	require.NoError(t, deliveries.Complete(dir, "delivery-1", "hash", time.Now()))

	assert.Equal(t, []string{"delivery-1"}, deliveries.CompletionMarkers(),
		"a repeated completion for the same id must not duplicate the marker")
}

// TestPruneHookDeliveries_RemovesOnlyTheExcessPastTheJournalMax drives the real
// retention threshold end to end: past HookDeliveryJournalMax total entries, the
// oldest COMPLETED records are removed down to the cap, while a still-pending
// record — never a removal candidate, regardless of age — is left untouched on
// top of that cap.
func TestPruneHookDeliveries_RemovesOnlyTheExcessPastTheJournalMax(t *testing.T) {
	dir := t.TempDir()
	noopSync := func(string) error { return nil }
	const totalCompleted = HookDeliveryJournalMax + 5

	for i := range totalCompleted {
		require.NoError(t, writeHookDelivery(dir, HookDelivery{
			DeliveryID: fmt.Sprintf("completed-%03d", i),
			Hash:       "hash",
			State:      hookDeliveryStateCompleted,
			CreatedAt:  time.Unix(int64(i), 0),
			UpdatedAt:  time.Unix(int64(i), 0),
		}, noopSync))
	}
	require.NoError(t, writeHookDelivery(dir, HookDelivery{
		DeliveryID: "still-pending",
		Hash:       "hash",
		State:      hookDeliveryStatePending,
		CreatedAt:  time.Unix(0, 0),
		UpdatedAt:  time.Unix(0, 0),
	}, noopSync))

	pruneHookDeliveries(dir)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, HookDeliveryJournalMax,
		"completed records are trimmed down to the cap; the pending record rides on top of it")

	_, foundOldest, err := readHookDelivery(dir, "completed-000")
	require.NoError(t, err)
	assert.False(t, foundOldest, "the oldest completed records must be the ones removed")

	_, foundNewest, err := readHookDelivery(dir, fmt.Sprintf("completed-%03d", totalCompleted-1))
	require.NoError(t, err)
	assert.True(t, foundNewest, "the newest completed records must survive")

	_, foundPending, err := readHookDelivery(dir, "still-pending")
	require.NoError(t, err)
	assert.True(t, foundPending, "a pending record must never be pruned regardless of age or count")
}
