package sqlite

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestEventStore(t *testing.T) models.Store {
	t.Helper()
	s, err := NewEventStore(":memory:")
	require.NoError(t, err)
	return s
}

func cancelledCtx() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestNewEventStore_InvalidPath_ReturnsError(t *testing.T) {
	_, err := NewEventStore("/nonexistent-dir-crowbar/db.sqlite")
	assert.Error(t, err)
}

// TestNewEventStore_ReadonlyDB_JournalModeError covers the PRAGMA
// journal_mode=WAL error branch: create a valid sqlite file, then strip
// write permission from both the file and its parent directory so the PRAGMA
// (which needs to write the WAL header) fails while gorm.Open itself still
// succeeds.
func TestNewEventStore_ReadonlyDB_JournalModeError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; permission denial has no effect")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "ro.sqlite")

	s, err := NewEventStore(path)
	require.NoError(t, err)
	type closer interface{ Close() error }
	require.NoError(t, s.(closer).Close())

	require.NoError(t, os.Chmod(path, 0o444))
	require.NoError(t, os.Chmod(dir, 0o555))
	t.Cleanup(func() {
		_ = os.Chmod(dir, 0o755)
		_ = os.Chmod(path, 0o644)
	})

	_, err = NewEventStore(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "journal_mode")
}

func TestEventStore_Append_ThenReadFrom(t *testing.T) {
	s := newTestEventStore(t)
	ctx := context.Background()
	require.NoError(t, s.Append(ctx, "agg-1", 1, []byte("e1")))
	blobs, err := s.ReadFrom(ctx, "agg-1", 1)
	require.NoError(t, err)
	require.Len(t, blobs, 1)
	assert.Equal(t, []byte("e1"), blobs[0])
}

func TestEventStore_Append_VersionConflict(t *testing.T) {
	s := newTestEventStore(t)
	ctx := context.Background()
	require.NoError(t, s.Append(ctx, "agg-1", 1, []byte("first")))
	err := s.Append(ctx, "agg-1", 1, []byte("dup"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, models.ErrPipelineFailed))
}

func TestEventStore_ReadFrom_Offset(t *testing.T) {
	s := newTestEventStore(t)
	ctx := context.Background()
	require.NoError(t, s.Append(ctx, "agg-1", 1, []byte("e1")))
	require.NoError(t, s.Append(ctx, "agg-1", 2, []byte("e2")))
	blobs, err := s.ReadFrom(ctx, "agg-1", 2)
	require.NoError(t, err)
	require.Len(t, blobs, 1)
	assert.Equal(t, []byte("e2"), blobs[0])
}

func TestEventStore_ReadRange_TruncatesCount(t *testing.T) {
	s := newTestEventStore(t)
	ctx := context.Background()
	require.NoError(t, s.Append(ctx, "agg-1", 1, []byte("e1")))
	require.NoError(t, s.Append(ctx, "agg-1", 2, []byte("e2")))
	blobs, err := s.ReadRange(ctx, "agg-1", 1, 100)
	require.NoError(t, err)
	assert.Len(t, blobs, 2)
}

// counter is the extra-method surface eventStore keeps beyond the v0.8
// models.Store interface (which dropped Count). Tests reach it by assertion,
// mirroring the AggregateIDs tests.
type counter interface {
	Count(ctx context.Context, aggregateID string, fromVersion int64) (int64, error)
}

func TestEventStore_Count(t *testing.T) {
	s := newTestEventStore(t)
	ctx := context.Background()
	require.NoError(t, s.Append(ctx, "agg-1", 1, []byte("e1")))
	require.NoError(t, s.Append(ctx, "agg-1", 2, []byte("e2")))
	c, ok := s.(counter)
	require.True(t, ok)
	count, err := c.Count(ctx, "agg-1", 1)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)
}

func TestEventStore_Delete_RemovesAggregate(t *testing.T) {
	s := newTestEventStore(t)
	ctx := context.Background()
	require.NoError(t, s.Append(ctx, "agg-1", 1, []byte("e1")))
	require.NoError(t, s.Delete(ctx, "agg-1"))
	blobs, err := s.ReadFrom(ctx, "agg-1", 1)
	require.NoError(t, err)
	assert.Empty(t, blobs)
}

func TestEventStore_Delete_ContextCancelled(t *testing.T) {
	s := newTestEventStore(t)
	err := s.Delete(cancelledCtx(), "agg-1")
	assert.Error(t, err)
}

func TestEventStore_Close_Success(t *testing.T) {
	s, err := NewEventStore(":memory:")
	require.NoError(t, err)
	type closer interface {
		Close() error
	}
	cl, ok := s.(closer)
	require.True(t, ok, "expected eventStore to implement io.Closer")
	assert.NoError(t, cl.Close())
}

func TestEventStore_Append_ContextCancelled(t *testing.T) {
	s := newTestEventStore(t)
	err := s.Append(cancelledCtx(), "agg-1", 1, []byte("d"))
	assert.Error(t, err)
}

func TestEventStore_ReadFrom_ContextCancelled(t *testing.T) {
	s := newTestEventStore(t)
	_, err := s.ReadFrom(cancelledCtx(), "agg-1", 1)
	assert.Error(t, err)
}

func TestEventStore_ReadRange_ContextCancelled(t *testing.T) {
	s := newTestEventStore(t)
	_, err := s.ReadRange(cancelledCtx(), "agg-1", 1, 10)
	assert.Error(t, err)
}

func TestEventStore_Count_ContextCancelled(t *testing.T) {
	s := newTestEventStore(t)
	c, ok := s.(counter)
	require.True(t, ok)
	_, err := c.Count(cancelledCtx(), "agg-1", 1)
	assert.Error(t, err)
}

func TestEventStore_AggregateIDs_ReturnsDistinct(t *testing.T) {
	s := newTestEventStore(t)
	ctx := context.Background()
	require.NoError(t, s.Append(ctx, "agg-1", 1, []byte("a")))
	require.NoError(t, s.Append(ctx, "agg-1", 2, []byte("b")))
	require.NoError(t, s.Append(ctx, "agg-2", 1, []byte("c")))

	lister, ok := s.(interface {
		AggregateIDs(ctx context.Context) ([]string, error)
	})
	require.True(t, ok)

	ids, err := lister.AggregateIDs(ctx)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"agg-1", "agg-2"}, ids)
}

func TestEventStore_AggregateIDs_ContextCancelled(t *testing.T) {
	s := newTestEventStore(t)
	lister, ok := s.(interface {
		AggregateIDs(ctx context.Context) ([]string, error)
	})
	require.True(t, ok)

	_, err := lister.AggregateIDs(cancelledCtx())
	assert.Error(t, err)
}

func TestEventStore_AggregateIDs_EmptyStore(t *testing.T) {
	s := newTestEventStore(t)
	lister, ok := s.(interface {
		AggregateIDs(ctx context.Context) ([]string, error)
	})
	require.True(t, ok)

	ids, err := lister.AggregateIDs(context.Background())
	require.NoError(t, err)
	assert.Empty(t, ids)
}
