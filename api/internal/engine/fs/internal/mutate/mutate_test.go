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
	"github.com/char2cs/crowbar/api/internal/testutil"
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

func TestCreateFile_MkdirError(
	t *testing.T,
) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod restrictions differ on windows")
	}
	dir := t.TempDir()
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

func TestRename_SourceMissing(
	t *testing.T,
) {
	dir := t.TempDir()

	err := mutate.Rename(dir, "nonexistent.txt", "dest.txt")
	require.Error(t, err)
}

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
	src, srcErr := os.ReadFile(filepath.Join(dir, "src.txt"))
	require.NoError(t, srcErr)
	assert.Equal(t, []byte("SOURCE"), src)
}

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

func TestRegression_Copy_CleansUpPartialDestOnFailure(
	t *testing.T,
) {
	testutil.RequirePermissionEnforcement(t)
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

func TestCreateDir_MkdirError(
	t *testing.T,
) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod restrictions differ on windows")
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "blocker"), []byte("x"), 0o600))

	err := mutate.CreateDir(dir, "blocker/child")
	require.Error(t, err)
}

func TestRename_MkdirError(
	t *testing.T,
) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod restrictions differ on windows")
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("data"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "blocker"), []byte("x"), 0o600))

	err := mutate.Rename(dir, "src.txt", "blocker/subdir/dst.txt")
	require.Error(t, err)
}

var escapeCases = []struct {
	name string
	path string
}{
	{"parent traversal", "../../etc/passwd"},
	{"absolute path", "/etc/passwd"},
	{"mid-path escape", "a/../../b"},
}

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

func TestRegression_CreateFile_AllowsInWorkspace(
	t *testing.T,
) {
	root := t.TempDir()
	require.NoError(t, mutate.CreateFile(root, "sub/new.txt"))
	_, err := os.Stat(filepath.Join(root, "sub/new.txt"))
	require.NoError(t, err)
}

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

func TestRegression_CreateDir_AllowsInWorkspace(
	t *testing.T,
) {
	root := t.TempDir()
	require.NoError(t, mutate.CreateDir(root, "a/b/c"))
	info, err := os.Stat(filepath.Join(root, "a/b/c"))
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

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

	_, err := os.Stat(filepath.Join(root, "src.txt"))
	require.NoError(t, err)
}

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

	_, err := os.Stat(victim)
	require.NoError(t, err)
}

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

func TestRegression_Delete_AllowsInWorkspace(
	t *testing.T,
) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "del.txt"), nil, 0o600))

	require.NoError(t, mutate.Delete(root, "del.txt"))

	_, err := os.Stat(filepath.Join(root, "del.txt"))
	assert.True(t, os.IsNotExist(err))
}

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

func TestCopy_DestinationInsideSource(
	t *testing.T,
) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o700))

	err := mutate.Copy(dir, "src", "src/inner")
	require.ErrorIs(t, err, fs.ErrInvalid)
}

func TestCopy_SourceMissing(
	t *testing.T,
) {
	dir := t.TempDir()
	err := mutate.Copy(dir, "nope.txt", "nope copy.txt")
	require.Error(t, err)
	assert.True(t, os.IsNotExist(err) || errors.Is(err, fs.ErrNotExist))
}

func TestCopy_DestMkdirError(
	t *testing.T,
) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod restrictions differ on windows")
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "blocker"), []byte("x"), 0o600))

	err := mutate.Copy(dir, "a.txt", "blocker/sub/copy.txt")
	require.Error(t, err)
}

func TestCopy_Directory_ReadDirError(
	t *testing.T,
) {
	testutil.RequirePermissionEnforcement(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	require.NoError(t, os.MkdirAll(src, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(src, "f.txt"), []byte("x"), 0o600))
	require.NoError(t, os.Chmod(src, 0o000))
	t.Cleanup(func() { _ = os.Chmod(src, 0o700) })

	err := mutate.Copy(dir, "src", "dst")
	require.Error(t, err, "an unreadable source directory must fail the copy, not silently produce an empty one")
}

func TestCopy_File_DestOpenError(
	t *testing.T,
) {
	testutil.RequirePermissionEnforcement(t)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o600))
	destDir := filepath.Join(dir, "readonly")
	require.NoError(t, os.MkdirAll(destDir, 0o700))
	require.NoError(t, os.Chmod(destDir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(destDir, 0o700) })

	err := mutate.Copy(dir, "a.txt", "readonly/copy.txt")
	require.Error(t, err)

	_, statErr := os.Stat(filepath.Join(destDir, "copy.txt"))
	assert.True(t, os.IsNotExist(statErr), "no partial file may be left in the unwritable destination")
}

func TestCopy_Directory_DestParentNotWritable(
	t *testing.T,
) {
	testutil.RequirePermissionEnforcement(t)
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src/f.txt"), []byte("x"), 0o600))
	destDir := filepath.Join(dir, "readonly")
	require.NoError(t, os.MkdirAll(destDir, 0o700))
	require.NoError(t, os.Chmod(destDir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(destDir, 0o700) })

	err := mutate.Copy(dir, "src", "readonly/dst")
	require.Error(t, err, "Mkdir inside a non-writable directory must fail, not silently skip the copy")

	_, statErr := os.Stat(filepath.Join(destDir, "dst"))
	assert.True(t, os.IsNotExist(statErr), "no partial directory may be left in the unwritable destination")
}

func TestDelete_PermissionError(
	t *testing.T,
) {
	testutil.RequirePermissionEnforcement(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "locked")
	require.NoError(t, os.MkdirAll(target, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(target, "child.txt"), []byte("x"), 0o600))
	require.NoError(t, os.Chmod(target, 0o500))
	t.Cleanup(func() { _ = os.Chmod(target, 0o700) })

	err := mutate.Delete(dir, "locked")
	require.Error(t, err, "a locked directory's contents cannot be removed and must surface an error")
}

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
