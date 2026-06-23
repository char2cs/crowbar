package mutate_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/fs/internal/mutate"
	"github.com/char2cs/crowbar/api/internal/engine/fs/safepath"
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
	require.NoError(t, os.WriteFile(filepath.Join(dir, "exists.txt"), nil, 0o600))
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
	require.NoError(t, os.WriteFile(filepath.Join(dir, "parent"), []byte("x"), 0o600))

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
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "existing"), 0o700))

	err := mutate.CreateDir(dir, "existing")
	require.NoError(t, err)
}

func TestRename_File(
	t *testing.T,
) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "old.txt"), []byte("data"), 0o600))

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
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("x"), 0o600))

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
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src/file.txt"), []byte("hello"), 0o600))

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
	require.NoError(t, os.WriteFile(filepath.Join(dir, "del.txt"), nil, 0o600))

	err := mutate.Delete(dir, "del.txt")
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dir, "del.txt"))
	assert.True(t, os.IsNotExist(err))
}

func TestDelete_Dir(
	t *testing.T,
) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub/nested"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub/nested/f.txt"), nil, 0o600))

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
	require.NoError(t, os.WriteFile(filepath.Join(dir, "blocker"), []byte("x"), 0o600))

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
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("data"), 0o600))
	// "blocker" is a regular file so MkdirAll("blocker/subdir") fails.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "blocker"), []byte("x"), 0o600))

	err := mutate.Rename(dir, "src.txt", "blocker/subdir/dst.txt")
	require.Error(t, err)
}

// escapeCases are the adversarial workspace-relative paths every fs mutation
// must reject (security finding R1).
var escapeCases = []struct {
	name string
	path string
}{
	{"parent traversal", "../../etc/passwd"},
	{"absolute path", "/etc/passwd"},
	{"mid-path escape", "a/../../b"},
}

// TestRegression_CreateFile_RejectsPathEscape verifies CreateFile cannot create
// a file outside the workspace root. Writing e.g. ../.git/hooks/pre-commit is
// the RCE vector this guards.
func TestRegression_CreateFile_RejectsPathEscape(
	t *testing.T,
) {
	root := t.TempDir()
	outside := t.TempDir()

	for _, tc := range escapeCases {
		t.Run(tc.name, func(t *testing.T) {
			err := mutate.CreateFile(root, tc.path)
			require.ErrorIs(t, err, safepath.ErrPathEscapesWorkspace)
		})
	}

	entries, err := os.ReadDir(outside)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// TestRegression_CreateFile_AllowsInWorkspace confirms normal creation still
// works.
func TestRegression_CreateFile_AllowsInWorkspace(
	t *testing.T,
) {
	root := t.TempDir()
	require.NoError(t, mutate.CreateFile(root, "sub/new.txt"))
	_, err := os.Stat(filepath.Join(root, "sub/new.txt"))
	require.NoError(t, err)
}

// TestRegression_CreateDir_RejectsPathEscape verifies CreateDir cannot create a
// directory outside the workspace root.
func TestRegression_CreateDir_RejectsPathEscape(
	t *testing.T,
) {
	root := t.TempDir()
	for _, tc := range escapeCases {
		t.Run(tc.name, func(t *testing.T) {
			err := mutate.CreateDir(root, tc.path)
			require.ErrorIs(t, err, safepath.ErrPathEscapesWorkspace)
		})
	}
}

// TestRegression_CreateDir_AllowsInWorkspace confirms normal creation works.
func TestRegression_CreateDir_AllowsInWorkspace(
	t *testing.T,
) {
	root := t.TempDir()
	require.NoError(t, mutate.CreateDir(root, "a/b/c"))
	info, err := os.Stat(filepath.Join(root, "a/b/c"))
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

// TestRegression_Rename_RejectsOldPathEscape verifies Rename validates the
// SOURCE path: an escaping oldPath cannot move host files into the workspace.
func TestRegression_Rename_RejectsOldPathEscape(
	t *testing.T,
) {
	root := t.TempDir()
	for _, tc := range escapeCases {
		t.Run(tc.name, func(t *testing.T) {
			err := mutate.Rename(root, tc.path, "dest.txt")
			require.ErrorIs(t, err, safepath.ErrPathEscapesWorkspace)
		})
	}
}

// TestRegression_Rename_RejectsNewPathEscape verifies Rename validates the
// DESTINATION path: an escaping newPath cannot move workspace files out of the
// workspace.
func TestRegression_Rename_RejectsNewPathEscape(
	t *testing.T,
) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "src.txt"), []byte("x"), 0o600))

	for _, tc := range escapeCases {
		t.Run(tc.name, func(t *testing.T) {
			err := mutate.Rename(root, "src.txt", tc.path)
			require.ErrorIs(t, err, safepath.ErrPathEscapesWorkspace)
		})
	}

	// Source untouched: the rename never happened.
	_, err := os.Stat(filepath.Join(root, "src.txt"))
	require.NoError(t, err)
}

// TestRegression_Rename_AllowsInWorkspace confirms a normal in-workspace rename
// still works.
func TestRegression_Rename_AllowsInWorkspace(
	t *testing.T,
) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "old.txt"), []byte("x"), 0o600))

	require.NoError(t, mutate.Rename(root, "old.txt", "dst/new.txt"))

	data, err := os.ReadFile(filepath.Join(root, "dst/new.txt"))
	require.NoError(t, err)
	assert.Equal(t, "x", string(data))
}

// TestRegression_Delete_RejectsPathEscape verifies Delete cannot remove files
// outside the workspace root (arbitrary host delete vector).
func TestRegression_Delete_RejectsPathEscape(
	t *testing.T,
) {
	root := t.TempDir()
	outside := t.TempDir()
	victim := filepath.Join(outside, "important.txt")
	require.NoError(t, os.WriteFile(victim, []byte("keep me"), 0o600))

	for _, tc := range escapeCases {
		t.Run(tc.name, func(t *testing.T) {
			err := mutate.Delete(root, tc.path)
			require.ErrorIs(t, err, safepath.ErrPathEscapesWorkspace)
		})
	}

	// The outside file survived.
	_, err := os.Stat(victim)
	require.NoError(t, err)
}

// TestRegression_Delete_RejectsSymlinkEscape verifies Delete cannot traverse a
// symlinked parent to remove content outside the workspace.
func TestRegression_Delete_RejectsSymlinkEscape(
	t *testing.T,
) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "victim.txt"), []byte("keep"), 0o600))
	root := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "link")))

	err := mutate.Delete(root, "link/victim.txt")
	require.ErrorIs(t, err, safepath.ErrPathEscapesWorkspace)

	_, statErr := os.Stat(filepath.Join(outside, "victim.txt"))
	require.NoError(t, statErr, "victim outside the root must survive")
}

// TestRegression_Delete_AllowsInWorkspace confirms a normal in-workspace delete
// still works.
func TestRegression_Delete_AllowsInWorkspace(
	t *testing.T,
) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "del.txt"), nil, 0o600))

	require.NoError(t, mutate.Delete(root, "del.txt"))

	_, err := os.Stat(filepath.Join(root, "del.txt"))
	assert.True(t, os.IsNotExist(err))
}
