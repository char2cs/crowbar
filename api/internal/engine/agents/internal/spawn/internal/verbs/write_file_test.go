package verbs

// White-box tests for write_file.go's unexported helpers. spawn_test.go (one
// package up) already drives writeFile end to end for the mkdir-fails,
// content-write, missing-"from", and successful-copy branches; the ones here
// are the paths that public entry point never reaches: a "~/" source and
// copyFile/closeBoth's own error branches.

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandHome_ExpandsATildeSlashPrefixAgainstTheRealHomeDir(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	got := expandHome("~/configs/settings.json")

	assert.Equal(t, filepath.Join(home, "configs/settings.json"), got)
}

func TestCopyFile_OpenSrcFails(t *testing.T) {
	dir := t.TempDir()

	err := copyFile(filepath.Join(dir, "does-not-exist"), filepath.Join(dir, "dst"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "copy open")
}

func TestCopyFile_CreateDstFails(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	require.NoError(t, os.WriteFile(src, []byte("hi"), 0o600))

	err := copyFile(src, filepath.Join(dir, "missing-parent", "dst"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "copy create")
}

// A directory opens without error but fails to read as a byte stream, so this
// exercises io.Copy's error path rather than os.Open's — the same technique
// selfinstall's own copyFile tests use for its analogous helper.
func TestCopyFile_CopyFailsWhenSourceIsADirectory(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "adir")
	require.NoError(t, os.Mkdir(srcDir, 0o755))

	err := copyFile(srcDir, filepath.Join(dir, "dst"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "agents: copy:")
}

func TestCloseBoth_ThePrimaryErrorAlwaysWinsOverACloseFailure(t *testing.T) {
	primary := errors.New("write failed")

	got := closeBoth(primary, errors.New("close also failed"))

	assert.Equal(t, primary, got, "a close failure must never mask the real error")
}

func TestCloseBoth_ACloseOnlyFailureStillSurfaces(t *testing.T) {
	err := closeBoth(nil, errors.New("disk full on close"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "disk full on close")
}

func TestCloseBoth_NoErrorsIsNoError(t *testing.T) {
	assert.NoError(t, closeBoth(nil, nil))
}
