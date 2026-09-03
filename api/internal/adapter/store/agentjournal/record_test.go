package agentjournal_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter/store/agentjournal"
)

func noopSyncDir(string) error { return nil }

func TestWriteRecord_SurfacesAnEncodeFailure(t *testing.T) {
	// json.Marshal rejects a channel field; the atomic-write sequence must never
	// reach disk (no temp file, no rename) for a record it can't even encode.
	dir := t.TempDir()

	unencodable := struct {
		C chan int
	}{C: make(chan int)}

	err := agentjournal.WriteRecord(dir, "record.json", unencodable, "tmp-*", noopSyncDir)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "encode")

	entries, readErr := os.ReadDir(dir)
	require.NoError(t, readErr)
	assert.Empty(t, entries, "an encode failure must leave no temp file behind")
}

func TestWriteRecord_SurfacesATempFileCreationFailure(t *testing.T) {
	// A directory that does not exist can never host a temp file; os.CreateTemp
	// fails immediately, before anything is written.
	missingDir := filepath.Join(t.TempDir(), "does-not-exist")

	err := agentjournal.WriteRecord(missingDir, "record.json", map[string]string{"k": "v"}, "tmp-*", noopSyncDir)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "create temp")
}

func TestWriteRecord_SurfacesARenameFailure(t *testing.T) {
	// os.Rename onto a path already occupied by a non-empty directory fails
	// (EISDIR/ENOTEMPTY) — a real, deterministic way to exercise the commit
	// step's failure branch without touching filesystem permissions.
	dir := t.TempDir()
	occupied := filepath.Join(dir, "record.json")
	require.NoError(t, os.MkdirAll(occupied, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(occupied, "keep-me-non-empty"), []byte("x"), 0o600))

	err := agentjournal.WriteRecord(dir, "record.json", map[string]string{"k": "v"}, "tmp-*", noopSyncDir)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "commit")
}

func TestWriteRecord_PropagatesADirSyncFailure(t *testing.T) {
	// The final step fsyncs the parent directory so the rename itself survives a
	// crash. A failure there must reach the caller — the record's bytes may be on
	// disk, but its durability is exactly what this step promises.
	dir := t.TempDir()
	sentinel := errors.New("simulated dir fsync failure")

	err := agentjournal.WriteRecord(dir, "record.json", map[string]string{"k": "v"}, "tmp-*", func(string) error {
		return sentinel
	})

	require.ErrorIs(t, err, sentinel)
	// The record itself is committed even though the durability sync failed —
	// WriteRecord does not roll back a successful rename.
	assert.FileExists(t, filepath.Join(dir, "record.json"))
}

func TestReadRecord_TreatsAMissingFileAsNotFoundNotAnError(t *testing.T) {
	dir := t.TempDir()
	var into map[string]string

	found, err := agentjournal.ReadRecord(filepath.Join(dir, "missing.json"), &into)

	require.NoError(t, err)
	assert.False(t, found)
}

func TestReadRecord_SurfacesARealReadFailure(t *testing.T) {
	// A path that exists but is a directory, not a file, fails os.ReadFile with
	// something other than os.ErrNotExist — that distinction is exactly what
	// readRecord's two early-return branches exist to tell apart.
	dir := t.TempDir()
	dirAsRecord := filepath.Join(dir, "record.json")
	require.NoError(t, os.MkdirAll(dirAsRecord, 0o700))
	var into map[string]string

	found, err := agentjournal.ReadRecord(dirAsRecord, &into)

	require.Error(t, err)
	assert.False(t, found)
	assert.NotErrorIs(t, err, os.ErrNotExist)
}

func TestReadRecord_SurfacesADecodeFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "record.json")
	require.NoError(t, os.WriteFile(path, []byte("not valid json"), 0o600))
	var into map[string]string

	found, err := agentjournal.ReadRecord(path, &into)

	require.Error(t, err)
	assert.False(t, found)
	assert.Contains(t, err.Error(), "decode")
}

func TestSyncJournalDir_SurfacesAnOpenFailureOnAVanishedDirectory(t *testing.T) {
	// The parent directory writeRecord just committed a rename into can, in
	// principle, be removed by the time the durability fsync runs (a concurrent
	// workspace teardown) — os.Open on it must fail loudly, not panic or be
	// swallowed.
	vanished := filepath.Join(t.TempDir(), "does-not-exist")

	err := agentjournal.SyncJournalDir(vanished)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "open dir for sync")
}

func TestStageRecord_SurfacesAChmodFailureOnAClosedFile(t *testing.T) {
	// Chmod (and every other op stageRecord performs) on an already-closed file
	// descriptor fails with a wrapped os.ErrClosed — a state writeRecord's own
	// os.CreateTemp can never itself produce, but a real failure mode stageRecord
	// must still report correctly.
	tmp, err := os.CreateTemp(t.TempDir(), "tmp-*")
	require.NoError(t, err)
	require.NoError(t, tmp.Close())

	err = agentjournal.StageRecord(tmp, []byte("data"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "chmod temp")
}

func TestStageRecord_SurfacesAWriteFailureOnAReadOnlyHandle(t *testing.T) {
	// A file descriptor opened O_RDONLY still allows Chmod (a metadata change,
	// not an I/O operation on the open handle) but rejects Write outright — this
	// isolates the write failure branch from the chmod one above.
	dir := t.TempDir()
	path := filepath.Join(dir, "readonly")
	require.NoError(t, os.WriteFile(path, nil, 0o600))
	ro, err := os.Open(path) //nolint:gosec // test-owned temp file
	require.NoError(t, err)
	defer ro.Close()

	err = agentjournal.StageRecord(ro, []byte("data"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "write temp")
}
