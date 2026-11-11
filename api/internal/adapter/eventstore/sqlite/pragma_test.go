package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An append must not pay an fsync: the event log and snapshot store run
// synchronous=NORMAL (under WAL), which keeps a committed write across an
// application crash. 1 is PRAGMA synchronous's answer for NORMAL.

func TestNewEventStore_IsSynchronousNormal(t *testing.T) {
	s, err := NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	require.NoError(t, err)
	es, ok := s.(*eventStore)
	require.True(t, ok, "NewEventStore returned %T", s)
	var mode int
	require.NoError(t, es.db.QueryRowContext(context.Background(), "PRAGMA synchronous").Scan(&mode))
	assert.Equal(t, 1, mode)
}

func TestNewSnapshotStore_IsSynchronousNormal(t *testing.T) {
	s, err := NewSnapshotStore(filepath.Join(t.TempDir(), "snapshots.db"))
	require.NoError(t, err)
	ss, ok := s.(*snapshotStore)
	require.True(t, ok, "NewSnapshotStore returned %T", s)
	var mode int
	require.NoError(t, ss.db.Raw("PRAGMA synchronous").Scan(&mode).Error)
	assert.Equal(t, 1, mode)
}

// TestNewEventStore_FilesAreOwnerOnly: the event log and its snapshot file are
// created 0600, and SQLite gives their -wal and -shm the same mode.
func TestNewEventStore_FilesAreOwnerOnly(t *testing.T) {
	dir := t.TempDir()
	events := filepath.Join(dir, "events.db")
	snaps := filepath.Join(dir, "snapshots.db")
	_, err := NewEventStore(events)
	require.NoError(t, err)
	_, err = NewSnapshotStore(snaps)
	require.NoError(t, err)
	for _, path := range []string{events, snaps} {
		for _, p := range []string{path, path + "-wal", path + "-shm"} {
			info, err := os.Stat(p)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), p)
		}
	}
}
