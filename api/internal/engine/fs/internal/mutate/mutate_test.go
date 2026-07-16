package mutate_test

import (
	"errors"
	"io/fs"
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

// TestRegression_Rename_RefusesToClobberExistingDest pins the silent-data-loss
// fix: os.Rename replaces an occupied destination file with no error, so a rename
// or drag-drop move onto an existing name destroyed the occupant. Rename now
// refuses with fs.ErrExist (mapped to 409), and the occupant survives intact.
func TestRegression_Rename_RefusesToClobberExistingDest(
	t *testing.T,
) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("SOURCE"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dst.txt"), []byte("PRECIOUS"), 0o600))

	err := mutate.Rename(dir, "src.txt", "dst.txt")
	require.ErrorIs(t, err, fs.ErrExist)

	kept, readErr := os.ReadFile(filepath.Join(dir, "dst.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, []byte("PRECIOUS"), kept, "existing destination must be untouched")
	// Source also survives — the rename never happened.
	src, srcErr := os.ReadFile(filepath.Join(dir, "src.txt"))
	require.NoError(t, srcErr)
	assert.Equal(t, []byte("SOURCE"), src)
}

// TestRegression_Rename_RefusesToClobberExistingDir verifies the collision guard
// covers a directory destination too (os.Rename silently replaces an empty one).
func TestRegression_Rename_RefusesToClobberExistingDir(
	t *testing.T,
) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "from"), 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "to"), 0o700))

	err := mutate.Rename(dir, "from", "to")
	require.ErrorIs(t, err, fs.ErrExist)

	_, statErr := os.Stat(filepath.Join(dir, "from"))
	require.NoError(t, statErr, "source directory must survive a refused clobber")
}

// TestRename_CaseOnlyRenameAllowed verifies the same-file exception: renaming a
// file to a case variant of its own name (Foo → foo) is a legitimate rename, not
// a clobber, and must not be rejected as a collision.
func TestRename_CaseOnlyRenameAllowed(
	t *testing.T,
) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Foo.txt"), []byte("data"), 0o600))

	err := mutate.Rename(dir, "Foo.txt", "foo.txt")
	require.NoError(t, err, "case-only rename must be allowed")

	data, readErr := os.ReadFile(filepath.Join(dir, "foo.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, []byte("data"), data)
}

// TestRegression_Copy_CleansUpPartialDestOnFailure verifies a recursive copy that
// fails partway removes its half-populated destination rather than leaving cruft
// that would both mislead the tree and make a retry fail on destination-exists.
// An unreadable leaf (chmod 000) forces the mid-copy failure.
func TestRegression_Copy_CleansUpPartialDestOnFailure(
	t *testing.T,
) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permission semantics")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses the unreadable-file permission, so no failure is provoked")
	}
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src/ok.txt"), []byte("ok"), 0o600))
	unreadable := filepath.Join(dir, "src/secret.bin")
	require.NoError(t, os.WriteFile(unreadable, []byte("no read"), 0o600))
	require.NoError(t, os.Chmod(unreadable, 0o000))
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o600) })

	err := mutate.Copy(dir, "src", "dst")
	require.Error(t, err, "copy of a tree with an unreadable leaf must fail")

	_, statErr := os.Stat(filepath.Join(dir, "dst"))
	assert.True(t, os.IsNotExist(statErr), "the partial destination must be removed on failure")
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

// TestCopy_File_BinaryBytesFaithful proves the copy is a byte copy, not a text
// round-trip: bytes that are invalid UTF-8 (and contain NULs) must come out
// identical. This is the case the base64 content read corrupts.
func TestCopy_File_BinaryBytesFaithful(
	t *testing.T,
) {
	dir := t.TempDir()
	binary := []byte{0x89, 0x50, 0x4E, 0x47, 0x00, 0xFF, 0xFE, 0x00, 0x01, 0x02}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "img.png"), binary, 0o600))

	require.NoError(t, mutate.Copy(dir, "img.png", "img copy.png"))

	got, err := os.ReadFile(filepath.Join(dir, "img copy.png"))
	require.NoError(t, err)
	assert.Equal(t, binary, got)
}

// TestCopy_Directory_Recursive copies a nested tree, including a binary leaf,
// and creates missing parents of the destination.
func TestCopy_Directory_Recursive(
	t *testing.T,
) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src/nested"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src/a.txt"), []byte("text"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src/nested/b.bin"), []byte{0x00, 0x01}, 0o600))

	require.NoError(t, mutate.Copy(dir, "src", "sub/src copy"))

	text, err := os.ReadFile(filepath.Join(dir, "sub/src copy/a.txt"))
	require.NoError(t, err)
	assert.Equal(t, []byte("text"), text)
	bin, err := os.ReadFile(filepath.Join(dir, "sub/src copy/nested/b.bin"))
	require.NoError(t, err)
	assert.Equal(t, []byte{0x00, 0x01}, bin)
}

// TestCopy_Symlink recreates the link itself rather than following it.
func TestCopy_Symlink(
	t *testing.T,
) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "target.txt"), []byte("t"), 0o600))
	require.NoError(t, os.Symlink("target.txt", filepath.Join(dir, "link")))

	require.NoError(t, mutate.Copy(dir, "link", "link copy"))

	got, err := os.Readlink(filepath.Join(dir, "link copy"))
	require.NoError(t, err)
	assert.Equal(t, "target.txt", got)
}

// TestCopy_DestinationExists refuses to clobber an existing destination with
// the fs.ErrExist sentinel (mapped to 409 by the API layer).
func TestCopy_DestinationExists(
	t *testing.T,
) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("keep"), 0o600))

	err := mutate.Copy(dir, "a.txt", "b.txt")
	require.ErrorIs(t, err, fs.ErrExist)

	kept, readErr := os.ReadFile(filepath.Join(dir, "b.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, []byte("keep"), kept, "existing destination must be untouched")
}

// TestCopy_DestinationInsideSource rejects the self-recursive copy with the
// fs.ErrInvalid sentinel (mapped to 400 by the API layer).
func TestCopy_DestinationInsideSource(
	t *testing.T,
) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o700))

	err := mutate.Copy(dir, "src", "src/inner")
	require.ErrorIs(t, err, fs.ErrInvalid)
}

// TestCopy_SourceMissing surfaces the Lstat not-found error.
func TestCopy_SourceMissing(
	t *testing.T,
) {
	dir := t.TempDir()
	err := mutate.Copy(dir, "nope.txt", "nope copy.txt")
	require.Error(t, err)
	assert.True(t, os.IsNotExist(err) || errors.Is(err, fs.ErrNotExist))
}

// TestCopy_TraversalRejected keeps both operands inside the workspace.
func TestCopy_TraversalRejected(
	t *testing.T,
) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o600))

	err := mutate.Copy(dir, "a.txt", "../escape.txt")
	require.ErrorIs(t, err, safepath.ErrPathEscapesWorkspace)

	err = mutate.Copy(dir, "../etc/passwd", "stolen.txt")
	require.ErrorIs(t, err, safepath.ErrPathEscapesWorkspace)
}
