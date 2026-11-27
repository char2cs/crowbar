package persistence

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/testutil"
)

type fakeTempFile struct {
	real        *os.File
	failWrite   bool
	failSync    bool
	failClose   bool
	closeCalled bool
	afterClose  func()
}

var errInjected = errors.New("injected failure")

func (f *fakeTempFile) Write(p []byte) (int, error) {
	if f.failWrite {
		return 0, errInjected
	}
	return f.real.Write(p)
}

func (f *fakeTempFile) Name() string { return f.real.Name() }

func (f *fakeTempFile) Sync() error {
	if f.failSync {
		return errInjected
	}
	return f.real.Sync()
}

func (f *fakeTempFile) Close() error {
	f.closeCalled = true
	cerr := f.real.Close()
	if f.afterClose != nil {
		f.afterClose()
	}
	if f.failClose {
		return errInjected
	}
	return cerr
}

func TestWriteBuf_WriteError(t *testing.T) {
	dir := t.TempDir()
	orig := createTemp
	t.Cleanup(func() { createTemp = orig })

	var staged string
	createTemp = func(d, pattern string) (tempFile, error) {
		real, err := os.CreateTemp(d, pattern)
		require.NoError(t, err)
		staged = real.Name()
		return &fakeTempFile{real: real, failWrite: true}, nil
	}

	err := WriteBuf(dir, "s", []byte("data"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write temp")

	_, statErr := os.Stat(staged)
	assert.True(t, os.IsNotExist(statErr), "staging temp file should be removed")
	_, statErr = os.Stat(filepath.Join(dir, "s.buf"))
	assert.True(t, os.IsNotExist(statErr), "no .buf should be committed on write failure")
}

func TestWriteBuf_SyncError(t *testing.T) {
	dir := t.TempDir()
	orig := createTemp
	t.Cleanup(func() { createTemp = orig })

	var staged string
	createTemp = func(d, pattern string) (tempFile, error) {
		real, err := os.CreateTemp(d, pattern)
		require.NoError(t, err)
		staged = real.Name()
		return &fakeTempFile{real: real, failSync: true}, nil
	}

	err := WriteBuf(dir, "s", []byte("data"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sync temp")

	_, statErr := os.Stat(staged)
	assert.True(t, os.IsNotExist(statErr), "staging temp file should be removed")
}

func TestWriteBuf_CloseError(t *testing.T) {
	dir := t.TempDir()
	orig := createTemp
	t.Cleanup(func() { createTemp = orig })

	var staged string
	createTemp = func(d, pattern string) (tempFile, error) {
		real, err := os.CreateTemp(d, pattern)
		require.NoError(t, err)
		staged = real.Name()
		return &fakeTempFile{real: real, failClose: true}, nil
	}

	err := WriteBuf(dir, "s", []byte("data"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "close temp")

	_, statErr := os.Stat(staged)
	assert.True(t, os.IsNotExist(statErr), "staging temp file should be removed on close failure")
}

func TestWriteBuf_CreateTempError(t *testing.T) {
	dir := t.TempDir()
	orig := createTemp
	t.Cleanup(func() { createTemp = orig })

	createTemp = func(string, string) (tempFile, error) {
		return nil, errInjected
	}

	err := WriteBuf(dir, "s", []byte("data"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create temp")
}

func TestWriteBuf_ACleanupFailureIsReturnedWithTheRenameFailure(t *testing.T) {
	testutil.RequirePermissionEnforcement(t)
	dir := t.TempDir()
	orig := createTemp
	t.Cleanup(func() { createTemp = orig })
	t.Cleanup(func() { require.NoError(t, os.Chmod(dir, 0o700)) })

	createTemp = func(d, pattern string) (tempFile, error) {
		real, err := os.CreateTemp(d, pattern)
		require.NoError(t, err)
		return &fakeTempFile{real: real, afterClose: func() { require.NoError(t, os.Chmod(dir, 0o500)) }}, nil
	}

	err := WriteBuf(dir, "s", []byte("data"))

	require.ErrorIs(t, err, fs.ErrPermission)
	assert.Contains(t, err.Error(), "persistence: rename")
	assert.Contains(t, err.Error(), "persistence: remove temp")
}
