package persistence_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/terminal/internal/persistence"
)

func TestWriteReadBuf_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	data := []byte("hello terminal scrollback")

	require.NoError(t, persistence.WriteBuf(dir, "sess-abc", data))

	got, err := persistence.ReadBuf(dir, "sess-abc")
	require.NoError(t, err)
	assert.Equal(t, data, got)
}

func TestReadBuf_MissingFileReturnsNilNil(t *testing.T) {
	dir := t.TempDir()

	got, err := persistence.ReadBuf(dir, "nonexistent")
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestDeleteBuf_RemovesFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, persistence.WriteBuf(dir, "s1", []byte("data")))

	require.NoError(t, persistence.DeleteBuf(dir, "s1"))

	got, err := persistence.ReadBuf(dir, "s1")
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestDeleteBuf_MissingFileIsNotError(t *testing.T) {
	dir := t.TempDir()
	assert.NoError(t, persistence.DeleteBuf(dir, "does-not-exist"))
}

func TestWriteBuf_ConcurrentWritesNoCorruption(t *testing.T) {
	dir := t.TempDir()
	sessionID := "sess-concurrent"

	payloadA := bytes.Repeat([]byte("A"), 1024)
	payloadB := bytes.Repeat([]byte("B"), 1024)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			_ = persistence.WriteBuf(dir, sessionID, payloadA)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			_ = persistence.WriteBuf(dir, sessionID, payloadB)
		}
	}()

	wg.Wait()

	got, err := persistence.ReadBuf(dir, sessionID)
	require.NoError(t, err)
	require.NotNil(t, got)

	// Content must be exactly one of the two payloads — no interleaving.
	isA := bytes.Equal(got, payloadA)
	isB := bytes.Equal(got, payloadB)
	assert.True(t, isA || isB, "content must be exactly one of the two payloads, got len=%d", len(got))
}

func TestWriteBuf_NoLeftoverTempFiles(t *testing.T) {
	dir := t.TempDir()
	sessionID := "sess-cleanup"
	data := []byte("some scrollback bytes")

	require.NoError(t, persistence.WriteBuf(dir, sessionID, data))

	// After success there must be no .buf-* temp files left.
	pattern := filepath.Join(dir, fmt.Sprintf("%s.buf-*", sessionID))
	matches, err := filepath.Glob(pattern)
	require.NoError(t, err)
	assert.Empty(t, matches, "leftover temp files found: %v", matches)

	// The real .buf file must exist.
	_, err = os.Stat(filepath.Join(dir, sessionID+".buf"))
	assert.NoError(t, err)
}

func TestWriteBuf_CreatesDir(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "nested", "subdir")

	require.NoError(t, persistence.WriteBuf(dir, "s", []byte("hi")))

	got, err := persistence.ReadBuf(dir, "s")
	require.NoError(t, err)
	assert.Equal(t, []byte("hi"), got)
}

func TestWriteBuf_MkdirError(t *testing.T) {
	base := t.TempDir()
	// Make a regular file, then ask to write under a path that treats it as a
	// directory — MkdirAll cannot create a dir beneath a file.
	blocker := filepath.Join(base, "iam-a-file")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))

	err := persistence.WriteBuf(filepath.Join(blocker, "child"), "s", []byte("data"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mkdir")
}

func TestWriteBuf_RenameError(t *testing.T) {
	dir := t.TempDir()
	// Occupy the destination path with a NON-EMPTY directory so renaming the
	// staging file over it fails (rename onto a non-empty dir is not allowed).
	dest := filepath.Join(dir, "s.buf")
	require.NoError(t, os.Mkdir(dest, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dest, "occupant"), []byte("x"), 0o600))

	err := persistence.WriteBuf(dir, "s", []byte("data"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rename")

	// No staging temp file should be left behind after the failed rename.
	matches, gErr := filepath.Glob(filepath.Join(dir, "s.buf-*"))
	require.NoError(t, gErr)
	assert.Empty(t, matches, "leftover temp files after rename failure: %v", matches)
}

func TestReadBuf_RealIOError(t *testing.T) {
	dir := t.TempDir()
	// A directory at the .buf path makes os.ReadFile fail with a non-NotExist
	// error (EISDIR), exercising the wrapped-error branch.
	require.NoError(t, os.Mkdir(filepath.Join(dir, "s.buf"), 0o755))

	got, err := persistence.ReadBuf(dir, "s")
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "read s")
}

func TestDeleteBuf_RealIOError(t *testing.T) {
	dir := t.TempDir()
	// A non-empty directory at the .buf path makes os.Remove fail with a
	// non-NotExist error (ENOTEMPTY), exercising the wrapped-error branch.
	bufDir := filepath.Join(dir, "s.buf")
	require.NoError(t, os.Mkdir(bufDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bufDir, "occupant"), []byte("x"), 0o600))

	err := persistence.DeleteBuf(dir, "s")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete s")
}
