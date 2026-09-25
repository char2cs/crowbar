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

func TestUncommittedFiles_CountsTrackedAndUntrackedButNotIgnored(t *testing.T) {
	repo := initRepo(t)
	makeCommit(t, repo, ".gitignore", "build/\n", "ignore build")
	makeCommit(t, repo, "a.txt", "a\n", "a")
	e := git.New()
	ctx := context.Background()

	n, err := e.UncommittedFiles(ctx, repo)
	require.NoError(t, err)
	assert.Zero(t, n, "a clean checkout has nothing uncommitted")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "a.txt"), []byte("changed\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(repo, "new", "deep"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "new", "deep", "b.txt"), []byte("b\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "new", "c.txt"), []byte("c\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(repo, "build"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "build", "out.bin"), []byte("x"), 0o600))

	n, err = e.UncommittedFiles(ctx, repo)
	require.NoError(t, err)
	assert.Equal(t, 3, n, "one modified file and two untracked ones; the ignored build output is not work")
}

func TestUnmergedCommits_CountsOnlyCommitsNoOtherRefReaches(t *testing.T) {
	repo := initRepo(t)
	makeCommit(t, repo, "base.txt", "base\n", "base")
	gitRun(t, repo, "checkout", "-q", "-b", "feature/x")
	makeCommit(t, repo, "f1.txt", "1\n", "f1")
	makeCommit(t, repo, "f2.txt", "2\n", "f2")
	gitRun(t, repo, "checkout", "-q", "main")
	e := git.New()
	ctx := context.Background()

	n, err := e.UnmergedCommits(ctx, repo, []string{"refs/heads/feature/x"}, "feature/x")
	require.NoError(t, err)
	assert.Equal(t, 2, n, "both feature commits exist only on the branch being dropped")

	n, err = e.UnmergedCommits(ctx, repo, []string{"refs/heads/feature/x"}, "")
	require.NoError(t, err)
	assert.Zero(t, n, "a branch that is kept keeps its own commits")

	gitRun(t, repo, "branch", "backup", "feature/x~1")
	n, err = e.UnmergedCommits(ctx, repo, []string{"refs/heads/feature/x"}, "feature/x")
	require.NoError(t, err)
	assert.Equal(t, 1, n, "a commit another branch reaches is not lost")

	gitRun(t, repo, "tag", "keep", "feature/x")
	n, err = e.UnmergedCommits(ctx, repo, []string{"refs/heads/feature/x"}, "feature/x")
	require.NoError(t, err)
	assert.Zero(t, n, "a tagged commit is not lost")
}

func TestUnmergedCommits_ResolvesHEADInTheWorktreeAndIgnoresMissingTips(t *testing.T) {
	repo := initRepo(t)
	makeCommit(t, repo, "base.txt", "base\n", "base")
	wt := filepath.Join(t.TempDir(), "wt")
	gitRun(t, repo, "worktree", "add", "-q", "--detach", wt)
	makeCommit(t, wt, "detached.txt", "d\n", "on a detached HEAD")
	e := git.New()
	ctx := context.Background()

	n, err := e.UnmergedCommits(ctx, wt, []string{"HEAD", "refs/heads/gone"}, "gone")
	require.NoError(t, err)
	assert.Equal(t, 1, n, "a commit only the worktree's detached HEAD reaches dies with the worktree")
}
