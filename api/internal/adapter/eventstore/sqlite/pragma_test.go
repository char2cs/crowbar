package sqlite

import (
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
	var mode int
	require.NoError(t, s.(*eventStore).db.Raw("PRAGMA synchronous").Scan(&mode).Error)
	assert.Equal(t, 1, mode)
}

func TestNewSnapshotStore_IsSynchronousNormal(t *testing.T) {
	s, err := NewSnapshotStore(filepath.Join(t.TempDir(), "snapshots.db"))
	require.NoError(t, err)
	var mode int
	require.NoError(t, s.(*snapshotStore).db.Raw("PRAGMA synchronous").Scan(&mode).Error)
	assert.Equal(t, 1, mode)
}
