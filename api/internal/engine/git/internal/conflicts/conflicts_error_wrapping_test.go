// conflicts_error_wrapping_test.go covers the error-wrapping call sites inside
// ParseFile and ResolveHunk themselves. extractConflictBlocks and resolvedText
// already have direct unit tests for their own error returns
// (conflicts_internal_test.go), but that does not exercise the separate
// "if err != nil { return fmt.Errorf(...) }" blocks where ParseFile and
// ResolveHunk wrap those errors — those need the real public entry points.
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

// TestParseFile_UnclosedConflictMarker_WrapsExtractError pins ParseFile's own
// "conflicts: parse file: %w" wrapper (a block distinct from
// extractConflictBlocks's own error return, which conflicts_internal_test.go
// already covers directly). A file with a stray "<<<<<<<" and no closing
// ">>>>>>>" — e.g. one edited by hand after an aborted merge — must surface as
// a real, attributed error rather than a panic or a silently empty result.
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

// TestResolveHunk_UnclosedConflictMarker_WrapsExtractError is ResolveHunk's
// counterpart to the ParseFile test above: the same malformed content, reached
// through the resolve path's own "conflicts: resolve hunk: parse: %w" wrapper.
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

// TestResolveHunk_UnresolvedResolution_ReturnsError covers the caller-facing
// side of resolvedText's default branch: passing back "unresolved" (or any
// value other than the four real resolutions) on an otherwise-valid hunk must
// fail the whole resolve rather than write garbage into the file.
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

	// The file must be left untouched: a rejected resolution is not a partial one.
	data, readErr := os.ReadFile(absPath)
	require.NoError(t, readErr)
	assert.Contains(t, string(data), "<<<<<<<", "a rejected resolution must not touch the file")
}

// TestResolveHunk_PathIsDirectory_ReadFileError covers the os.ReadFile error
// branch in ResolveHunk specifically — as opposed to the os.Stat error branch
// TestResolveHunk_MissingFile already covers in conflicts_test.go. Passing a
// directory as the "file" makes Stat succeed (a directory has a mode) but
// ReadFile fail, which is the only realistic way to separate the two branches.
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

// TestParseFile_AddAddConflict_NoBaseStage pins fetchBase's ExitCode != 0
// branch: an add/add conflict (both sides independently create the same new
// path) has no common-ancestor stage at all, so `git show :1:<path>` fails and
// fetchBase must degrade to an empty Base rather than propagating the failure
// — a base is optional context for the UI, and its absence is not itself an
// error.
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
