package attachments_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/attachments"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/engine/fs/safepath"
)

func TestUpload_ChunksAssembleIntoTheStoredAttachment(t *testing.T) {
	dir := t.TempDir()
	data := pngBytes()
	received, err := attachments.BeginUpload(dir, "ab12", "photo.png", int64(len(data)))
	require.NoError(t, err)
	assert.Zero(t, received)

	received, err = attachments.AppendUpload(dir, "ab12", 0, bytes.NewReader(data[:10]))
	require.NoError(t, err)
	assert.EqualValues(t, 10, received)
	received, err = attachments.AppendUpload(dir, "ab12", 10, bytes.NewReader(data[10:]))
	require.NoError(t, err)
	assert.EqualValues(t, len(data), received)

	fileName, contentType, err := attachments.CompleteUpload(dir, "ab12", "photo.png", int64(len(data)))
	require.NoError(t, err)
	assert.Equal(t, "ab12-photo.png", fileName)
	assert.Equal(t, "image/png", contentType)
	got, err := os.ReadFile(filepath.Join(dir, fileName))
	require.NoError(t, err)
	assert.Equal(t, data, got)
	_, err = os.Stat(filepath.Join(dir, ".uploads", "ab12.part"))
	require.ErrorIs(t, err, os.ErrNotExist)
	info, err := os.Stat(filepath.Join(dir, fileName))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestUpload_ResendingAChunkIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	_, err := attachments.BeginUpload(dir, "ab12", "a.bin", 8)
	require.NoError(t, err)
	_, err = attachments.AppendUpload(dir, "ab12", 0, bytes.NewReader([]byte("abcd")))
	require.NoError(t, err)
	received, err := attachments.AppendUpload(dir, "ab12", 0, bytes.NewReader([]byte("abcd")))
	require.NoError(t, err)
	assert.EqualValues(t, 4, received)
}

func TestUpload_AGapIsAConflict(t *testing.T) {
	dir := t.TempDir()
	_, err := attachments.BeginUpload(dir, "ab12", "a.bin", 8)
	require.NoError(t, err)
	_, err = attachments.AppendUpload(dir, "ab12", 4, bytes.NewReader([]byte("abcd")))
	require.ErrorIs(t, err, problem.ErrConflict)
}

func TestUpload_BeginAgainReportsWhatWasReceived(t *testing.T) {
	dir := t.TempDir()
	_, err := attachments.BeginUpload(dir, "ab12", "a.bin", 8)
	require.NoError(t, err)
	_, err = attachments.AppendUpload(dir, "ab12", 0, bytes.NewReader([]byte("abcd")))
	require.NoError(t, err)
	received, err := attachments.BeginUpload(dir, "ab12", "a.bin", 8)
	require.NoError(t, err)
	assert.EqualValues(t, 4, received)
}

func TestUpload_BeginRefusesAnOversizeDeclaration(t *testing.T) {
	_, err := attachments.BeginUpload(t.TempDir(), "ab12", "a.bin", attachments.MaxBytes+1)
	require.ErrorIs(t, err, safepath.ErrFileTooLarge)
}

func TestUpload_AppendRefusesBytesPastTheCap(t *testing.T) {
	dir := t.TempDir()
	_, err := attachments.BeginUpload(dir, "ab12", "a.bin", attachments.MaxBytes)
	require.NoError(t, err)
	_, err = attachments.AppendUpload(dir, "ab12", 0, bytes.NewReader(make([]byte, attachments.MaxBytes+1)))
	require.ErrorIs(t, err, safepath.ErrFileTooLarge)
}

func TestUpload_RefusesAnInvalidIDAndABlankName(t *testing.T) {
	dir := t.TempDir()
	_, err := attachments.BeginUpload(dir, "../escape", "a.bin", 1)
	require.ErrorIs(t, err, problem.ErrInvalidArgument)
	_, err = attachments.BeginUpload(dir, "ab12", "", 1)
	require.ErrorIs(t, err, problem.ErrInvalidArgument)
	_, err = attachments.AppendUpload(dir, "../escape", 0, bytes.NewReader(nil))
	require.ErrorIs(t, err, problem.ErrInvalidArgument)
	require.ErrorIs(t, attachments.AbortUpload(dir, "../escape"), problem.ErrInvalidArgument)
}

func TestUpload_AppendWithoutBeginIsNotFound(t *testing.T) {
	_, err := attachments.AppendUpload(t.TempDir(), "ab12", 0, bytes.NewReader([]byte("x")))
	require.ErrorIs(t, err, attachments.ErrNotFound)
}

func TestUpload_CompleteRefusesAShortUpload(t *testing.T) {
	dir := t.TempDir()
	_, err := attachments.BeginUpload(dir, "ab12", "a.bin", 8)
	require.NoError(t, err)
	_, err = attachments.AppendUpload(dir, "ab12", 0, bytes.NewReader([]byte("abcd")))
	require.NoError(t, err)
	_, _, err = attachments.CompleteUpload(dir, "ab12", "a.bin", 8)
	require.ErrorIs(t, err, problem.ErrConflict)
}

func TestUpload_CompleteNeverOverwritesAStoredAttachment(t *testing.T) {
	dir := t.TempDir()
	_, _, err := attachments.Store(dir, "ab12", "a.bin", []byte("old"))
	require.NoError(t, err)
	_, err = attachments.BeginUpload(dir, "ab12", "a.bin", 3)
	require.NoError(t, err)
	_, err = attachments.AppendUpload(dir, "ab12", 0, bytes.NewReader([]byte("new")))
	require.NoError(t, err)
	_, _, err = attachments.CompleteUpload(dir, "ab12", "a.bin", 3)
	require.ErrorIs(t, err, problem.ErrConflict)
	got, err := os.ReadFile(filepath.Join(dir, "ab12-a.bin"))
	require.NoError(t, err)
	assert.Equal(t, "old", string(got))
}

func TestUpload_AbortDeletesTheStagingFileAndIsRepeatable(t *testing.T) {
	dir := t.TempDir()
	_, err := attachments.BeginUpload(dir, "ab12", "a.bin", 4)
	require.NoError(t, err)
	require.NoError(t, attachments.AbortUpload(dir, "ab12"))
	require.NoError(t, attachments.AbortUpload(dir, "ab12"))
	entries, err := os.ReadDir(filepath.Join(dir, ".uploads"))
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestUpload_BeginSweepsStalePartsOnly(t *testing.T) {
	dir := t.TempDir()
	_, err := attachments.BeginUpload(dir, "old1", "a.bin", 4)
	require.NoError(t, err)
	_, err = attachments.BeginUpload(dir, "new1", "a.bin", 4)
	require.NoError(t, err)
	stale := time.Now().Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(filepath.Join(dir, ".uploads", "old1.part"), stale, stale))

	_, err = attachments.BeginUpload(dir, "new2", "a.bin", 4)
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dir, ".uploads", "old1.part"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(dir, ".uploads", "new1.part"))
	assert.NoError(t, err)
}
