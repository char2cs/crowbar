package branchreview_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/branchreview"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

// newDivergedWorkspaceFixture builds a REAL git repository — not a mocked git
// engine — because the property under test is what resolveDiffRef computes
// against actual git plumbing; a stubbed MergeBaseFn would only assert this
// package's own arithmetic back at itself.
//
// The base branch advances AFTER the child forks, and the child is then
// rebased onto that new tip. That is the exact scenario a frozen ForkPointSha
// gets wrong: it stays pinned to the original fork commit while the live
// merge base moves to the base branch's new tip. Without the rebase step the
// two answers would coincide by accident (an un-rebased child's merge base
// with an advanced parent is still the original fork commit), so the rebase
// is what actually forces the two implementations to disagree.
func newDivergedWorkspaceFixture(
	t *testing.T,
) (branchreview.Usecase, domain.Workspace, string) {
	t.Helper()

	repoPath := testInitRepo(t)
	baseBranch := trimNewline(testGitRun(t, repoPath, "rev-parse", "--abbrev-ref", "HEAD"))
	testCommitInWorktree(t, repoPath, "base.txt", "base v1\n", "base: seed")

	childPath := filepath.Join(t.TempDir(), "child-wt")
	testGitRun(t, repoPath, "worktree", "add", "-b", "feature/child", childPath)
	staleForkSha := trimNewline(testGitRun(t, repoPath, "rev-parse", "HEAD"))
	testCommitInWorktree(t, childPath, "child.txt", "child change\n", "child: add file")

	testCommitInWorktree(t, repoPath, "base.txt", "base v2\n", "base: advance after fork")
	testGitRun(t, childPath, "rebase", baseBranch)

	wantRef := trimNewline(testGitRun(t, repoPath, "rev-parse", baseBranch))

	parent := domain.Workspace{ID: "parent-ws", Branch: baseBranch}
	child := domain.Workspace{
		ID:           "child-ws",
		RepoID:       "repo1",
		Branch:       "feature/child",
		WorktreePath: childPath,
		ParentID:     "parent-ws",
		ForkPointSha: staleForkSha,
	}
	wsMock := &mockWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			switch id {
			case child.ID:
				return child, nil
			case parent.ID:
				return parent, nil
			}
			return domain.Workspace{}, apperr.ErrNotFound
		},
	}

	uc := branchreview.New(wsMock, noopThreads(), mocks.NewRepositoryStore(), enginegit.New(), fixedNow)
	return uc, child, wantRef
}

// TestGetScope_ReturnsTheRefTheReviewDiffsAgainst is the divergence guard, and
// it is the reason this fixture exists: the ref a review reports must be the
// LIVE merge base, never the frozen ForkPointSha the workspace row still
// carries. The two agree for an un-rebased branch, which is why the fixture goes
// to the trouble of advancing the base and rebasing the child — that is what
// makes a shortcut to the recorded fork point observable at all.
//
// It asks GetScope, which is the only method that answers this question now:
// GetBase was exported for the agent surface, replaced there by GetScope, and
// then deleted with no production caller left. The guard outlived it because the
// property is about resolveDiffRef, which both went through.
func TestGetScope_ReturnsTheRefTheReviewDiffsAgainst(
	t *testing.T,
) {
	uc, ws, wantRef := newDivergedWorkspaceFixture(t)

	scope, err := uc.GetScope(context.Background(), ws)
	require.NoError(t, err)
	require.Equal(t, wantRef, scope.Base)
	require.NotEqual(t, ws.Branch, scope.Base,
		"the review must report the merge base, not the bare branch name")
	require.NotEqual(t, ws.ForkPointSha, scope.Base,
		"the review must not shortcut to the frozen fork point once the base branch has advanced past it")
}

// The not-found half of the deleted GetBase's coverage, kept on the method that
// still performs that lookup. GetScope cannot carry it: it takes the caller's
// ALREADY-RESOLVED workspace, so there is no id for it to fail to resolve —
// which is precisely why GetFiles is where an unknown workspace still has to
// come back as a not-found rather than as a git failure.
func TestGetFiles_UnknownWorkspaceIsNotFound(
	t *testing.T,
) {
	uc, _, _ := newDivergedWorkspaceFixture(t)

	_, err := uc.GetFiles(context.Background(), "nope", "")
	require.Error(t, err)
	require.ErrorIs(t, err, apperr.ErrNotFound)
}

// --- real-git test helpers (testing.T variants of the *_bench helpers above) ---

func testInitRepo(
	t *testing.T,
) string {
	t.Helper()
	dir := t.TempDir()
	testGitRun(t, dir, "init")
	testGitRun(t, dir, "config", "user.email", "t@t")
	testGitRun(t, dir, "config", "user.name", "t")
	testGitRun(t, dir, "commit", "--allow-empty", "-m", "init")
	return dir
}

func testGitRun(
	t *testing.T,
	dir string,
	args ...string,
) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(
		cmd.Environ(),
		"GIT_AUTHOR_NAME=t",
		"GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t",
		"GIT_COMMITTER_EMAIL=t@t",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return string(out)
}

func testCommitInWorktree(
	t *testing.T,
	worktree string,
	file string,
	content string,
	msg string,
) {
	t.Helper()
	path := filepath.Join(worktree, file)
	cmd := exec.Command("sh", "-c", "printf '%s' "+shellQuote(content)+" > "+shellQuote(path))
	require.NoError(t, cmd.Run())
	testGitRun(t, worktree, "add", file)
	testGitRun(t, worktree, "commit", "-m", msg)
}
