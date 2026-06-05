package sqlite

import (
	"context"
	"errors"
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

func TestEventStore_Count(t *testing.T) {
	s := newTestEventStore(t)
	ctx := context.Background()
	require.NoError(t, s.Append(ctx, "agg-1", 1, []byte("e1")))
	require.NoError(t, s.Append(ctx, "agg-1", 2, []byte("e2")))
	count, err := s.Count(ctx, "agg-1", 1)
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
	_, err := s.Count(cancelledCtx(), "agg-1", 1)
	assert.Error(t, err)
}
