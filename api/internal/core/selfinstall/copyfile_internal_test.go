package selfinstall

// White-box tests for the unexported copyFile helper: Install always copies
// the running test binary (which always exists), so these branches can't be
// reached through the public Install API and need direct, same-package tests.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCopyFile_OpenSrcFails(t *testing.T) {
	dir := t.TempDir()
	err := copyFile(filepath.Join(dir, "does-not-exist"), filepath.Join(dir, "dst"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "open src")
}

func TestCopyFile_CreateDstFails(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	require.NoError(t, os.WriteFile(src, []byte("hi"), 0o644))

	dst := filepath.Join(dir, "missing-parent", "dst") // parent dir doesn't exist
	err := copyFile(src, dst)
	require.Error(t, err)
	require.Contains(t, err.Error(), "create")
}

func TestCopyFile_CopyFails_SrcIsDir(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "adir")
	require.NoError(t, os.Mkdir(srcDir, 0o755))

	// Opening a directory succeeds but reading it as a file fails, so this
	// exercises io.Copy's error path rather than os.Open's.
	dst := filepath.Join(dir, "dst")
	err := copyFile(srcDir, dst)
	require.Error(t, err)
	require.Contains(t, err.Error(), "copy")
}
