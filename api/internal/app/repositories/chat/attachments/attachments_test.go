package attachments_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/attachments"
	"github.com/char2cs/crowbar/api/internal/engine/fs/safepath"
)

func pngBytes() []byte {
	return append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...)
}

func TestStore_WritesTheIDPrefixedFilename(t *testing.T) {
	dir := t.TempDir()
	fileName, contentType, err := attachments.Store(dir, "ab12", "photo.png", pngBytes())
	require.NoError(t, err)
	assert.Equal(t, "ab12-photo.png", fileName)
	assert.Equal(t, "image/png", contentType)
	data, err := os.ReadFile(filepath.Join(dir, fileName))
	require.NoError(t, err)
	assert.Equal(t, pngBytes(), data)
}

func TestStore_SanitizesAPathTraversalOriginalName(t *testing.T) {
	dir := t.TempDir()
	fileName, _, err := attachments.Store(dir, "ab12", "../../etc/passwd", []byte("x"))
	require.NoError(t, err)
	assert.Equal(t, "ab12-passwd", fileName)
}

func TestStore_CollapsesSpacesAndParensInTheOriginalName(t *testing.T) {
	dir := t.TempDir()
	fileName, _, err := attachments.Store(dir, "ab12", "my photo (1).png", []byte("x"))
	require.NoError(t, err)
	assert.NotContains(t, fileName, " ")
	assert.NotContains(t, fileName, "(")
}

func TestStore_RefusesAnInvalidID(t *testing.T) {
	_, _, err := attachments.Store(t.TempDir(), "../escape", "a.png", []byte("x"))
	assert.ErrorIs(t, err, apperr.ErrInvalidArgument)
}

func TestStore_RefusesABlankOriginalName(t *testing.T) {
	_, _, err := attachments.Store(t.TempDir(), "ab12", "", []byte("x"))
	assert.ErrorIs(t, err, apperr.ErrInvalidArgument)
}

func TestStore_RefusesOversizeData(t *testing.T) {
	oversized := make([]byte, attachments.MaxBytes+1)
	_, _, err := attachments.Store(t.TempDir(), "ab12", "big.bin", oversized)
	assert.ErrorIs(t, err, safepath.ErrFileTooLarge)
}

func TestStore_RefusesACollidingFilename(t *testing.T) {
	dir := t.TempDir()
	_, _, err := attachments.Store(dir, "ab12", "photo.png", []byte("first"))
	require.NoError(t, err)
	_, _, err = attachments.Store(dir, "ab12", "photo.png", []byte("second"))
	assert.ErrorIs(t, err, apperr.ErrConflict)
}

func TestRead_RoundTripsStoredBytes(t *testing.T) {
	dir := t.TempDir()
	fileName, _, err := attachments.Store(dir, "ab12", "photo.png", pngBytes())
	require.NoError(t, err)

	data, contentType, err := attachments.Read(dir, fileName)
	require.NoError(t, err)
	assert.Equal(t, pngBytes(), data)
	assert.Equal(t, "image/png", contentType)
}

func TestRead_MissingFileIsNotFound(t *testing.T) {
	_, _, err := attachments.Read(t.TempDir(), "no-such-file.png")
	assert.ErrorIs(t, err, attachments.ErrNotFound)
}

func TestRead_RefusesAFileNameCarryingASeparator(t *testing.T) {
	dir := t.TempDir()
	_, _, err := attachments.Read(dir, "../outside.png")
	assert.ErrorIs(t, err, attachments.ErrNotFound)
}

func TestSyntheticName_UsesTheSniffedExtension(t *testing.T) {
	name := attachments.SyntheticName("image/png", func() string { return "20260904T120000" })
	assert.Equal(t, "pasted-image-20260904T120000.png", name)
}

func TestContentType_SVGGetsItsOwnMIMEType(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)
	assert.Equal(t, "image/svg+xml", attachments.ContentType(svg))
}

// Additional tests for complete coverage

func TestStore_CreatesDirectoryIfNeeded(t *testing.T) {
	baseDir := t.TempDir()
	nestedDir := filepath.Join(baseDir, "nested", "attachment", "dir")
	fileName, _, err := attachments.Store(nestedDir, "ab12", "test.png", pngBytes())
	require.NoError(t, err)
	assert.Equal(t, "ab12-test.png", fileName)
	data, err := os.ReadFile(filepath.Join(nestedDir, fileName))
	require.NoError(t, err)
	assert.Equal(t, pngBytes(), data)
}

func TestContentType_JPEG(t *testing.T) {
	jpeg := append([]byte("\xff\xd8\xff"), make([]byte, 32)...)
	ct := attachments.ContentType(jpeg)
	assert.Equal(t, "image/jpeg", ct)
}

func TestContentType_GIF(t *testing.T) {
	gif := append([]byte("GIF89a"), make([]byte, 32)...)
	ct := attachments.ContentType(gif)
	assert.Equal(t, "image/gif", ct)
}

func TestContentType_PDF(t *testing.T) {
	pdf := append([]byte("%PDF-1.4"), make([]byte, 32)...)
	ct := attachments.ContentType(pdf)
	assert.Equal(t, "application/pdf", ct)
}

func TestContentType_SVGInLargeData(t *testing.T) {
	// Test that SVG is detected even when data exceeds 512 bytes
	svg := make([]byte, 600)
	copy(svg, []byte("<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>"))
	assert.Equal(t, "image/svg+xml", attachments.ContentType(svg))
}

func TestContentType_SVGShortData(t *testing.T) {
	// Test SVG detection with short data
	svg := []byte("<svg></svg>")
	assert.Equal(t, "image/svg+xml", attachments.ContentType(svg))
}

func TestContentType_PlainTextFallback(t *testing.T) {
	text := []byte("this is plain text")
	ct := attachments.ContentType(text)
	assert.Equal(t, "text/plain; charset=utf-8", ct)
}

func TestContentType_BinaryFallback(t *testing.T) {
	binary := []byte("\x00\x01\x02\x03\x04\x05")
	ct := attachments.ContentType(binary)
	assert.Equal(t, "application/octet-stream", ct)
}

func TestSyntheticName_JPEGExtension(t *testing.T) {
	name := attachments.SyntheticName("image/jpeg", func() string { return "ts1" })
	assert.Equal(t, "pasted-image-ts1.jpg", name)
}

func TestSyntheticName_GIFExtension(t *testing.T) {
	name := attachments.SyntheticName("image/gif", func() string { return "ts2" })
	assert.Equal(t, "pasted-image-ts2.gif", name)
}

func TestSyntheticName_WebPExtension(t *testing.T) {
	name := attachments.SyntheticName("image/webp", func() string { return "ts3" })
	assert.Equal(t, "pasted-image-ts3.webp", name)
}

func TestSyntheticName_PDFExtension(t *testing.T) {
	name := attachments.SyntheticName("application/pdf", func() string { return "ts4" })
	assert.Equal(t, "pasted-image-ts4.pdf", name)
}

func TestSyntheticName_UnknownTypeExtension(t *testing.T) {
	name := attachments.SyntheticName("application/unknown", func() string { return "ts5" })
	assert.Equal(t, "pasted-image-ts5.bin", name)
}

func TestRead_EmptyFileName(t *testing.T) {
	_, _, err := attachments.Read(t.TempDir(), "")
	assert.ErrorIs(t, err, attachments.ErrNotFound)
}

func TestRead_FileNameWithDotDot(t *testing.T) {
	dir := t.TempDir()
	_, _, err := attachments.Read(dir, "file/../other.png")
	assert.ErrorIs(t, err, attachments.ErrNotFound)
}

func TestRead_FileNameIsJustSlash(t *testing.T) {
	dir := t.TempDir()
	_, _, err := attachments.Read(dir, "/")
	assert.ErrorIs(t, err, attachments.ErrNotFound)
}

func TestValidID_ValidNanoidStyle(t *testing.T) {
	assert.True(t, attachments.ValidID("ab12"))
	assert.True(t, attachments.ValidID("ABC123-_xyz"))
	assert.True(t, attachments.ValidID("a"))
	assert.True(t, attachments.ValidID("A"))
	assert.True(t, attachments.ValidID("0"))
	assert.True(t, attachments.ValidID("_"))
	assert.True(t, attachments.ValidID("-"))
}

func TestValidID_InvalidIDsRejected(t *testing.T) {
	assert.False(t, attachments.ValidID(""))
	assert.False(t, attachments.ValidID("a/b"))
	assert.False(t, attachments.ValidID("a.b"))
	assert.False(t, attachments.ValidID("a b"))
	assert.False(t, attachments.ValidID("a@b"))
	assert.False(t, attachments.ValidID("../escape"))
	assert.False(t, attachments.ValidID(strings.Repeat("a", 129)))
}

func TestStore_SanitizesBrackets(t *testing.T) {
	dir := t.TempDir()
	fileName, _, err := attachments.Store(dir, "ab12", "my[file].png", []byte("x"))
	require.NoError(t, err)
	assert.NotContains(t, fileName, "[")
	assert.NotContains(t, fileName, "]")
}

func TestStore_SanitizesMultipleWhitespace(t *testing.T) {
	dir := t.TempDir()
	fileName, _, err := attachments.Store(dir, "ab12", "my   file.png", []byte("x"))
	require.NoError(t, err)
	// Multiple spaces should collapse to single underscore
	assert.NotContains(t, fileName, "   ")
}

func TestStore_MaxBytesExactlyAllowed(t *testing.T) {
	dir := t.TempDir()
	data := make([]byte, attachments.MaxBytes)
	fileName, _, err := attachments.Store(dir, "ab12", "max.bin", data)
	require.NoError(t, err)
	assert.Equal(t, "ab12-max.bin", fileName)
}

func TestRead_ContentTypeSniffedFromStoredData(t *testing.T) {
	dir := t.TempDir()
	jpegData := append([]byte("\xff\xd8\xff"), make([]byte, 64)...)
	fileName, _, err := attachments.Store(dir, "ab12", "photo.dat", jpegData)
	require.NoError(t, err)

	data, contentType, err := attachments.Read(dir, fileName)
	require.NoError(t, err)
	assert.Equal(t, jpegData, data)
	assert.Equal(t, "image/jpeg", contentType)
}

func TestStore_OriginalNameWithMultipleDots(t *testing.T) {
	dir := t.TempDir()
	fileName, _, err := attachments.Store(dir, "ab12", "my.archive.tar.gz", []byte("x"))
	require.NoError(t, err)
	assert.Equal(t, "ab12-my.archive.tar.gz", fileName)
}

func TestStore_OriginalNameWithOnlyExtension(t *testing.T) {
	dir := t.TempDir()
	fileName, _, err := attachments.Store(dir, "ab12", ".hidden", []byte("x"))
	require.NoError(t, err)
	assert.Equal(t, "ab12-.hidden", fileName)
}

func TestRead_FileTooLargeIsNotFound(t *testing.T) {
	dir := t.TempDir()
	// Create a file that's too large
	fileName := "toolarge.bin"
	// Write a file larger than MaxBytes
	oversized := make([]byte, attachments.MaxBytes+1)
	err := os.WriteFile(filepath.Join(dir, fileName), oversized, 0o600)
	require.NoError(t, err)

	// Try to read it - should get ErrNotFound
	_, _, err = attachments.Read(dir, fileName)
	assert.ErrorIs(t, err, attachments.ErrNotFound)
}

func TestStore_OriginalNameBecomesEmptyAfterSanitization(t *testing.T) {
	// Names that sanitize to empty should be rejected
	_, _, err := attachments.Store(t.TempDir(), "ab12", ".", []byte("x"))
	assert.ErrorIs(t, err, apperr.ErrInvalidArgument)
}

func TestStore_OriginalNameWithDoubleDotsBecomesEmpty(t *testing.T) {
	_, _, err := attachments.Store(t.TempDir(), "ab12", "..", []byte("x"))
	assert.ErrorIs(t, err, apperr.ErrInvalidArgument)
}

func TestStore_OriginalNameAsPathSeparator(t *testing.T) {
	_, _, err := attachments.Store(t.TempDir(), "ab12", "/", []byte("x"))
	assert.ErrorIs(t, err, apperr.ErrInvalidArgument)
}

func TestRead_OpenErrorReturnsNotFound(t *testing.T) {
	dir := t.TempDir()
	// Create a file with restricted permissions (unreadable)
	fileName := "forbidden.bin"
	filePath := filepath.Join(dir, fileName)
	err := os.WriteFile(filePath, []byte("data"), 0o000)
	require.NoError(t, err)
	defer os.Chmod(filePath, 0o644) // Restore for cleanup

	// Try to read it - should get ErrNotFound due to permission denied
	_, _, err = attachments.Read(dir, fileName)
	assert.ErrorIs(t, err, attachments.ErrNotFound)
}

func TestStore_MkdirAllFailureReturnsError(t *testing.T) {
	dir := t.TempDir()
	// Pre-create a plain file at the path where we want to create a dir
	blockingFile := filepath.Join(dir, "blocked")
	err := os.WriteFile(blockingFile, []byte("x"), 0o600)
	require.NoError(t, err)

	// Try to store with this blocking file as the target dir
	// os.MkdirAll will fail with ENOTDIR (not a directory)
	_, _, err = attachments.Store(blockingFile, "ab12", "test.png", []byte("data"))
	assert.Error(t, err)
	// Should NOT be a conflict error - it's an MkdirAll failure
	assert.NotErrorIs(t, err, apperr.ErrConflict)
}

func TestStore_OpenFileFailureReturnsError(t *testing.T) {
	dir := t.TempDir()
	// Pre-create the dir, then restrict write permissions
	err := os.Chmod(dir, 0o500) // r-x------: can read/search but not write
	require.NoError(t, err)
	defer os.Chmod(dir, 0o755) // Restore for cleanup

	// Try to create a file in the write-protected dir
	// os.OpenFile will fail with EACCES (permission denied) on the create
	_, _, err = attachments.Store(dir, "ab12", "test.png", []byte("data"))
	assert.Error(t, err)
	// Should NOT be a conflict error - it's a create/permission failure
	assert.NotErrorIs(t, err, apperr.ErrConflict)
}
