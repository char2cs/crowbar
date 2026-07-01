package replace_test

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/search/internal/replace"
)

func writeFile(
	t *testing.T,
	dir string,
	name string,
	content string,
) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestApplyToFile_BasicReplace(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "file.go", "hello world\nhello again\n")

	re := regexp.MustCompile(`hello`)
	require.NoError(t, replace.ApplyToFile(path, re, "goodbye", false))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "goodbye world\ngoodbye again\n", string(data))
}

func TestApplyToFile_Locked(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "file.go", "hello\n")

	re := regexp.MustCompile(`hello`)
	err := replace.ApplyToFile(path, re, "goodbye", true)
	require.ErrorIs(t, err, replace.ErrLocked)

	// File must be untouched.
	data, err2 := os.ReadFile(path)
	require.NoError(t, err2)
	assert.Equal(t, "hello\n", string(data))
}

func TestApplyToFile_BackreferenceReplacement(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "file.go", "foo bar baz\n")

	re := regexp.MustCompile(`(foo) (bar)`)
	require.NoError(t, replace.ApplyToFile(path, re, "$2 $1", false))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "bar foo baz\n", string(data))
}

func TestApplyToFile_NoMatchIsNoOp(t *testing.T) {
	dir := t.TempDir()
	original := "no match here\n"
	path := writeFile(t, dir, "file.go", original)

	re := regexp.MustCompile(`xyz`)
	require.NoError(t, replace.ApplyToFile(path, re, "abc", false))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, original, string(data))
}

func TestApplyToFile_MissingFile(t *testing.T) {
	re := regexp.MustCompile(`hello`)
	err := replace.ApplyToFile("/nonexistent/path/file.go", re, "goodbye", false)
	require.Error(t, err)
}

func TestApplyToFile_IsAtomic(t *testing.T) {
	// Verify no temp files are left behind after a successful replace.
	dir := t.TempDir()
	path := writeFile(t, dir, "file.go", "hello\n")

	re := regexp.MustCompile(`hello`)
	require.NoError(t, replace.ApplyToFile(path, re, "goodbye", false))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), ".crowbar-replace-")
	}
}

func TestApplyToFile_BinarySkipped(t *testing.T) {
	dir := t.TempDir()
	// Binary content: null bytes guarantee match.IsBinary returns true.
	binPath := filepath.Join(dir, "binary.bin")
	require.NoError(t, os.WriteFile(binPath, []byte{0x00, 0x01, 0x02, 0x7f}, 0o600))

	original, err := os.ReadFile(binPath)
	require.NoError(t, err)

	re := regexp.MustCompile(`\x00`)
	require.NoError(t, replace.ApplyToFile(binPath, re, "X", false))

	// File must be untouched.
	got, err := os.ReadFile(binPath)
	require.NoError(t, err)
	assert.Equal(t, original, got)
}

func TestApplyToFile_PreservesPermissions(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "script.sh", "#!/bin/sh\nhello\n")

	require.NoError(t, os.Chmod(path, 0o755))

	re := regexp.MustCompile(`hello`)
	require.NoError(t, replace.ApplyToFile(path, re, "goodbye", false))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "#!/bin/sh\ngoodbye\n", string(data))
}

// TestApplyToFile_NoReadPermission covers the "open for sniff" error branch:
// os.Stat succeeds (it doesn't require read permission on the file itself),
// but the subsequent os.Open for the binary sniff fails because the file has
// no read permission.
func TestApplyToFile_NoReadPermission(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; cannot test permission denial")
	}
	dir := t.TempDir()
	path := writeFile(t, dir, "file.go", "hello\n")
	require.NoError(t, os.Chmod(path, 0o000))
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	re := regexp.MustCompile(`hello`)
	err := replace.ApplyToFile(path, re, "goodbye", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "open for sniff")
}

// TestApplyToFile_DirectoryAsPath covers the binary-sniff read error branch
// (readErr != io.EOF): os.Stat and os.Open both succeed against a directory,
// but reading from it fails with "is a directory".
func TestApplyToFile_DirectoryAsPath(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "subdir")
	require.NoError(t, os.Mkdir(sub, 0o755))

	re := regexp.MustCompile(`hello`)
	err := replace.ApplyToFile(sub, re, "goodbye", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sniff")
}

func TestApplyToFile_RenameFailsWhenDirReadOnly(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; cannot test permission denial")
	}
	dir := t.TempDir()
	path := writeFile(t, dir, "file.go", "hello\n")

	// Make the directory read-only so os.Rename fails.
	require.NoError(t, os.Chmod(dir, 0o555))
	defer func() { _ = os.Chmod(dir, 0o755) }()

	re := regexp.MustCompile(`hello`)
	err := replace.ApplyToFile(path, re, "goodbye", false)
	// Should fail because the temp file cannot be created (dir not writable).
	require.Error(t, err)
}
