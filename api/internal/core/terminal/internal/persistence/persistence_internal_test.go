package persistence

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeTempFile is a tempFile whose Write/Sync/Close can be made to fail, so the
// corresponding error branches in WriteBuf are exercised. It writes through to a
// real backing file on disk so the test can also assert that a failed write
// leaves no committed .buf and removes the staging file.
type fakeTempFile struct {
	real        *os.File
	failWrite   bool
	failSync    bool
	failClose   bool
	closeCalled bool
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
	// Always release the underlying fd so the test's TempDir cleanup succeeds,
	// but report the injected error to the caller when requested.
	cerr := f.real.Close()
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

	// The staging file must have been removed, and no committed .buf created.
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
