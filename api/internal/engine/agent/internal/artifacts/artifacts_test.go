package artifacts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agent/internal/artifacts"
)

func initRepo(
	t *testing.T,
) string {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}

	run("git", "init")
	run("git", "config", "user.email", "test@test.com")
	run("git", "config", "user.name", "Test")
	run("git", "checkout", "-b", "main")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("init"), 0o644))
	run("git", "add", ".")
	run("git", "commit", "-m", "init")

	run("git", "checkout", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("changed"), 0o644))
	run("git", "add", ".")
	run("git", "commit", "-m", "change")

	return dir
}

func TestWriteDiff_withChanges(
	t *testing.T,
) {
	dir := initRepo(t)
	dest := filepath.Join(t.TempDir(), "diff.patch")

	err := artifacts.WriteDiff(dir, "main", "feature", dest)
	require.NoError(t, err)

	data, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Contains(t, string(data), "changed")
}

func TestWriteDiff_emptyDiff(
	t *testing.T,
) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	run("git", "init")
	run("git", "config", "user.email", "test@test.com")
	run("git", "config", "user.name", "Test")
	run("git", "checkout", "-b", "main")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("init"), 0o644))
	run("git", "add", ".")
	run("git", "commit", "-m", "init")
	run("git", "checkout", "-b", "feature")

	dest := filepath.Join(t.TempDir(), "diff.patch")
	err := artifacts.WriteDiff(dir, "main", "feature", dest)
	require.NoError(t, err)

	data, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Empty(t, data)
}

func TestWriteDiff_badRepo(
	t *testing.T,
) {
	dir := t.TempDir()
	dest := filepath.Join(t.TempDir(), "diff.patch")
	err := artifacts.WriteDiff(dir, "main", "feature", dest)
	require.Error(t, err)
}

func TestWriteDiff_writeFileError(
	t *testing.T,
) {
	dir := initRepo(t)
	// destPath inside a non-existent subdirectory causes os.WriteFile to fail.
	dest := filepath.Join(t.TempDir(), "nonexistent", "diff.patch")
	err := artifacts.WriteDiff(dir, "main", "feature", dest)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "artifacts: write")
}
