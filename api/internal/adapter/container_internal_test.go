package adapter

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/metadata"
)

// This file white-box tests newLocked's error/cleanup paths and the small
// close* helpers directly; the black-box container_test.go only exercises the
// happy paths reachable through the exported New/Close surface.

// blockPath plants a directory at path so a later sqlite Open (which needs to
// create/open a regular file there) fails.
func blockPath(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(path, 0o750))
}

func TestNewLocked_ChatEventStoreOpenError(t *testing.T) {
	home := t.TempDir()
	stateDir := metadata.GetStateDirPathAt(home)
	require.NoError(t, os.MkdirAll(stateDir, 0o750))
	blockPath(t, filepath.Join(stateDir, "chat_"+eventStreamDBName))

	c, err := newLocked(home, stateDir, nil)
	assert.Error(t, err)
	assert.Nil(t, c)
	assert.Contains(t, err.Error(), "chat event store")
}

func TestNewLocked_ReviewThreadEventStoreOpenError(t *testing.T) {
	home := t.TempDir()
	stateDir := metadata.GetStateDirPathAt(home)
	require.NoError(t, os.MkdirAll(stateDir, 0o750))
	// The reviewthread event log now lives at state/events/review_thread.db;
	// plant a directory there so its sqlite open fails.
	eventsDir := metadata.GetEventsPathAt(home)
	require.NoError(t, os.MkdirAll(eventsDir, 0o750))
	blockPath(t, filepath.Join(eventsDir, reviewThreadDBName))

	c, err := newLocked(home, stateDir, nil)
	assert.Error(t, err)
	assert.Nil(t, c)
	assert.Contains(t, err.Error(), "review thread event store")
}

func TestNewLocked_GlobalViewOpenError(t *testing.T) {
	home := t.TempDir()
	stateDir := metadata.GetStateDirPathAt(home)
	require.NoError(t, os.MkdirAll(stateDir, 0o750))
	blockPath(t, filepath.Join(stateDir, viewDBName))

	c, err := newLocked(home, stateDir, nil)
	assert.Error(t, err)
	assert.Nil(t, c)
	assert.Contains(t, err.Error(), "global view")
}

func TestNewLocked_Success(t *testing.T) {
	home := t.TempDir()
	stateDir := metadata.GetStateDirPathAt(home)
	require.NoError(t, os.MkdirAll(stateDir, 0o750))

	c, err := newLocked(home, stateDir, nil)
	require.NoError(t, err)
	require.NotNil(t, c)
	t.Cleanup(func() { _ = c.Close() })

	assert.Equal(t, home, c.crowbarHome)
	assert.NotNil(t, c.chatES)
	assert.NotNil(t, c.reviewThreadES)
	assert.NotNil(t, c.globalView)
	assert.Len(t, c.globalClosers, 2)
}

// fakeCloser records whether Close was called and what it returned.
type fakeCloser struct {
	called bool
	err    error
}

func (f *fakeCloser) Close() error {
	f.called = true
	return f.err
}

// notACloser deliberately does not implement io.Closer.
type notACloser struct{}

func TestCloseIfCloser_ClosesWhenImplemented(t *testing.T) {
	fc := &fakeCloser{}
	closeIfCloser(fc)
	assert.True(t, fc.called)
}

func TestCloseIfCloser_NoopWhenNotACloser(t *testing.T) {
	assert.NotPanics(t, func() { closeIfCloser(notACloser{}) })
}

// fakeStoreCloser satisfies asynxModels.Store (via the embedded nil interface,
// never invoked in these tests) and io.Closer, exercising closeEventStore's
// type-asserts-to-Closer branch.
type fakeStoreCloser struct {
	asynxModels.Store
	closer *fakeCloser
}

func (f fakeStoreCloser) Close() error { return f.closer.Close() }

// notACloserStore satisfies asynxModels.Store but not io.Closer.
type notACloserStore struct {
	asynxModels.Store
}

func TestCloseEventStore_ClosesWhenImplemented(t *testing.T) {
	fc := &fakeCloser{}
	assert.NoError(t, closeEventStore(fakeStoreCloser{closer: fc}))
	assert.True(t, fc.called)
}

func TestCloseEventStore_PropagatesCloseError(t *testing.T) {
	wantErr := errors.New("boom")
	fc := &fakeCloser{err: wantErr}
	assert.ErrorIs(t, closeEventStore(fakeStoreCloser{closer: fc}), wantErr)
}

func TestCloseEventStore_NoopWhenNotACloser(t *testing.T) {
	assert.NoError(t, closeEventStore(notACloserStore{}))
}

func TestCloseViewDB_ErrorOnInvalidConnPool(t *testing.T) {
	// A *gorm.DB with a non-nil Config but no ConnPool set makes DB() return
	// gorm.ErrInvalidDB (see gorm.io/gorm.(*DB).DB); the embedded *Config must be
	// non-nil or the promoted ConnPool field access itself panics.
	err := closeViewDB(&gormdb.DB{Config: &gormdb.Config{}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "db handle")
}

func TestClose_PropagatesGlobalViewCloseError(t *testing.T) {
	home := t.TempDir()
	c, err := New(WithHomeDir(home))
	require.NoError(t, err)

	// Swap in a globalView whose DB() fails, forcing Close's closeViewDB branch
	// to error rather than silently succeeding.
	c.globalView = &gormdb.DB{Config: &gormdb.Config{}}

	closeErr := c.Close()
	assert.Error(t, closeErr)
	assert.Contains(t, closeErr.Error(), "close global view")
}
