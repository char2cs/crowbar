package mutate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCopySymlink_ReadlinkError(
	t *testing.T,
) {
	dir := t.TempDir()
	notALink := filepath.Join(dir, "plain.txt")
	require.NoError(t, os.WriteFile(notALink, []byte("x"), 0o600))

	err := copySymlink(notALink, filepath.Join(dir, "dest"))
	require.Error(t, err, "Readlink on a non-symlink must fail, not silently create a bogus link")
}

func TestCopyDir_MkdirError(
	t *testing.T,
) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	require.NoError(t, os.MkdirAll(src, 0o700))
	dest := filepath.Join(dir, "dest")
	require.NoError(t, os.MkdirAll(dest, 0o700))

	err := copyDir(src, dest)
	require.Error(t, err, "mkdir over an already-occupied destination must fail")
}

func TestCopyDirEntry_InfoError(
	t *testing.T,
) {
	dir := t.TempDir()
	target := filepath.Join(dir, "gone.txt")
	require.NoError(t, os.WriteFile(target, []byte("x"), 0o600))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	entry := entries[0]

	require.NoError(t, os.Remove(target))

	copyErr := copyDirEntry(dir, filepath.Join(dir, "dest"), entry)
	require.Error(t, copyErr, "a file removed between ReadDir and Info() must surface as an error, not panic")
	assert.True(t, os.IsNotExist(copyErr))
}

func TestCopyFileBytes_IoCopyError(
	t *testing.T,
) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	require.NoError(t, os.MkdirAll(srcDir, 0o700))

	err := copyFileBytes(srcDir, filepath.Join(dir, "dest"))
	require.Error(t, err, "reading a directory as if it were a file's bytes must fail, not hang or panic")
}
