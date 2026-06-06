package git_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git"
)

func TestWorktreeAddBranch_CreatesBranchAndReturnsStartSha(
	t *testing.T,
) {
	repo := initRepo(t)
	makeCommit(t, repo, "file.txt", "content\n", "init")
	e := git.New()
	ctx := context.Background()

	child := filepath.Join(t.TempDir(), "child")
	startSha, err := e.WorktreeAddBranch(ctx, repo, child, "feature/x", "HEAD")

	require.NoError(t, err)
	assert.NotEmpty(t, startSha)
	assert.Equal(t, headSHA(t, repo), startSha)

	entries, err := e.WorktreeList(ctx, repo)
	require.NoError(t, err)
	found := false
	for _, en := range entries {
		if en.Branch == "feature/x" {
			found = true
		}
	}
	assert.True(t, found, "worktree on branch feature/x not found")
}

func TestWorktreeAddBranch_InvalidStartPoint_ReturnsError(
	t *testing.T,
) {
	repo := initRepo(t)
	makeCommit(t, repo, "file.txt", "content\n", "init")
	e := git.New()
	ctx := context.Background()

	child := filepath.Join(t.TempDir(), "child")
	_, err := e.WorktreeAddBranch(ctx, repo, child, "feature/y", "nonexistent-ref")

	require.Error(t, err)
}
