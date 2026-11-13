package git_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git"
	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
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
// git identity does; the user's global identity must not fill the gap.
func withoutIdentity(t *testing.T, repo string) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
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

// The user's state each case starts from on main, which p1 diverged from: each
// step writes a file ("write", name, content) or runs git.
var userStates = map[string][][]string{
	"staged":               {{"write", "mine.txt", "mine\n"}, {"add", "mine.txt"}},
	"dirty":                {{"write", "base.txt", "dirty\n"}},
	"untracked":            {{"write", "p1.txt", "mine\n"}},
	"intent-to-add":        {{"write", "ita.txt", "mine\n"}, {"add", "-N", "ita.txt"}},
	"staged+dirty":         {{"write", "main.txt", "s1\n"}, {"add", "."}, {"write", "main.txt", "s2\n"}},
	"stash+resolved merge": {{"write", "base.txt", "stashed\n"}, {"stash", "push", "-q"}, {"merge", "--no-ff", "--no-commit", "p2"}},
	"dirty submodule": {
		{"-c", "protocol.file.allow=always", "submodule", "add", "-q", "SUB", "sub"},
		{"commit", "-qm", "sub"},
		{"write", "sub/s.txt", "dirty\n"},
	},
}

func setUpState(t *testing.T, repo string, steps [][]string) {
	t.Helper()
	for _, step := range steps {
		if step[0] == "write" {
			require.NoError(t, os.WriteFile(filepath.Join(repo, step[1]), []byte(step[2]), 0o600))
			continue
		}
		if slices.Contains(step, "SUB") {
			sub := initRepo(t)
			makeCommit(t, sub, "s.txt", "s\n", "s")
			step = slices.Replace(slices.Clone(step), slices.Index(step, "SUB"), slices.Index(step, "SUB")+1, sub)
		}
		gitRun(t, repo, step...)
	}
}

var stoppableVerbs = map[string]func(e git.Engine, repo, parent string) error{
	"merge":       func(e git.Engine, r, _ string) error { return e.Merge(context.Background(), r, "p1") },
	"mergecommit": func(e git.Engine, r, _ string) error { return e.MergeCommit(context.Background(), r, "p1") },
	"squash":      func(e git.Engine, r, _ string) error { return e.MergeSquash(context.Background(), r, "p1", "s") },
	// main fast-forwards to ahead, so git carries the user's staged work into it.
	"squash ff": func(e git.Engine, r, _ string) error { return e.MergeSquash(context.Background(), r, "ahead", "s") },
	"rebase":    func(e git.Engine, r, _ string) error { return e.Rebase(context.Background(), r, "p1") },
	"rebaseonto": func(e git.Engine, r, _ string) error {
		return e.RebaseOnto(context.Background(), r, "p1", "base", "main")
	},
	"rebase+ff": func(e git.Engine, r, p string) error {
		return e.RebaseThenFFMerge(context.Background(), r, "p1", p, "main")
	},
	"pull":        func(e git.Engine, r, _ string) error { return e.Pull(context.Background(), r, "merge") },
	"pull rebase": func(e git.Engine, r, _ string) error { return e.Pull(context.Background(), r, "rebase") },
}

// userView is everything of the user's a failed command must leave unchanged.
func userView(t *testing.T, e git.Engine, repo string) string {
	t.Helper()
	view := ""
	for _, args := range [][]string{{"status", "--porcelain=v2"}, {"stash", "list"}, {"diff", "--cached"}, {"diff"}, {"ls-files", "-o"}} {
		view += gitOutput(t, repo, args...) + "\n--\n"
	}
	op, err := e.OperationInProgress(context.Background(), repo)
	require.NoError(t, err)
	return view + "op=" + op
}

// Every stoppable verb, refused by git or by a hook over each kind of the
// user's own state, leaves that state exactly as it found it.
func TestStoppableVerbs_AFailureLeavesTheUsersStateAsFound(t *testing.T) {
	for verb, run := range stoppableVerbs {
		for state, steps := range userStates {
			for _, hook := range []string{"", "pre-commit", "pre-merge-commit"} {
				t.Run(verb+"/"+state+"/"+hook, func(t *testing.T) {
					t.Parallel()
					repo := initRepo(t)
					makeCommit(t, repo, "base.txt", "base\n", "base")
					gitRun(t, repo, "tag", "base")
					gitRun(t, repo, "branch", "p2")
					gitRun(t, repo, "checkout", "-q", "-b", "p1")
					makeCommit(t, repo, "p1.txt", "p1\n", "p1")
					gitRun(t, repo, "checkout", "-q", "p2")
					makeCommit(t, repo, "p2.txt", "p2\n", "p2")
					gitRun(t, repo, "checkout", "-q", "main")
					makeCommit(t, repo, "main.txt", "main\n", "main")
					gitRun(t, repo, "checkout", "-q", "-b", "ahead")
					makeCommit(t, repo, "ahead.txt", "ahead\n", "ahead")
					gitRun(t, repo, "checkout", "-q", "main")
					gitRun(t, repo, "branch", "-q", "--set-upstream-to=p1")
					parent := filepath.Join(t.TempDir(), "parent")
					gitRun(t, repo, "worktree", "add", "-q", "--detach", parent, "p1")
					setUpState(t, repo, steps)
					if hook != "" {
						failingHook(t, repo, hook)
					}
					e := git.New()
					before := userView(t, e, repo)

					if run(e, repo, parent) == nil {
						t.Skip("the verb succeeded over this state")
					}

					assert.Equal(t, before, userView(t, e, repo))
				})
			}
		}
	}
}

func TestMerge_NeverAbortsAMergeTheUserIsResolving(t *testing.T) {
	ctx := context.Background()
	_, wt := conflictingWorktree(t)
	e := git.New()
	require.ErrorIs(t, e.Merge(ctx, wt, "other"), git.ErrConflict)
	require.NoError(t, os.WriteFile(filepath.Join(wt, "f.txt"), []byte("resolved\n"), 0o600))
	gitRun(t, wt, "add", "f.txt")

	require.Error(t, e.Merge(ctx, wt, "other"))

	op, err := e.OperationInProgress(ctx, wt)
	require.NoError(t, err)
	assert.Equal(t, "merge", op, "the user's resolved merge is theirs to conclude")
	assert.Equal(t, "resolved", gitOutput(t, wt, "show", ":f.txt"))
}

// The request ends while git is stopped mid-rebase; the abort still runs.
func TestRebaseOnto_ARequestCancelledMidRebaseStillLeavesTheWorktreeAsFound(t *testing.T) {
	repo, wt := stackedWorktree(t)
	before := headSHA(t, wt)
	tip := gitOutput(t, repo, "rev-parse", "p2")
	base := gitOutput(t, repo, "merge-base", "p1", "child")
	withoutIdentity(t, repo)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e := git.NewWithExec(func(c context.Context, dir string, args ...string) gitexec.Result {
		r := gitexec.Git(c, dir, args...)
		if args[0] == "rebase" {
			cancel()
		}
		return r
	})

	require.Error(t, e.RebaseOnto(ctx, wt, tip, base, "child"))

	requireAtRest(t, git.New(), wt, before)
}
