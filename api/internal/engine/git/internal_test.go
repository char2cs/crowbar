package git_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git"
)

func TestComputeStatus(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")

	status, err := git.ExportedComputeStatus(ctx, dir)

	require.NoError(t, err)
	assert.Equal(t, "main", status.Branch)
}

func TestComputeWorkingTreeSummary_WithFork(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")
	fork := headSHA(t, dir)
	makeCommit(t, dir, "new.txt", "new\n", "after fork")

	added, deleted, hasConflicts, hasCommits, err := git.ExportedComputeWorkingTreeSummary(ctx, dir, fork)

	require.NoError(t, err)
	assert.GreaterOrEqual(t, added, 0)
	assert.GreaterOrEqual(t, deleted, 0)
	assert.False(t, hasConflicts)
	assert.True(t, hasCommits)
}

func TestParseWorktreeList_SingleEntry(
	t *testing.T,
) {
	output := "worktree /path/to/repo\nHEAD abc123\nbranch refs/heads/main\n\n"

	entries := git.ExportedParseWorktreeList(output)

	require.Len(t, entries, 1)
	assert.Equal(t, "/path/to/repo", entries[0].Path)
	assert.Equal(t, "abc123", entries[0].Head)
	assert.Equal(t, "main", entries[0].Branch)
}

func TestParseWorktreeList_MultipleEntries(
	t *testing.T,
) {
	output := "worktree /repo\nHEAD abc1\nbranch refs/heads/main\n\nworktree /wt\nHEAD def2\nbranch refs/heads/feature\n\n"

	entries := git.ExportedParseWorktreeList(output)

	require.Len(t, entries, 2)
	assert.Equal(t, "/repo", entries[0].Path)
	assert.Equal(t, "/wt", entries[1].Path)
	assert.Equal(t, "feature", entries[1].Branch)
}

func TestParseWorktreeList_DetachedHead(
	t *testing.T,
) {
	output := "worktree /repo\nHEAD abc123\ndetached\n\n"

	entries := git.ExportedParseWorktreeList(output)

	require.Len(t, entries, 1)
	assert.Equal(t, "abc123", entries[0].Head)
	assert.Empty(t, entries[0].Branch)
}

func TestParseWorktreeList_EmptyOutput(
	t *testing.T,
) {
	entries := git.ExportedParseWorktreeList("")
	assert.Empty(t, entries)
}

func TestParseNumstat_Basic(
	t *testing.T,
) {
	output := "5\t3\tfile.go\n10\t2\tother.go\n"

	added, deleted, err := git.ExportedParseNumstat(output)

	require.NoError(t, err)
	assert.Equal(t, 15, added)
	assert.Equal(t, 5, deleted)
}

func TestParseNumstat_BinaryFile(
	t *testing.T,
) {
	output := "-\t-\tbinary.bin\n"

	added, deleted, err := git.ExportedParseNumstat(output)

	require.NoError(t, err)
	assert.Equal(t, 0, added)
	assert.Equal(t, 0, deleted)
}

func TestParseNumstat_EmptyOutput(
	t *testing.T,
) {
	added, deleted, err := git.ExportedParseNumstat("")

	require.NoError(t, err)
	assert.Equal(t, 0, added)
	assert.Equal(t, 0, deleted)
}

func TestDetectInProgressOp_MergeHead(
	t *testing.T,
) {
	dir := initRepo(t)
	gitDir := filepath.Join(dir, ".git")

	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "MERGE_HEAD"), []byte("abc123"), 0600))

	op := git.ExportedDetectInProgressOp(dir)
	assert.Equal(t, "merge", op)
}

func TestDetectInProgressOp_RebaseMerge(
	t *testing.T,
) {
	dir := initRepo(t)
	gitDir := filepath.Join(dir, ".git")

	require.NoError(t, os.Mkdir(filepath.Join(gitDir, "rebase-merge"), 0700))

	op := git.ExportedDetectInProgressOp(dir)
	assert.Equal(t, "rebase", op)
}

func TestDetectInProgressOp_RebaseApply(
	t *testing.T,
) {
	dir := initRepo(t)
	gitDir := filepath.Join(dir, ".git")

	require.NoError(t, os.Mkdir(filepath.Join(gitDir, "rebase-apply"), 0700))

	op := git.ExportedDetectInProgressOp(dir)
	assert.Equal(t, "rebase", op)
}

func TestDetectInProgressOp_None(
	t *testing.T,
) {
	dir := initRepo(t)

	op := git.ExportedDetectInProgressOp(dir)
	assert.Empty(t, op)
}
