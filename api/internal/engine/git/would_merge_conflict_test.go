package git_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git"
)

func TestWouldMergeConflict_NonOverlappingIsClean(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "base.txt", "base\n", "base")

	gitRun(t, dir, "checkout", "-b", "feature")
	makeCommit(t, dir, "feature.txt", "feature\n", "feature work")

	gitRun(t, dir, "checkout", "main")
	makeCommit(t, dir, "main.txt", "main\n", "main work")

	got, err := git.New().WouldMergeConflict(ctx, dir, "main", "feature")
	require.NoError(t, err)
	assert.False(t, got, "edits to different files merge cleanly")
}

func TestWouldMergeConflict_OverlappingEditsConflict(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "shared.txt", "base\n", "base")

	gitRun(t, dir, "checkout", "-b", "feature")
	makeCommit(t, dir, "shared.txt", "feature change\n", "feature edit")

	gitRun(t, dir, "checkout", "main")
	makeCommit(t, dir, "shared.txt", "main change\n", "main edit")

	got, err := git.New().WouldMergeConflict(ctx, dir, "main", "feature")
	require.NoError(t, err)
	assert.True(t, got, "diverging edits to the same line conflict")
}

// Critical regression: git merge-tree exits 1 for a conflict AND for an
// unresolvable ref, but the latter writes no tree to stdout. The unresolvable
// case must return an ERROR (so the caller fails OPEN and allows the merge),
// never be misread as a conflict that wrongly blocks a clean branch.
func TestWouldMergeConflict_UnknownRefErrors(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")

	got, err := git.New().WouldMergeConflict(ctx, dir, "main", "no-such-branch")
	require.Error(t, err)
	assert.False(t, got, "an unresolvable ref must not be reported as a conflict")
}

// Critical regression: a missing worktree directory must error (fail open), not
// be misread as a conflict.
func TestWouldMergeConflict_MissingRepoErrors(t *testing.T) {
	ctx := context.Background()
	got, err := git.New().WouldMergeConflict(ctx, "/no/such/repo/path/xyz123", "main", "feature")
	require.Error(t, err)
	assert.False(t, got, "a missing worktree must not be reported as a conflict")
}
