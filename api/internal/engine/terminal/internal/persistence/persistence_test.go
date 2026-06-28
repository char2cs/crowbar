package persistence_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/terminal/internal/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
