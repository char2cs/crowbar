package mutate

// White-box tests for copyNode's unexported helpers. Copy's own preconditions
// (Lstat confirms a symlink before calling copySymlink, destFull is confirmed
// absent before copyDir/copyFileBytes run) make several of their internal
// error branches unreachable through the public Copy API without a genuine
// TOCTOU race. Calling the helpers directly, same-package, sidesteps those
// preconditions and exercises the real error-handling code deterministically.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCopySymlink_ReadlinkError covers the os.Readlink error branch: Copy's own
// Lstat check always confirms srcFull is a symlink before copySymlink ever
// runs, so a Readlink failure there would need the link to vanish in the
// instant between that check and this call — a race, not a reachable state.
// Calling copySymlink directly on a path that was NEVER a symlink triggers the
// same failure deterministically.
func TestCopySymlink_ReadlinkError(
	t *testing.T,
) {
	dir := t.TempDir()
	notALink := filepath.Join(dir, "plain.txt")
	require.NoError(t, os.WriteFile(notALink, []byte("x"), 0o600))

	err := copySymlink(notALink, filepath.Join(dir, "dest"))
	require.Error(t, err, "Readlink on a non-symlink must fail, not silently create a bogus link")
}

// TestCopyDir_MkdirError covers copyDir's os.Mkdir error branch: every real
// call site reaches copyDir with destFull already confirmed absent (Copy's own
// Lstat guard, or — recursively — the parent's freshly created directory), so
// os.Mkdir there always succeeds today. Calling copyDir directly with an
// ALREADY-EXISTING destFull exercises the same "mkdir over an occupied path"
// failure without needing a race to occupy it mid-copy.
func TestCopyDir_MkdirError(
	t *testing.T,
) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	require.NoError(t, os.MkdirAll(src, 0o700))
	dest := filepath.Join(dir, "dest")
	require.NoError(t, os.MkdirAll(dest, 0o700)) // already occupied

	err := copyDir(src, dest)
	require.Error(t, err, "mkdir over an already-occupied destination must fail")
}

// TestCopyDirEntry_InfoError covers copyDirEntry's entry.Info() error branch:
// os.DirEntry.Info() performs a fresh Lstat at call time (confirmed on this
// platform — it is not served from a ReadDir-time cache), so a file removed
// after ReadDir returns its entry but before Info() is called surfaces a real
// ErrNotExist, exactly as entry.Info()'s doc comment describes. No background
// goroutine or timing window is needed: everything here runs in one goroutine,
// strictly sequenced.
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

// TestCopyFileBytes_IoCopyError covers io.Copy's error branch inside
// copyFileBytes: every real call site reaches it only after copyNode's
// info.IsDir() check has already routed a directory to copyDir instead, so
// io.Copy(dest, src) reading from src never sees anything but a regular file's
// bytes through the public Copy API — an actual mid-copy write failure would
// need a real disk-full/quota condition, not reproducible in a portable test.
// Calling copyFileBytes directly with a DIRECTORY as the source sidesteps that
// dispatch: os.Open succeeds on a directory (as it does on Unix), so the
// failure genuinely comes from io.Copy's own Read on it, not from Open.
func TestCopyFileBytes_IoCopyError(
	t *testing.T,
) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	require.NoError(t, os.MkdirAll(srcDir, 0o700))

	err := copyFileBytes(srcDir, filepath.Join(dir, "dest"))
	require.Error(t, err, "reading a directory as if it were a file's bytes must fail, not hang or panic")
}
