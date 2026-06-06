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

func TestMergeSquash_ProducesSquashedCommit(
	t *testing.T,
) {
	repo := initRepo(t)
	makeCommit(t, repo, "base.txt", "base\n", "base commit")

	// create child branch with two commits
	gitRun(t, repo, "checkout", "-b", "feature/sq")
	makeCommit(t, repo, "a.txt", "a\n", "commit A")
	makeCommit(t, repo, "b.txt", "b\n", "commit B")

	// switch back to main
	gitRun(t, repo, "checkout", "main")
	parentTipBefore := headSHA(t, repo)

	e := git.New()
	ctx := context.Background()

	err := e.MergeSquash(ctx, repo, "feature/sq", "squashed work")

	require.NoError(t, err)
	parentTipAfter := headSHA(t, repo)
	assert.NotEqual(t, parentTipBefore, parentTipAfter, "parent tip should advance after squash merge")

	// the squash should produce exactly one new commit on main
	commits, err := e.Log(ctx, repo, 10, 0)
	require.NoError(t, err)
	assert.Equal(t, "squashed work", commits[0].Message)
}

func TestMergeSquash_InvalidBranch_ReturnsError(
	t *testing.T,
) {
	repo := initRepo(t)
	makeCommit(t, repo, "base.txt", "base\n", "init")
	e := git.New()
	ctx := context.Background()

	err := e.MergeSquash(ctx, repo, "nonexistent-branch", "subject")

	require.Error(t, err)
}

func TestMergeSquash_Conflict_ReturnsErrConflict(
	t *testing.T,
) {
	repo := initRepo(t)
	makeCommit(t, repo, "conflict.txt", "original\n", "base commit")

	gitRun(t, repo, "checkout", "-b", "feature/conflict")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "conflict.txt"), []byte("feature\n"), 0o600))
	gitRun(t, repo, "add", "conflict.txt")
	gitRun(t, repo, "commit", "-m", "feature change")

	gitRun(t, repo, "checkout", "main")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "conflict.txt"), []byte("main\n"), 0o600))
	gitRun(t, repo, "add", "conflict.txt")
	gitRun(t, repo, "commit", "-m", "main change")

	e := git.New()
	ctx := context.Background()

	err := e.MergeSquash(ctx, repo, "feature/conflict", "subject")

	require.Error(t, err)
	assert.ErrorIs(t, err, git.ErrConflict)
}

func TestMergeSquash_NoNetChange_ReturnsNil(
	t *testing.T,
) {
	repo := initRepo(t)
	makeCommit(t, repo, "base.txt", "base\n", "base commit")

	// branch from main with no new commits: squash stages nothing.
	gitRun(t, repo, "branch", "feature/empty")
	parentTipBefore := headSHA(t, repo)

	e := git.New()
	ctx := context.Background()

	err := e.MergeSquash(ctx, repo, "feature/empty", "noop squash")

	require.NoError(t, err)
	assert.Equal(t, parentTipBefore, headSHA(t, repo), "parent must be unchanged")
}
