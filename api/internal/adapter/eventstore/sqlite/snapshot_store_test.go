package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestSnapshotStore(t *testing.T) models.SnapshotStore {
	t.Helper()
	s, err := NewSnapshotStore(":memory:")
	require.NoError(t, err)
	return s
}

func rowCount(t *testing.T, s models.SnapshotStore, aggregateID string) int64 {
	t.Helper()
	impl, ok := s.(*snapshotStore)
	require.True(t, ok, "expected concrete *snapshotStore")
	var n int64
	require.NoError(t, impl.db.
		Model(&snapshotEntry{}).
		Where("aggregate_id = ?", aggregateID).
		Count(&n).Error)
	return n
}

func TestNewSnapshotStore_InvalidPath_ReturnsError(t *testing.T) {
	_, err := NewSnapshotStore("/nonexistent-dir-crowbar/snap.sqlite")
	assert.Error(t, err)
}

// TestNewSnapshotStore_ReadonlyDB_JournalModeError mirrors the event store's
// PRAGMA journal_mode=WAL failure branch: a valid file made read-only fails the
// WAL header write while gorm.Open still succeeds.
func TestNewSnapshotStore_ReadonlyDB_JournalModeError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; permission denial has no effect")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "ro.sqlite")

	s, err := NewSnapshotStore(path)
	require.NoError(t, err)
	type closer interface{ Close() error }
	require.NoError(t, s.(closer).Close())

	require.NoError(t, os.Chmod(path, 0o444))
	require.NoError(t, os.Chmod(dir, 0o555))
	t.Cleanup(func() {
		_ = os.Chmod(dir, 0o755)
		_ = os.Chmod(path, 0o644)
	})

	_, err = NewSnapshotStore(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "journal_mode")
}

func TestSnapshotStore_Put_ThenGet(t *testing.T) {
	s := newTestSnapshotStore(t)
	ctx := context.Background()
	require.NoError(t, s.Put(ctx, "agg-1", 1, []byte("v1")))

	data, found, err := s.Get(ctx, "agg-1")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, []byte("v1"), data)
}

func TestSnapshotStore_Get_Missing_NotAnError(t *testing.T) {
	s := newTestSnapshotStore(t)
	data, found, err := s.Get(context.Background(), "never-snapshotted")
	require.NoError(t, err)
	assert.False(t, found)
	assert.Nil(t, data)
}

func TestSnapshotStore_Put_Twice_Upserts_SingleRow(t *testing.T) {
	s := newTestSnapshotStore(t)
	ctx := context.Background()
	require.NoError(t, s.Put(ctx, "agg-1", 1, []byte("v1")))
	require.NoError(t, s.Put(ctx, "agg-1", 2, []byte("v2")))
	require.NoError(t, s.Put(ctx, "agg-1", 3, []byte("v3")))

	data, found, err := s.Get(ctx, "agg-1")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, []byte("v3"), data)

	assert.Equal(t, int64(1), rowCount(t, s, "agg-1"),
		"upsert must keep exactly one row per aggregate")
}

// TestSnapshotStore_Put_OlderVersion_DoesNotOverwrite proves the monotonicity
// guard (ON CONFLICT ... WHERE excluded.version > snapshots.version). Removing
// the guard's WHERE clause makes this test fail.
func TestSnapshotStore_Put_OlderVersion_DoesNotOverwrite(t *testing.T) {
	s := newTestSnapshotStore(t)
	ctx := context.Background()
	require.NoError(t, s.Put(ctx, "agg-1", 5, []byte("newer")))
	require.NoError(t, s.Put(ctx, "agg-1", 2, []byte("older")))

	data, found, err := s.Get(ctx, "agg-1")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, []byte("newer"), data,
		"an older-version Put must not clobber a newer snapshot")

	assert.Equal(t, int64(1), rowCount(t, s, "agg-1"))
}

func TestSnapshotStore_Put_EqualVersion_DoesNotOverwrite(t *testing.T) {
	s := newTestSnapshotStore(t)
	ctx := context.Background()
	require.NoError(t, s.Put(ctx, "agg-1", 3, []byte("first")))
	require.NoError(t, s.Put(ctx, "agg-1", 3, []byte("second")))

	data, _, err := s.Get(ctx, "agg-1")
	require.NoError(t, err)
	assert.Equal(t, []byte("first"), data,
		"guard is strict >, so an equal-version Put is a no-op")
}

func TestSnapshotStore_Delete_RemovesSnapshot(t *testing.T) {
	s := newTestSnapshotStore(t)
	ctx := context.Background()
	require.NoError(t, s.Put(ctx, "agg-1", 1, []byte("v1")))
	require.NoError(t, s.Delete(ctx, "agg-1"))

	_, found, err := s.Get(ctx, "agg-1")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestSnapshotStore_Delete_Idempotent(t *testing.T) {
	s := newTestSnapshotStore(t)
	assert.NoError(t, s.Delete(context.Background(), "never-existed"))
}

func TestSnapshotStore_Put_ContextCancelled(t *testing.T) {
	s := newTestSnapshotStore(t)
	err := s.Put(cancelledCtx(), "agg-1", 1, []byte("v1"))
	assert.Error(t, err)
}

func TestSnapshotStore_Get_ContextCancelled(t *testing.T) {
	s := newTestSnapshotStore(t)
	_, _, err := s.Get(cancelledCtx(), "agg-1")
	assert.Error(t, err)
}

func TestSnapshotStore_Delete_ContextCancelled(t *testing.T) {
	s := newTestSnapshotStore(t)
	err := s.Delete(cancelledCtx(), "agg-1")
	assert.Error(t, err)
}

func TestSnapshotStore_Close_Success(t *testing.T) {
	s, err := NewSnapshotStore(":memory:")
	require.NoError(t, err)
	type closer interface{ Close() error }
	cl, ok := s.(closer)
	require.True(t, ok, "expected snapshotStore to implement io.Closer")
	assert.NoError(t, cl.Close())
}
