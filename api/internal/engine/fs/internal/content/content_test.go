package content_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/fs/internal/content"
)

func TestRead_TextFile(
	t *testing.T,
) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello world"), 0o600))

	fc, err := content.Read(dir, "hello.txt")
	require.NoError(t, err)
	assert.Equal(t, "hello world", fc.Content)
	assert.Empty(t, fc.Encoding)
}

func TestRead_BinaryFile(
	t *testing.T,
) {
	dir := t.TempDir()
	data := []byte{0x00, 0x01, 0x02, 0x03, 0xFF}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bin.dat"), data, 0o600))

	fc, err := content.Read(dir, "bin.dat")
	require.NoError(t, err)
	assert.Equal(t, "base64", fc.Encoding)
	assert.NotEmpty(t, fc.Content)
}

func TestRead_MissingFile(
	t *testing.T,
) {
	dir := t.TempDir()
	_, err := content.Read(dir, "missing.txt")
	require.Error(t, err)
}

// TestRead_LargeBinaryFile verifies that a file larger than 8 KiB containing
// a null byte in the first 8192 bytes is correctly detected as binary.
func TestRead_LargeBinaryFile(
	t *testing.T,
) {
	dir := t.TempDir()
	// Build 10 KiB of data with a null byte at position 100.
	data := bytes.Repeat([]byte("a"), 10240)
	data[100] = 0x00
	require.NoError(t, os.WriteFile(filepath.Join(dir, "large.bin"), data, 0o600))

	fc, err := content.Read(dir, "large.bin")
	require.NoError(t, err)
	assert.Equal(t, "base64", fc.Encoding)
}

// TestRead_LargeTextFile verifies that a file larger than 8 KiB with no null
// bytes in the first 8192 bytes is treated as text (isBinary limit branch).
func TestRead_LargeTextFile(
	t *testing.T,
) {
	dir := t.TempDir()
	// 10 KiB of plain ASCII — null byte only beyond the 8192-byte window.
	data := bytes.Repeat([]byte("x"), 8193)
	data[8192] = 0x00 // beyond the 8192-byte check window
	require.NoError(t, os.WriteFile(filepath.Join(dir, "large.txt"), data, 0o600))

	fc, err := content.Read(dir, "large.txt")
	require.NoError(t, err)
	assert.Empty(t, fc.Encoding, "expected text (null byte is past the 8 KiB scan limit)")
}

func TestWrite_CreatesFile(
	t *testing.T,
) {
	dir := t.TempDir()
	err := content.Write(dir, "new.txt", "content here")
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "new.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content here", string(data))
}

func TestWrite_CreatesParentDirs(
	t *testing.T,
) {
	dir := t.TempDir()
	err := content.Write(dir, "a/b/c.txt", "nested")
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "a/b/c.txt"))
	require.NoError(t, err)
	assert.Equal(t, "nested", string(data))
}

func TestReadWrite_RoundTrip(
	t *testing.T,
) {
	dir := t.TempDir()
	original := "hello\nworld\n"
	require.NoError(t, content.Write(dir, "round.txt", original))

	fc, err := content.Read(dir, "round.txt")
	require.NoError(t, err)
	assert.Equal(t, original, fc.Content)
}

// TestWrite_MkdirError exercises the os.MkdirAll error path by placing a
// regular file where a directory must be created.
func TestWrite_MkdirError(
	t *testing.T,
) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod restrictions differ on windows")
	}
	dir := t.TempDir()
	// "parent" exists as a file — MkdirAll cannot use it as a directory.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "parent"), []byte("x"), 0o600))

	err := content.Write(dir, "parent/child.txt", "hello")
	require.Error(t, err)
}

// TestWrite_WriteFileError exercises the os.WriteFile error path by writing
// to a read-only directory.
func TestWrite_WriteFileError(
	t *testing.T,
) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod restrictions differ on windows")
	}
	dir := t.TempDir()
	roDir := filepath.Join(dir, "readonly")
	require.NoError(t, os.MkdirAll(roDir, 0o700))
	require.NoError(t, os.Chmod(roDir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(roDir, 0o755) })

	err := content.Write(dir, "readonly/file.txt", "hello")
	require.Error(t, err)
}
