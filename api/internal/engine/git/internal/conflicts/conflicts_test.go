package conflicts_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/conflicts"
)

func newConflictRepo(
	t *testing.T,
) (repoPath, conflictFile string) {
	t.Helper()

	dir := t.TempDir()

	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	git("init", "-b", "main")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "Test")
	git("config", "merge.conflictstyle", "merge")

	absPath := filepath.Join(dir, "hello.txt")
	require.NoError(t, os.WriteFile(absPath, []byte("line one\nshared line\nline three\n"), 0o644))
	git("add", "hello.txt")
	git("commit", "-m", "initial")

	git("checkout", "-b", "branch-a")
	require.NoError(t, os.WriteFile(absPath, []byte("line one\nours change\nline three\n"), 0o644))
	git("add", "hello.txt")
	git("commit", "-m", "branch-a change")

	git("checkout", "main")
	require.NoError(t, os.WriteFile(absPath, []byte("line one\ntheirs change\nline three\n"), 0o644))
	git("add", "hello.txt")
	git("commit", "-m", "main change")

	mergeCmd := exec.Command("git", "merge", "branch-a")
	mergeCmd.Dir = dir
	_ = mergeCmd.Run()

	return dir, "hello.txt"
}

func newCleanRepo(
	t *testing.T,
) string {
	t.Helper()
	dir := t.TempDir()

	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	git("init", "-b", "main")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "Test")

	filePath := filepath.Join(dir, "clean.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("no conflicts here\n"), 0o644))
	git("add", "clean.txt")
	git("commit", "-m", "initial")

	return dir
}

func TestConflictedFiles(
	t *testing.T,
) {
	repoPath, _ := newConflictRepo(t)

	ctx := context.Background()
	paths, err := conflicts.ConflictedFiles(ctx, repoPath)
	require.NoError(t, err)
	require.Len(t, paths, 1)
	assert.Equal(t, "hello.txt", paths[0])
}

// TestConflictedFiles_None exercises the empty-output branch.
func TestConflictedFiles_None(
	t *testing.T,
) {
	repoPath := newCleanRepo(t)

	ctx := context.Background()
	paths, err := conflicts.ConflictedFiles(ctx, repoPath)
	require.NoError(t, err)
	assert.Empty(t, paths)
}

func TestHasConflicts_True(
	t *testing.T,
) {
	repoPath, _ := newConflictRepo(t)

	ctx := context.Background()
	has, err := conflicts.HasConflicts(ctx, repoPath)
	require.NoError(t, err)
	assert.True(t, has)
}

func TestHasConflicts_False(
	t *testing.T,
) {
	repoPath := newCleanRepo(t)

	ctx := context.Background()
	has, err := conflicts.HasConflicts(ctx, repoPath)
	require.NoError(t, err)
	assert.False(t, has)
}

func TestParseFile(
	t *testing.T,
) {
	repoPath, conflictFile := newConflictRepo(t)

	ctx := context.Background()
	hunks, err := conflicts.ParseFile(ctx, repoPath, conflictFile)
	require.NoError(t, err)
	require.Len(t, hunks, 1)

	h := hunks[0]
	assert.NotEmpty(t, h.ID)
	assert.Greater(t, h.StartLine, 0)
	assert.GreaterOrEqual(t, h.EndLine, h.StartLine)
	assert.Equal(t, gitdomain.ConflictResolutionUnresolved, h.Resolution)

	assert.True(t, strings.Contains(h.Ours, "theirs change") || strings.Contains(h.Theirs, "theirs change"),
		"expected 'theirs change' in ours or theirs, got ours=%q theirs=%q", h.Ours, h.Theirs)
	assert.True(t, strings.Contains(h.Ours, "ours change") || strings.Contains(h.Theirs, "ours change"),
		"expected 'ours change' in ours or theirs, got ours=%q theirs=%q", h.Ours, h.Theirs)
}

// TestParseFile_NoMarkers exercises the branch where the file has no conflict
// markers — ParseFile should return an empty slice without error.
func TestParseFile_NoMarkers(
	t *testing.T,
) {
	repoPath := newCleanRepo(t)

	ctx := context.Background()
	hunks, err := conflicts.ParseFile(ctx, repoPath, "clean.txt")
	require.NoError(t, err)
	assert.Empty(t, hunks)
}

// TestParseFile_MissingFile exercises the os.ReadFile error path in ParseFile.
func TestParseFile_MissingFile(
	t *testing.T,
) {
	repoPath := newCleanRepo(t)

	ctx := context.Background()
	_, err := conflicts.ParseFile(ctx, repoPath, "nonexistent.txt")
	require.Error(t, err)
}

func TestResolveHunk_Ours(
	t *testing.T,
) {
	repoPath, conflictFile := newConflictRepo(t)

	ctx := context.Background()
	hunks, err := conflicts.ParseFile(ctx, repoPath, conflictFile)
	require.NoError(t, err)
	require.Len(t, hunks, 1)

	err = conflicts.ResolveHunk(
		ctx,
		repoPath,
		conflictFile,
		hunks[0].ID,
		gitdomain.ConflictResolutionOurs,
		"",
	)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(repoPath, conflictFile))
	require.NoError(t, err)

	content := string(data)
	assert.NotContains(t, content, "<<<<<<<")
	assert.NotContains(t, content, "=======")
	assert.NotContains(t, content, ">>>>>>>")
}

func TestResolveHunk_StagesWhenLastConflictResolved(
	t *testing.T,
) {
	repoPath, conflictFile := newConflictRepo(t)

	ctx := context.Background()
	hunks, err := conflicts.ParseFile(ctx, repoPath, conflictFile)
	require.NoError(t, err)
	require.Len(t, hunks, 1)

	err = conflicts.ResolveHunk(
		ctx,
		repoPath,
		conflictFile,
		hunks[0].ID,
		gitdomain.ConflictResolutionOurs,
		"",
	)
	require.NoError(t, err)

	hasConflicts, err := conflicts.HasConflicts(ctx, repoPath)
	require.NoError(t, err)
	assert.False(t, hasConflicts, "expected no conflicts after resolving the only hunk")
}

func TestResolveHunk_Theirs(
	t *testing.T,
) {
	repoPath, conflictFile := newConflictRepo(t)

	ctx := context.Background()
	hunks, err := conflicts.ParseFile(ctx, repoPath, conflictFile)
	require.NoError(t, err)
	require.Len(t, hunks, 1)

	err = conflicts.ResolveHunk(
		ctx,
		repoPath,
		conflictFile,
		hunks[0].ID,
		gitdomain.ConflictResolutionTheirs,
		"",
	)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(repoPath, conflictFile))
	require.NoError(t, err)

	content := string(data)
	assert.NotContains(t, content, "<<<<<<<")
}

func TestResolveHunk_Custom(
	t *testing.T,
) {
	repoPath, conflictFile := newConflictRepo(t)

	ctx := context.Background()
	hunks, err := conflicts.ParseFile(ctx, repoPath, conflictFile)
	require.NoError(t, err)
	require.Len(t, hunks, 1)

	err = conflicts.ResolveHunk(
		ctx,
		repoPath,
		conflictFile,
		hunks[0].ID,
		gitdomain.ConflictResolutionCustom,
		"custom resolution line",
	)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(repoPath, conflictFile))
	require.NoError(t, err)

	content := string(data)
	assert.NotContains(t, content, "<<<<<<<")
	assert.Contains(t, content, "custom resolution line")
}

func TestResolveHunk_Both(
	t *testing.T,
) {
	repoPath, conflictFile := newConflictRepo(t)

	ctx := context.Background()
	hunks, err := conflicts.ParseFile(ctx, repoPath, conflictFile)
	require.NoError(t, err)
	require.Len(t, hunks, 1)

	err = conflicts.ResolveHunk(
		ctx,
		repoPath,
		conflictFile,
		hunks[0].ID,
		gitdomain.ConflictResolutionBoth,
		"",
	)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(repoPath, conflictFile))
	require.NoError(t, err)

	content := string(data)
	assert.NotContains(t, content, "<<<<<<<")
}

// TestResolveHunk_HunkNotFound exercises the findBlock nil-return error path.
func TestResolveHunk_HunkNotFound(
	t *testing.T,
) {
	repoPath, conflictFile := newConflictRepo(t)

	ctx := context.Background()
	err := conflicts.ResolveHunk(
		ctx,
		repoPath,
		conflictFile,
		"000000000000", // valid-length but non-existent hunk ID
		gitdomain.ConflictResolutionOurs,
		"",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestResolveHunk_MissingFile exercises the os.ReadFile error path in ResolveHunk.
func TestResolveHunk_MissingFile(
	t *testing.T,
) {
	repoPath := newCleanRepo(t)

	ctx := context.Background()
	err := conflicts.ResolveHunk(
		ctx,
		repoPath,
		"does_not_exist.txt",
		"abc123",
		gitdomain.ConflictResolutionOurs,
		"",
	)
	require.Error(t, err)
}

// TestResolveHunk_WriteError exercises the os.WriteFile error path.
// We make the file read-only so the write fails after extracting blocks.
func TestResolveHunk_WriteError(
	t *testing.T,
) {
	repoPath, conflictFile := newConflictRepo(t)

	ctx := context.Background()
	hunks, err := conflicts.ParseFile(ctx, repoPath, conflictFile)
	require.NoError(t, err)
	require.Len(t, hunks, 1)

	// Make the directory read-only so WriteFile fails.
	absPath := filepath.Join(repoPath, conflictFile)
	require.NoError(t, os.Chmod(absPath, 0o444))
	t.Cleanup(func() { _ = os.Chmod(absPath, 0o644) })

	err = conflicts.ResolveHunk(
		ctx,
		repoPath,
		conflictFile,
		hunks[0].ID,
		gitdomain.ConflictResolutionOurs,
		"",
	)
	require.Error(t, err)
}

// TestHasMarkers_False exercises the hasMarkers false branch by calling
// ParseFile on a file with no markers (hasMarkers returns false for normal files).
// Covered indirectly: ResolveHunk calls hasMarkers on post-resolution content.
// This test ensures the no-markers path is hit without going through conflict setup.
func TestHasMarkers_FalseViaParseFile(
	t *testing.T,
) {
	repoPath := newCleanRepo(t)

	ctx := context.Background()
	hunks, err := conflicts.ParseFile(ctx, repoPath, "clean.txt")
	require.NoError(t, err)
	assert.Empty(t, hunks) // no conflict markers → empty result
}

// TestResolveHunk_HasMarkersReturnsTrue exercises the hasMarkers(updated) == true
// branch: resolve only the first hunk of a file that has multiple conflict hunks
// so that remaining markers prevent auto-staging.
func TestResolveHunk_HasMarkersReturnsTrue(
	t *testing.T,
) {
	repoPath, conflictFile := newConflictRepo(t)

	ctx := context.Background()

	// Manually write a two-hunk conflict file to force two hunks.
	twoHunk := "line1\n<<<<<<< HEAD\nhunk1-ours\n=======\nhunk1-theirs\n>>>>>>> b\nmiddle\n<<<<<<< HEAD\nhunk2-ours\n=======\nhunk2-theirs\n>>>>>>> b\nline_end\n"
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, conflictFile), []byte(twoHunk), 0o644))

	hunks, err := conflicts.ParseFile(ctx, repoPath, conflictFile)
	require.NoError(t, err)
	require.Len(t, hunks, 2)

	// Resolve only the first hunk — file still has a second conflict marker.
	err = conflicts.ResolveHunk(
		ctx,
		repoPath,
		conflictFile,
		hunks[0].ID,
		gitdomain.ConflictResolutionOurs,
		"",
	)
	require.NoError(t, err)

	// File should still contain the second conflict marker.
	data, err := os.ReadFile(filepath.Join(repoPath, conflictFile))
	require.NoError(t, err)
	assert.Contains(t, string(data), "<<<<<<<")
}

// TestConflictedFiles_NonGitDir exercises the RequireSuccess error path in
// ConflictedFiles by passing a directory that is not a git repository.
func TestConflictedFiles_NonGitDir(
	t *testing.T,
) {
	dir := t.TempDir() // plain directory, no git init

	ctx := context.Background()
	_, err := conflicts.ConflictedFiles(ctx, dir)
	require.Error(t, err)
}

// TestHasConflicts_NonGitDir exercises the RequireSuccess error path in
// HasConflicts by passing a directory that is not a git repository.
func TestHasConflicts_NonGitDir(
	t *testing.T,
) {
	dir := t.TempDir() // plain directory, no git init

	ctx := context.Background()
	_, err := conflicts.HasConflicts(ctx, dir)
	require.Error(t, err)
}

func TestConflictedFiles_InjectedExecError(
	t *testing.T,
) {
	conflicts.SetErrorRunner(t.Cleanup)
	ctx := context.Background()

	_, err := conflicts.ConflictedFiles(ctx, t.TempDir())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflicts: conflicted files")
}

func TestHasConflicts_InjectedExecError(
	t *testing.T,
) {
	conflicts.SetErrorRunner(t.Cleanup)
	ctx := context.Background()

	_, err := conflicts.HasConflicts(ctx, t.TempDir())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflicts: has conflicts")
}

func TestResolveHunk_InjectedExecError_Stage(
	t *testing.T,
) {
	repoPath, conflictFile := newConflictRepo(t)

	ctx := context.Background()
	hunks, err := conflicts.ParseFile(ctx, repoPath, conflictFile)
	require.NoError(t, err)
	require.Len(t, hunks, 1)

	conflicts.SetErrorRunner(t.Cleanup)

	err = conflicts.ResolveHunk(
		ctx,
		repoPath,
		conflictFile,
		hunks[0].ID,
		gitdomain.ConflictResolutionOurs,
		"",
	)
	require.Error(t, err)
}
