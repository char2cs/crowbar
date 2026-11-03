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

// stackedWorktree builds main -> p1 -> child plus a sibling p2, with child
// checked out in its own worktree, and returns (repo, child worktree).
func stackedWorktree(t *testing.T) (string, string) {
	t.Helper()
	repo := initRepo(t)
	makeCommit(t, repo, "base.txt", "base\n", "base")
	gitRun(t, repo, "checkout", "-b", "p1")
	makeCommit(t, repo, "p1.txt", "p1\n", "p1")
	gitRun(t, repo, "checkout", "-b", "p2", "main")
	makeCommit(t, repo, "p2.txt", "p2\n", "p2")
	gitRun(t, repo, "checkout", "-b", "child", "p1")
	makeCommit(t, repo, "child.txt", "child\n", "child")
	gitRun(t, repo, "checkout", "main")
	wt := filepath.Join(t.TempDir(), "child")
	gitRun(t, repo, "worktree", "add", wt, "child")
	return repo, wt
}

// withoutIdentity makes every commit in repo fail the way a container with no
// git identity does.
func withoutIdentity(t *testing.T, repo string) {
	t.Helper()
	gitRun(t, repo, "config", "--unset", "user.email")
	gitRun(t, repo, "config", "--unset", "user.name")
	gitRun(t, repo, "config", "user.useConfigOnly", "true")
}

func withIdentity(t *testing.T, repo string) {
	t.Helper()
	gitRun(t, repo, "config", "user.email", "test@test.com")
	gitRun(t, repo, "config", "user.name", "Test User")
}

func failingHook(t *testing.T, repo, name string) {
	t.Helper()
	hook := filepath.Join(gitOutput(t, repo, "rev-parse", "--path-format=absolute", "--git-common-dir"), "hooks", name)
	require.NoError(t, os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0o700))
}

func requireAtRest(t *testing.T, e git.Engine, dir, wantHead string) {
	t.Helper()
	op, err := e.OperationInProgress(context.Background(), dir)
	require.NoError(t, err)
	assert.Empty(t, op, "a failed command must not leave an operation in progress")
	assert.Equal(t, wantHead, headSHA(t, dir))
	assert.Empty(t, gitOutput(t, dir, "status", "--porcelain"))
}

func TestRebaseOnto_FailureBeforeAnyConflictLeavesTheWorktreeAsFoundAndTheNextRebaseWorks(t *testing.T) {
	ctx := context.Background()
	repo, wt := stackedWorktree(t)
	before := headSHA(t, wt)
	tip := gitOutput(t, repo, "rev-parse", "p2")
	base := gitOutput(t, repo, "merge-base", "p1", "child")
	withoutIdentity(t, repo)
	e := git.New()

	err := e.RebaseOnto(ctx, wt, tip, base, "child")

	require.Error(t, err)
	require.NotErrorIs(t, err, git.ErrConflict)
	requireAtRest(t, e, wt, before)
	assert.Equal(t, "child", gitOutput(t, wt, "branch", "--show-current"))

	withIdentity(t, repo)
	require.NoError(t, e.RebaseOnto(ctx, wt, tip, base, "child"))
	assert.Equal(t, tip, gitOutput(t, wt, "rev-parse", "HEAD~1"))
}

func TestRebase_FailureBeforeAnyConflictLeavesTheWorktreeAsFound(t *testing.T) {
	_, wt := stackedWorktree(t)
	before := headSHA(t, wt)
	withoutIdentity(t, wt)
	e := git.New()

	err := e.Rebase(context.Background(), wt, "p2")

	require.Error(t, err)
	requireAtRest(t, e, wt, before)
}

func TestRebaseThenFFMerge_FailureBeforeAnyConflictLeavesTheChildAsFound(t *testing.T) {
	repo, wt := stackedWorktree(t)
	before := headSHA(t, wt)
	withoutIdentity(t, repo)
	e := git.New()

	err := e.RebaseThenFFMerge(context.Background(), wt, "p2", repo, "child")

	require.Error(t, err)
	requireAtRest(t, e, wt, before)
}

func TestPull_RebaseFailureBeforeAnyConflictLeavesTheWorktreeAsFound(t *testing.T) {
	repo, wt := stackedWorktree(t)
	gitRun(t, wt, "remote", "add", "origin", repo)
	gitRun(t, wt, "fetch", "-q", "origin")
	gitRun(t, wt, "branch", "--set-upstream-to=origin/p2")
	before := headSHA(t, wt)
	withoutIdentity(t, repo)
	e := git.New()

	err := e.Pull(context.Background(), wt, "rebase")

	require.Error(t, err)
	requireAtRest(t, e, wt, before)
}

func TestMergeCommit_RefusedByAHookLeavesTheWorktreeAsFound(t *testing.T) {
	repo, _ := stackedWorktree(t)
	gitRun(t, repo, "checkout", "p2")
	before := headSHA(t, repo)
	failingHook(t, repo, "pre-merge-commit")
	e := git.New()

	err := e.MergeCommit(context.Background(), repo, "p1")

	require.Error(t, err)
	requireAtRest(t, e, repo, before)
}

func TestMerge_RefusedByAHookLeavesTheWorktreeAsFound(t *testing.T) {
	repo, _ := stackedWorktree(t)
	gitRun(t, repo, "checkout", "p2")
	before := headSHA(t, repo)
	failingHook(t, repo, "pre-merge-commit")
	e := git.New()

	err := e.Merge(context.Background(), repo, "p1")

	require.Error(t, err)
	requireAtRest(t, e, repo, before)
}

func TestMergeSquash_CommitRefusedLeavesTheWorktreeAsFound(t *testing.T) {
	repo, _ := stackedWorktree(t)
	gitRun(t, repo, "checkout", "p2")
	before := headSHA(t, repo)
	failingHook(t, repo, "pre-commit")
	e := git.New()

	err := e.MergeSquash(context.Background(), repo, "p1", "squash p1")

	require.Error(t, err)
	requireAtRest(t, e, repo, before)
}

func conflictingWorktree(t *testing.T) (string, string) {
	t.Helper()
	repo := initRepo(t)
	makeCommit(t, repo, "f.txt", "base\n", "base")
	gitRun(t, repo, "checkout", "-b", "other")
	makeCommit(t, repo, "f.txt", "other\n", "other")
	gitRun(t, repo, "checkout", "-b", "child", "main")
	makeCommit(t, repo, "f.txt", "child\n", "child")
	gitRun(t, repo, "checkout", "main")
	wt := filepath.Join(t.TempDir(), "child")
	gitRun(t, repo, "worktree", "add", wt, "child")
	return repo, wt
}

func TestRebaseOnto_ConflictKeepsTheRebaseForTheUser(t *testing.T) {
	repo, wt := conflictingWorktree(t)
	e := git.New()

	err := e.RebaseOnto(context.Background(), wt, gitOutput(t, repo, "rev-parse", "other"), gitOutput(t, repo, "rev-parse", "main"), "child")

	require.ErrorIs(t, err, git.ErrConflict)
	op, opErr := e.OperationInProgress(context.Background(), wt)
	require.NoError(t, opErr)
	assert.Equal(t, "rebase", op)
}

func TestRebaseOnto_NeverAbortsARebaseThatWasAlreadyInProgress(t *testing.T) {
	ctx := context.Background()
	repo, wt := conflictingWorktree(t)
	e := git.New()
	require.ErrorIs(t, e.Rebase(ctx, wt, "other"), git.ErrConflict)

	err := e.RebaseOnto(ctx, wt, gitOutput(t, repo, "rev-parse", "other"), gitOutput(t, repo, "rev-parse", "main"), "child")

	require.Error(t, err)
	op, opErr := e.OperationInProgress(ctx, wt)
	require.NoError(t, opErr)
	assert.Equal(t, "rebase", op, "the user's own rebase is theirs to finish")
}

// diverged is a repo on main with p1 and main each one commit past base, and
// a file the user staged plus one they left dirty.
func diverged(t *testing.T) string {
	t.Helper()
	repo := initRepo(t)
	makeCommit(t, repo, "base.txt", "base\n", "base")
	gitRun(t, repo, "checkout", "-b", "p1")
	makeCommit(t, repo, "p1.txt", "p1\n", "p1")
	gitRun(t, repo, "checkout", "main")
	makeCommit(t, repo, "main.txt", "main\n", "main")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "mine.txt"), []byte("mine\n"), 0o600))
	gitRun(t, repo, "add", "mine.txt")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "base.txt"), []byte("dirty\n"), 0o600))
	return repo
}

func requireUsersChangesKept(t *testing.T, repo string) {
	t.Helper()
	assert.Equal(t, "mine.txt", gitOutput(t, repo, "diff", "--cached", "--name-only"), "the staged file stays staged")
	assert.Equal(t, "base.txt", gitOutput(t, repo, "diff", "--name-only"), "the dirty file stays dirty")
	assert.FileExists(t, filepath.Join(repo, "mine.txt"))
}

func TestMergeVerbs_RefusedOverTheUsersChangesKeepThem(t *testing.T) {
	verbs := map[string]func(git.Engine, string) error{
		"merge":  func(e git.Engine, repo string) error { return e.Merge(context.Background(), repo, "p1") },
		"commit": func(e git.Engine, repo string) error { return e.MergeCommit(context.Background(), repo, "p1") },
		"squash": func(e git.Engine, repo string) error { return e.MergeSquash(context.Background(), repo, "p1", "s") },
	}
	for name, run := range verbs {
		t.Run(name, func(t *testing.T) {
			repo := diverged(t)

			require.Error(t, run(git.New(), repo))

			requireUsersChangesKept(t, repo)
		})
	}
}

func TestPull_RefusedOverTheUsersChangesKeepsThem(t *testing.T) {
	for _, mode := range []string{"merge", "rebase"} {
		t.Run(mode, func(t *testing.T) {
			repo := diverged(t)
			clone := filepath.Join(t.TempDir(), "clone")
			gitRun(t, repo, "branch", "up", "p1")
			gitRun(t, repo, "worktree", "add", "-q", clone, "-b", "work", "main")
			gitRun(t, clone, "branch", "--set-upstream-to=up")
			require.NoError(t, os.WriteFile(filepath.Join(clone, "mine.txt"), []byte("mine\n"), 0o600))
			gitRun(t, clone, "add", "mine.txt")
			require.NoError(t, os.WriteFile(filepath.Join(clone, "base.txt"), []byte("dirty\n"), 0o600))

			require.Error(t, git.New().Pull(context.Background(), clone, mode))

			requireUsersChangesKept(t, clone)
		})
	}
}
