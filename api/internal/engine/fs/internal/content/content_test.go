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
	"github.com/char2cs/crowbar/api/internal/engine/fs/safepath"
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

// TestRegression_ContentRead_RejectsOversizeFile verifies that content.Read
// rejects files above the cap with ErrFileTooLarge and that files below the
// cap are still returned correctly (hardening finding R16).
// A 4-byte cap is injected via content.ReadWithCap so no 25 MiB file needs to
// exist on disk.
func TestRegression_ContentRead_RejectsOversizeFile(
	t *testing.T,
) {
	dir := t.TempDir()

	// File that exceeds the injected cap (5 bytes > 4-byte cap).
	require.NoError(t, os.WriteFile(filepath.Join(dir, "big.txt"), []byte("hello"), 0o600))
	_, err := content.ReadWithCap(dir, "big.txt", 4)
	require.Error(t, err)
	require.ErrorIs(t, err, content.ErrFileTooLarge, "oversized file must return ErrFileTooLarge")

	// File that fits within the injected cap (3 bytes <= 4-byte cap).
	require.NoError(t, os.WriteFile(filepath.Join(dir, "small.txt"), []byte("hi\n"), 0o600))
	fc, err := content.ReadWithCap(dir, "small.txt", 4)
	require.NoError(t, err, "file within cap must succeed")
	assert.Equal(t, "hi\n", fc.Content)
}

// escapeCases are the adversarial workspace-relative paths every fs op must
// reject (security finding R1).
var escapeCases = []struct {
	name string
	path string
}{
	{"parent traversal", "../../etc/passwd"},
	{"absolute path", "/etc/passwd"},
	{"mid-path escape", "a/../../b"},
}

// TestRegression_Read_RejectsPathEscape verifies content.Read refuses any path
// that escapes the workspace root, so it cannot read arbitrary host files.
func TestRegression_Read_RejectsPathEscape(
	t *testing.T,
) {
	root := t.TempDir()
	// A real secret outside the root to prove it is never returned.
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "passwd"), []byte("root:x:0:0"), 0o600))

	for _, tc := range escapeCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := content.Read(root, tc.path)
			require.ErrorIs(t, err, safepath.ErrPathEscapesWorkspace)
		})
	}
}

// TestRegression_Read_AllowsInWorkspace confirms a normal in-workspace read
// still works after the containment guard.
func TestRegression_Read_AllowsInWorkspace(
	t *testing.T,
) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "ok.txt"), []byte("hi"), 0o600))

	fc, err := content.Read(root, "ok.txt")
	require.NoError(t, err)
	assert.Equal(t, "hi", fc.Content)
}

// TestRegression_Write_RejectsPathEscape verifies content.Write refuses to
// write outside the workspace root (arbitrary host write / RCE vector).
func TestRegression_Write_RejectsPathEscape(
	t *testing.T,
) {
	root := t.TempDir()
	outside := t.TempDir()

	for _, tc := range escapeCases {
		t.Run(tc.name, func(t *testing.T) {
			err := content.Write(root, tc.path, "pwned")
			require.ErrorIs(t, err, safepath.ErrPathEscapesWorkspace)
		})
	}

	// Nothing was written outside the root.
	entries, err := os.ReadDir(outside)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// TestRegression_Write_RejectsSymlinkEscape verifies content.Write cannot
// follow a symlinked parent that points outside the workspace.
func TestRegression_Write_RejectsSymlinkEscape(
	t *testing.T,
) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	outside := t.TempDir()
	root := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "link")))

	err := content.Write(root, "link/escaped.txt", "pwned")
	require.ErrorIs(t, err, safepath.ErrPathEscapesWorkspace)

	_, statErr := os.Stat(filepath.Join(outside, "escaped.txt"))
	assert.True(t, os.IsNotExist(statErr), "write must not land outside the root via symlink")
}

// TestRegression_Write_AllowsInWorkspace confirms a normal in-workspace write
// still works after the containment guard.
func TestRegression_Write_AllowsInWorkspace(
	t *testing.T,
) {
	root := t.TempDir()
	require.NoError(t, content.Write(root, "sub/ok.txt", "data"))

	data, err := os.ReadFile(filepath.Join(root, "sub/ok.txt"))
	require.NoError(t, err)
	assert.Equal(t, "data", string(data))
}
