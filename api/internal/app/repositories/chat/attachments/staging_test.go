package attachments_test

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/attachments"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/engine/fs/safepath"
)

func begin(t *testing.T, dir string, size int64) {
	t.Helper()
	_, err := attachments.BeginUpload(dir, "ab12", "a.bin", size)
	require.NoError(t, err)
}

func put(dir string, offset int64, data string) (int64, error) {
	return attachments.AppendUpload(dir, "ab12", offset, bytes.NewReader([]byte(data)))
}

func partPath(dir, id string) string { return filepath.Join(dir, ".uploads", id+".part") }

func TestUpload_ChunksAssembleIntoTheStoredAttachment(t *testing.T) {
	dir := t.TempDir()
	data := pngBytes()
	_, err := attachments.BeginUpload(dir, "ab12", "photo.png", int64(len(data)))
	require.NoError(t, err)
	_, err = attachments.AppendUpload(dir, "ab12", 0, bytes.NewReader(data[:10]))
	require.NoError(t, err)
	_, err = attachments.AppendUpload(dir, "ab12", 10, bytes.NewReader(data[10:]))
	require.NoError(t, err)

	fileName, contentType, err := attachments.CompleteUpload(dir, "ab12", "photo.png", int64(len(data)))
	require.NoError(t, err)
	assert.Equal(t, "ab12-photo.png", fileName)
	assert.Equal(t, "image/png", contentType)
	got, err := os.ReadFile(filepath.Join(dir, fileName))
	require.NoError(t, err)
	assert.Equal(t, data, got)
	assert.NoFileExists(t, partPath(dir, "ab12"))
	info, err := os.Stat(filepath.Join(dir, fileName))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestUpload_ResendsAndGapsAndResumes(t *testing.T) {
	dir := t.TempDir()
	begin(t, dir, 8)
	_, err := put(dir, 4, "abcd")
	require.ErrorIs(t, err, problem.ErrConflict, "a gap")
	for range 2 {
		received, err := put(dir, 0, "abcd")
		require.NoError(t, err)
		assert.EqualValues(t, 4, received, "a resent chunk is a no-op")
	}
	received, err := attachments.BeginUpload(dir, "ab12", "a.bin", 8)
	require.NoError(t, err)
	assert.EqualValues(t, 4, received, "begin again reports what was received")
	_, _, err = attachments.CompleteUpload(dir, "ab12", "a.bin", 8)
	require.ErrorIs(t, err, problem.ErrConflict, "a short upload")
}

func TestUpload_RefusesWhatCannotBeStored(t *testing.T) {
	dir := t.TempDir()
	_, err := attachments.BeginUpload(dir, "ab12", "a.bin", attachments.MaxBytes+1)
	require.ErrorIs(t, err, safepath.ErrFileTooLarge)
	begin(t, dir, attachments.MaxBytes)
	_, err = attachments.AppendUpload(dir, "ab12", 0, bytes.NewReader(make([]byte, attachments.MaxBytes+1)))
	require.ErrorIs(t, err, safepath.ErrFileTooLarge)
	_, err = attachments.BeginUpload(dir, "../escape", "a.bin", 1)
	require.ErrorIs(t, err, problem.ErrInvalidArgument)
	_, err = attachments.BeginUpload(dir, "ab12", "", 1)
	require.ErrorIs(t, err, problem.ErrInvalidArgument)
	_, err = attachments.AppendUpload(dir, "../escape", 0, bytes.NewReader(nil))
	require.ErrorIs(t, err, problem.ErrInvalidArgument)
	require.ErrorIs(t, attachments.AbortUpload(dir, "../escape"), problem.ErrInvalidArgument)
	_, err = put(t.TempDir(), 0, "x")
	require.ErrorIs(t, err, attachments.ErrNotFound, "no begin")
}

func TestUpload_NeverOverwritesAStoredAttachment(t *testing.T) {
	dir := t.TempDir()
	_, _, err := attachments.Store(dir, "ab12", "a.bin", []byte("old"))
	require.NoError(t, err)
	_, err = attachments.BeginUpload(dir, "ab12", "a.bin", 5)
	require.ErrorIs(t, err, problem.ErrConflict)
	_, _, err = attachments.CompleteUpload(dir, "ab12", "a.bin", 5)
	require.ErrorIs(t, err, problem.ErrConflict)
	got, err := os.ReadFile(filepath.Join(dir, "ab12-a.bin"))
	require.NoError(t, err)
	assert.Equal(t, "old", string(got))
}

func TestUpload_BeginSweepsStalePartsOnlyAndAbortIsRepeatable(t *testing.T) {
	dir := t.TempDir()
	for _, id := range []string{"old1", "new1"} {
		_, err := attachments.BeginUpload(dir, id, "a.bin", 4)
		require.NoError(t, err)
	}
	stale := time.Now().Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(partPath(dir, "old1"), stale, stale))
	_, err := attachments.BeginUpload(dir, "new2", "a.bin", 4)
	require.NoError(t, err)
	assert.NoFileExists(t, partPath(dir, "old1"))
	assert.FileExists(t, partPath(dir, "new1"))
	for range 2 {
		require.NoError(t, attachments.AbortUpload(dir, "new1"))
	}
	assert.NoFileExists(t, partPath(dir, "new1"))
}

func TestUpload_CompleteAfterALostReplyReturnsTheStoredAttachment(t *testing.T) {
	dir := t.TempDir()
	data := pngBytes()
	size := int64(len(data))
	_, err := attachments.BeginUpload(dir, "ab12", "photo.png", size)
	require.NoError(t, err)
	_, err = attachments.AppendUpload(dir, "ab12", 0, bytes.NewReader(data))
	require.NoError(t, err)
	first, firstType, err := attachments.CompleteUpload(dir, "ab12", "photo.png", size)
	require.NoError(t, err)

	received, err := attachments.BeginUpload(dir, "ab12", "photo.png", size)
	require.NoError(t, err)
	assert.Equal(t, size, received)
	again, againType, err := attachments.CompleteUpload(dir, "ab12", "photo.png", size)
	require.NoError(t, err)
	assert.Equal(t, []string{first, firstType}, []string{again, againType})
}

func TestUpload_OverlappingChunksNeverInterleave(t *testing.T) {
	for range 100 {
		dir := t.TempDir()
		begin(t, dir, 8)
		var wg sync.WaitGroup
		for _, chunk := range []string{"aaaa", "bbbbbbbb"} {
			wg.Go(func() { _, _ = put(dir, 0, chunk) })
		}
		wg.Wait()
		got, err := os.ReadFile(partPath(dir, "ab12"))
		require.NoError(t, err)
		require.Contains(t, []string{"aaaa", "bbbbbbbb"}, string(got))
	}
}
