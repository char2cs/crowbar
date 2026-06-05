package mutate_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/fs/internal/mutate"
)

func TestCreateFile_CreatesEmptyFile(
	t *testing.T,
) {
	dir := t.TempDir()
	err := mutate.CreateFile(dir, "newfile.txt")
	require.NoError(t, err)

	info, err := os.Stat(filepath.Join(dir, "newfile.txt"))
	require.NoError(t, err)
	assert.False(t, info.IsDir())
	assert.Equal(t, int64(0), info.Size())
}

func TestCreateFile_CreatesParentDirs(
	t *testing.T,
) {
	dir := t.TempDir()
	err := mutate.CreateFile(dir, "a/b/file.txt")
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, "a/b/file.txt"))
	require.NoError(t, err)
}

func TestCreateFile_ErrorsOnExisting(
	t *testing.T,
) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "exists.txt"), nil, 0600))
	err := mutate.CreateFile(dir, "exists.txt")
	require.Error(t, err)
}

// TestCreateFile_MkdirError exercises the os.MkdirAll error path by placing
// a regular file where a directory is expected.
func TestCreateFile_MkdirError(
	t *testing.T,
) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod restrictions differ on windows")
	}
	dir := t.TempDir()
	// Create a regular file at "parent" so MkdirAll cannot create a dir there.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "parent"), []byte("x"), 0600))

	err := mutate.CreateFile(dir, "parent/child.txt")
	require.Error(t, err)
}

func TestCreateDir(
	t *testing.T,
) {
	dir := t.TempDir()
	err := mutate.CreateDir(dir, "subdir/nested")
	require.NoError(t, err)
	info, err := os.Stat(filepath.Join(dir, "subdir/nested"))
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

// TestCreateDir_AlreadyExists verifies CreateDir is idempotent.
func TestCreateDir_AlreadyExists(
	t *testing.T,
) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "existing"), 0700))

	err := mutate.CreateDir(dir, "existing")
	require.NoError(t, err)
}

func TestRename_File(
	t *testing.T,
) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "old.txt"), []byte("data"), 0600))

	err := mutate.Rename(dir, "old.txt", "new.txt")
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dir, "old.txt"))
	assert.True(t, os.IsNotExist(err))

	data, err := os.ReadFile(filepath.Join(dir, "new.txt"))
	require.NoError(t, err)
	assert.Equal(t, "data", string(data))
}

func TestRename_MovesToSubdir(
	t *testing.T,
) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("x"), 0600))

	err := mutate.Rename(dir, "file.txt", "subdir/file.txt")
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "subdir/file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "x", string(data))
}

// TestRename_SourceMissing exercises the os.Rename error path when source
// does not exist.
func TestRename_SourceMissing(
	t *testing.T,
) {
	dir := t.TempDir()

	err := mutate.Rename(dir, "nonexistent.txt", "dest.txt")
	require.Error(t, err)
}

// TestRename_CrossDir renames a file to a different subdirectory (parent dirs
// created automatically).
func TestRename_CrossDir(
	t *testing.T,
) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src/file.txt"), []byte("hello"), 0600))

	err := mutate.Rename(dir, "src/file.txt", "dst/deep/file.txt")
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "dst/deep/file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello", string(data))
}

func TestDelete_File(
	t *testing.T,
) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "del.txt"), nil, 0600))

	err := mutate.Delete(dir, "del.txt")
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dir, "del.txt"))
	assert.True(t, os.IsNotExist(err))
}

func TestDelete_Dir(
	t *testing.T,
) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub/nested"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub/nested/f.txt"), nil, 0600))

	err := mutate.Delete(dir, "sub")
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dir, "sub"))
	assert.True(t, os.IsNotExist(err))
}

func TestDelete_NonExistent_NoError(
	t *testing.T,
) {
	dir := t.TempDir()
	err := mutate.Delete(dir, "nonexistent.txt")
	require.NoError(t, err)
}

// TestCreateDir_MkdirError exercises the os.MkdirAll error path in CreateDir
// by placing a regular file where a directory path is needed.
func TestCreateDir_MkdirError(
	t *testing.T,
) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod restrictions differ on windows")
	}
	dir := t.TempDir()
	// Create a regular file at "blocker"; MkdirAll can't use it as a dir.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "blocker"), []byte("x"), 0600))

	err := mutate.CreateDir(dir, "blocker/child")
	require.Error(t, err)
}

// TestRename_MkdirError exercises the os.MkdirAll error path in Rename by
// placing a regular file where the destination parent directory must be.
func TestRename_MkdirError(
	t *testing.T,
) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod restrictions differ on windows")
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("data"), 0600))
	// "blocker" is a regular file so MkdirAll("blocker/subdir") fails.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "blocker"), []byte("x"), 0600))

	err := mutate.Rename(dir, "src.txt", "blocker/subdir/dst.txt")
	require.Error(t, err)
}
