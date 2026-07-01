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

func TestRebaseThenFFMerge_ReplaysChildThenFastForwards(
	t *testing.T,
) {
	ctx := context.Background()
	repo := initRepo(t)
	makeCommit(t, repo, "base.txt", "base\n", "base")

	gitRun(t, repo, "branch", "feat")
	childWT := filepath.Join(t.TempDir(), "child")
	gitRun(t, repo, "worktree", "add", childWT, "feat")
	makeCommit(t, childWT, "child.txt", "child\n", "child work")

	makeCommit(t, repo, "parent.txt", "parent\n", "parent advance")
	parentTipBefore := headSHA(t, repo)

	e := git.New()
	err := e.RebaseThenFFMerge(ctx, childWT, "main", repo, "feat")
	require.NoError(t, err)

	assert.NotEqual(t, parentTipBefore, headSHA(t, repo), "parent should fast-forward")
	_, statErr := os.Stat(filepath.Join(repo, "child.txt"))
	require.NoError(t, statErr, "child commit should be present on parent")
}

func TestRebaseThenFFMerge_Conflict_ReturnsErrConflict(
	t *testing.T,
) {
	ctx := context.Background()
	repo := initRepo(t)
	makeCommit(t, repo, "conflict.txt", "base\n", "base")

	gitRun(t, repo, "branch", "feat")
	childWT := filepath.Join(t.TempDir(), "child")
	gitRun(t, repo, "worktree", "add", childWT, "feat")
	require.NoError(t, os.WriteFile(filepath.Join(childWT, "conflict.txt"), []byte("child\n"), 0o600))
	gitRun(t, childWT, "add", "conflict.txt")
	gitRun(t, childWT, "commit", "-m", "child change")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "conflict.txt"), []byte("parent\n"), 0o600))
	gitRun(t, repo, "add", "conflict.txt")
	gitRun(t, repo, "commit", "-m", "parent change")

	e := git.New()
	err := e.RebaseThenFFMerge(ctx, childWT, "main", repo, "feat")
	require.ErrorIs(t, err, git.ErrConflict)
}

func TestRepoMutex_SharedAcrossWorktreesOfSameClone(
	t *testing.T,
) {
	repo := initRepo(t)
	makeCommit(t, repo, "base.txt", "base\n", "base")
	childWT := filepath.Join(t.TempDir(), "child")
	gitRun(t, repo, "worktree", "add", "-b", "wt", childWT)

	ctx := context.Background()
	assert.True(
		t,
		git.SameRepoMutex(ctx, repo, childWT),
		"sibling worktrees of one clone must share a lock",
	)
	assert.Equal(
		t,
		git.ExportedResolveCommonDir(ctx, repo),
		git.ExportedResolveCommonDir(ctx, childWT),
		"both worktrees must resolve to the same common dir",
	)
}

func TestResolveCommonDir_NonGitDir_FallsBackToPath(
	t *testing.T,
) {
	dir := t.TempDir()
	assert.Equal(t, dir, git.ExportedResolveCommonDir(context.Background(), dir))
}
