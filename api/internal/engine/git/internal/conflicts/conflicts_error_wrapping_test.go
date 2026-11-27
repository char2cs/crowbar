package conflicts_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/conflicts"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
}

func newBareRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitIn(t, dir, "init", "-b", "main")
	gitIn(t, dir, "config", "user.email", "test@example.com")
	gitIn(t, dir, "config", "user.name", "Test")
	return dir
}

func TestParseFile_UnclosedConflictMarker_WrapsExtractError(t *testing.T) {
	dir := newBareRepo(t)
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "broken.txt"),
		[]byte("line one\n<<<<<<< HEAD\nours\n=======\ntheirs\n"),
		0o644,
	))

	_, err := conflicts.ParseFile(context.Background(), dir, "broken.txt")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflicts: parse file:")
	assert.Contains(t, err.Error(), "unclosed conflict marker")
}

func TestResolveHunk_UnclosedConflictMarker_WrapsExtractError(t *testing.T) {
	dir := newBareRepo(t)
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "broken.txt"),
		[]byte("line one\n<<<<<<< HEAD\nours\n=======\ntheirs\n"),
		0o644,
	))

	err := conflicts.ResolveHunk(
		context.Background(), dir, "broken.txt", "any-id",
		gitdomain.ConflictResolutionOurs, "",
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflicts: resolve hunk: parse:")
	assert.Contains(t, err.Error(), "unclosed conflict marker")
}

func TestResolveHunk_UnresolvedResolution_ReturnsError(t *testing.T) {
	dir := newBareRepo(t)
	conflictFile := "hello.txt"
	absPath := filepath.Join(dir, conflictFile)
	require.NoError(t, os.WriteFile(absPath, []byte("line one\nshared\nline three\n"), 0o644))
	gitIn(t, dir, "add", conflictFile)
	gitIn(t, dir, "commit", "-m", "initial")
	gitIn(t, dir, "checkout", "-b", "branch-a")
	require.NoError(t, os.WriteFile(absPath, []byte("line one\nours change\nline three\n"), 0o644))
	gitIn(t, dir, "add", conflictFile)
	gitIn(t, dir, "commit", "-m", "a")
	gitIn(t, dir, "checkout", "main")
	require.NoError(t, os.WriteFile(absPath, []byte("line one\ntheirs change\nline three\n"), 0o644))
	gitIn(t, dir, "add", conflictFile)
	gitIn(t, dir, "commit", "-m", "b")
	mergeCmd := exec.Command("git", "merge", "branch-a")
	mergeCmd.Dir = dir
	_ = mergeCmd.Run()

	hunks, err := conflicts.ParseFile(context.Background(), dir, conflictFile)
	require.NoError(t, err)
	require.Len(t, hunks, 1)

	err = conflicts.ResolveHunk(
		context.Background(), dir, conflictFile, hunks[0].ID,
		gitdomain.ConflictResolutionUnresolved, "",
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflicts: resolve hunk:")
	assert.Contains(t, err.Error(), "unknown resolution")

	data, readErr := os.ReadFile(absPath)
	require.NoError(t, readErr)
	assert.Contains(t, string(data), "<<<<<<<", "a rejected resolution must not touch the file")
}

func TestResolveHunk_PathIsDirectory_ReadFileError(t *testing.T) {
	dir := newBareRepo(t)
	require.NoError(t, os.Mkdir(filepath.Join(dir, "adir"), 0o755))

	err := conflicts.ResolveHunk(
		context.Background(), dir, "adir", "any-id",
		gitdomain.ConflictResolutionOurs, "",
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflicts: resolve hunk: read:")
}

func TestParseFile_AddAddConflict_NoBaseStage(t *testing.T) {
	dir := newBareRepo(t)
	gitIn(t, dir, "config", "merge.conflictstyle", "merge")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "root.txt"), []byte("root\n"), 0o644))
	gitIn(t, dir, "add", "root.txt")
	gitIn(t, dir, "commit", "-m", "root")

	gitIn(t, dir, "checkout", "-b", "branch-a")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "newfile.txt"), []byte("ours version\n"), 0o644))
	gitIn(t, dir, "add", "newfile.txt")
	gitIn(t, dir, "commit", "-m", "add on branch-a")

	gitIn(t, dir, "checkout", "main")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "newfile.txt"), []byte("main version\n"), 0o644))
	gitIn(t, dir, "add", "newfile.txt")
	gitIn(t, dir, "commit", "-m", "add on main")

	mergeCmd := exec.Command("git", "merge", "branch-a")
	mergeCmd.Dir = dir
	_ = mergeCmd.Run()

	hunks, err := conflicts.ParseFile(context.Background(), dir, "newfile.txt")

	require.NoError(t, err)
	require.Len(t, hunks, 1)
	assert.Empty(t, hunks[0].Base, "an add/add conflict has no common ancestor to show as Base")
}
