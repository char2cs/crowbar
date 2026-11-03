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

// A squash into a parent holding the user's staged work is refused before it
// starts: committing would take that work into the squash, undoing would drop it.
func TestMergeSquash_RefusesAParentWithStagedWork(t *testing.T) {
	for name, edit := range map[string]string{"staged": "", "staged then edited": "edited\n"} {
		t.Run(name, func(t *testing.T) {
			repo := initRepo(t)
			makeCommit(t, repo, "base.txt", "base\n", "base")
			gitRun(t, repo, "checkout", "-b", "p1")
			makeCommit(t, repo, "p1.txt", "p1\n", "p1")
			gitRun(t, repo, "checkout", "main")
			require.NoError(t, os.WriteFile(filepath.Join(repo, "mine.txt"), []byte("mine\n"), 0o600))
			gitRun(t, repo, "add", "mine.txt")
			if edit != "" {
				require.NoError(t, os.WriteFile(filepath.Join(repo, "mine.txt"), []byte(edit), 0o600))
			}
			failingHook(t, repo, "pre-commit")

			err := git.New().MergeSquash(context.Background(), repo, "p1", "squash p1")

			require.ErrorIs(t, err, git.ErrDirtyTree)
			assert.Equal(t, "mine.txt", gitOutput(t, repo, "diff", "--cached", "--name-only"), "only the user's work is staged")
			assert.Equal(t, "mine", gitOutput(t, repo, "show", ":mine.txt"))
			assert.FileExists(t, filepath.Join(repo, "mine.txt"))
		})
	}
}

// A squash with no net change commits nothing and leaves no squash pending.
func TestMergeSquash_NoNetChangeLeavesNothingInProgress(t *testing.T) {
	repo := initRepo(t)
	makeCommit(t, repo, "base.txt", "base\n", "base")
	gitRun(t, repo, "checkout", "-b", "c")
	makeCommit(t, repo, "x.txt", "x\n", "x")
	gitRun(t, repo, "revert", "--no-edit", "HEAD")
	gitRun(t, repo, "checkout", "main")
	e := git.New()

	require.NoError(t, e.MergeSquash(context.Background(), repo, "c", "squash c"))

	op, err := e.OperationInProgress(context.Background(), repo)
	require.NoError(t, err)
	assert.Empty(t, op)
}

// An intent-to-add entry is staged work too: a squash committing over it, then
// refused by a hook, could not be undone (`reset --merge` rejects ita entries).
func TestMergeSquash_RefusesAParentWithAnIntentToAddEntry(t *testing.T) {
	repo := initRepo(t)
	makeCommit(t, repo, "base.txt", "base\n", "base")
	gitRun(t, repo, "checkout", "-b", "p1")
	makeCommit(t, repo, "p1.txt", "p1\n", "p1")
	gitRun(t, repo, "checkout", "main")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "ita.txt"), []byte("mine\n"), 0o600))
	gitRun(t, repo, "add", "-N", "ita.txt")
	failingHook(t, repo, "pre-commit")
	before := gitOutput(t, repo, "status", "--porcelain=v2")
	e := git.New()

	err := e.MergeSquash(context.Background(), repo, "p1", "squash p1")

	require.ErrorIs(t, err, git.ErrDirtyTree)
	assert.Equal(t, before, gitOutput(t, repo, "status", "--porcelain=v2"), "the user's ita entry is left as found")
	assert.NoFileExists(t, filepath.Join(repo, ".git", "SQUASH_MSG"))
	op, opErr := e.OperationInProgress(context.Background(), repo)
	require.NoError(t, opErr)
	assert.Empty(t, op)
}

// A parent branch with no commit yet has no HEAD to squash onto: refused in
// plain words, not as git's own error.
func TestMergeSquash_RefusesAParentWithNoCommitYet(t *testing.T) {
	repo := initRepo(t)
	makeCommit(t, repo, "base.txt", "base\n", "base")
	gitRun(t, repo, "branch", "p1")
	gitRun(t, repo, "switch", "-q", "--orphan", "empty")
	e := git.New()

	err := e.MergeSquash(context.Background(), repo, "p1", "squash p1")

	require.ErrorIs(t, err, git.ErrDirtyTree)
	assert.Equal(t, "the parent branch has no commits yet: commit to it first, then merge again", err.Error())
	assert.Empty(t, gitOutput(t, repo, "status", "--porcelain"))
}
