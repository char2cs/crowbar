package fs_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/fs"
	"github.com/char2cs/crowbar/api/internal/engine/fs/internal/tree"
)

func initDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
}

// nullProvider satisfies tree.StatusProvider returning an empty GitStatus.
type nullProvider struct{}

func (nullProvider) GitStatus(_ string) (gitdomain.GitStatus, error) {
	return gitdomain.GitStatus{}, nil
}

func TestTree_ListsFiles(t *testing.T) {
	dir := initDir(t)
	writeFile(t, dir, "main.go", "package main\n")
	writeFile(t, dir, "README.md", "# Readme\n")

	nodes, err := fs.New().Tree(dir, "", nullProvider{})

	require.NoError(t, err)
	var names []string
	for _, n := range nodes {
		names = append(names, n.Name)
	}
	assert.Contains(t, names, "main.go")
	assert.Contains(t, names, "README.md")
}

func TestTree_NonexistentDir_ReturnsError(t *testing.T) {
	_, err := fs.New().Tree(filepath.Join(t.TempDir(), "nope"), "", nullProvider{})
	require.Error(t, err)
}

func TestReadContent_ReturnsContent(t *testing.T) {
	dir := initDir(t)
	writeFile(t, dir, "file.txt", "hello world\n")

	fc, err := fs.New().ReadContent(dir, "file.txt")

	require.NoError(t, err)
	assert.Equal(t, "hello world\n", fc.Content)
}

func TestReadContent_MissingFile_ReturnsError(t *testing.T) {
	_, err := fs.New().ReadContent(t.TempDir(), "missing.txt")
	require.Error(t, err)
}

func TestWriteContent_Persists(t *testing.T) {
	dir := initDir(t)
	writeFile(t, dir, "file.txt", "old\n")

	require.NoError(t, fs.New().WriteContent(dir, "file.txt", "new\n"))

	got, err := os.ReadFile(filepath.Join(dir, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "new\n", string(got))
}

func TestWriteContent_CreatesParentDirs(t *testing.T) {
	dir := initDir(t)
	// Write creates intermediate directories automatically.
	require.NoError(t, fs.New().WriteContent(dir, filepath.Join("sub", "dir", "file.txt"), "hello\n"))
	got, err := os.ReadFile(filepath.Join(dir, "sub", "dir", "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello\n", string(got))
}

func TestCreateFile_CreatesEmptyFile(t *testing.T) {
	dir := initDir(t)

	require.NoError(t, fs.New().CreateFile(dir, "new.txt"))

	info, err := os.Stat(filepath.Join(dir, "new.txt"))
	require.NoError(t, err)
	assert.Equal(t, int64(0), info.Size())
}

func TestCreateFile_AlreadyExists_ReturnsError(t *testing.T) {
	dir := initDir(t)
	writeFile(t, dir, "exists.txt", "content\n")

	err := fs.New().CreateFile(dir, "exists.txt")
	require.Error(t, err)
}

func TestCreateDir_CreatesDirectory(t *testing.T) {
	dir := initDir(t)

	require.NoError(t, fs.New().CreateDir(dir, "subdir"))

	info, err := os.Stat(filepath.Join(dir, "subdir"))
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestCreateDir_AlreadyExists_IsIdempotent(t *testing.T) {
	dir := initDir(t)
	require.NoError(t, os.Mkdir(filepath.Join(dir, "existing"), 0o755))

	// CreateDir uses MkdirAll — existing directory is not an error.
	require.NoError(t, fs.New().CreateDir(dir, "existing"))
}

func TestRename_MovesFile(t *testing.T) {
	dir := initDir(t)
	writeFile(t, dir, "old.txt", "data\n")

	require.NoError(t, fs.New().Rename(dir, "old.txt", "new.txt"))

	_, err := os.Stat(filepath.Join(dir, "old.txt"))
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(dir, "new.txt"))
	require.NoError(t, err)
}

func TestRename_MissingSource_ReturnsError(t *testing.T) {
	err := fs.New().Rename(t.TempDir(), "ghost.txt", "dest.txt")
	require.Error(t, err)
}

func TestDelete_RemovesFile(t *testing.T) {
	dir := initDir(t)
	writeFile(t, dir, "del.txt", "bye\n")

	require.NoError(t, fs.New().Delete(dir, "del.txt"))

	_, err := os.Stat(filepath.Join(dir, "del.txt"))
	assert.True(t, os.IsNotExist(err))
}

func TestDelete_MissingFile_IsIdempotent(t *testing.T) {
	// Delete uses RemoveAll — missing path is not an error.
	require.NoError(t, fs.New().Delete(t.TempDir(), "ghost.txt"))
}

func TestNewWatcher_ReturnsNonNil(t *testing.T) {
	dir := initDir(t)
	w := fs.New().NewWatcher(t.Context(), "ws-1", dir, "", nil, nil)
	require.NotNil(t, w)
}

var _ tree.StatusProvider = nullProvider{} //nolint:unused
