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

	"github.com/char2cs/crowbar/api/internal/core/home"
)

func blockPath(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(path, 0o750))
}

func homeWithDirs(t *testing.T) home.Home {
	t.Helper()
	h, err := home.At(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(h.Events(), 0o750))
	require.NoError(t, os.MkdirAll(h.Store(), 0o750))
	return h
}

func TestOpenContainer_ReviewThreadEventStoreOpenError(t *testing.T) {
	h := homeWithDirs(t)
	blockPath(t, filepath.Join(h.Events(), reviewThreadDBName))

	c, err := openContainer(h)
	assert.Error(t, err)
	assert.Nil(t, c)
	assert.Contains(t, err.Error(), "review thread event store")
}

func TestOpenContainer_GlobalViewOpenError(t *testing.T) {
	h := homeWithDirs(t)
	blockPath(t, filepath.Join(h.State(), viewDBName))

	c, err := openContainer(h)
	assert.Error(t, err)
	assert.Nil(t, c)
	assert.Contains(t, err.Error(), "global view")
}

func TestOpenContainer_Success(t *testing.T) {
	h := homeWithDirs(t)

	c, err := openContainer(h)
	require.NoError(t, err)
	require.NotNil(t, c)
	t.Cleanup(func() { _ = c.Close() })

	assert.Equal(t, h, c.home)
	assert.NotNil(t, c.reviewThreadES)
	assert.NotNil(t, c.reviewThreadSS)
	assert.NotNil(t, c.workspaceSnapshotStore)
	assert.NotNil(t, c.agentChatSnapshotStore)
	assert.NotNil(t, c.globalView)
	assert.Len(t, c.globalClosers, 2)
}

type fakeCloser struct {
	called bool
	err    error
}

func (f *fakeCloser) Close() error {
	f.called = true
	return f.err
}

type fakeStoreCloser struct {
	asynxModels.Store
	closer *fakeCloser
}

func (f fakeStoreCloser) Close() error { return f.closer.Close() }

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
	err := closeViewDB(&gormdb.DB{Config: &gormdb.Config{}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "db handle")
}

func TestClose_PropagatesGlobalViewCloseError(t *testing.T) {
	c, err := New(WithHomeDir(t.TempDir()))
	require.NoError(t, err)

	c.globalView = &gormdb.DB{Config: &gormdb.Config{}}

	closeErr := c.Close()
	assert.Error(t, closeErr)
	assert.Contains(t, closeErr.Error(), "close global view")
}
