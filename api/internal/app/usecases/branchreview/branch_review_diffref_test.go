package branchreview_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/branchreview"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

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
		Provisioning: domain.WorkspaceProvisioned,
	}
	wsMock := &mockWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			switch id {
			case child.ID:
				return child, nil
			case parent.ID:
				return parent, nil
			}
			return domain.Workspace{}, problem.ErrNotFound
		},
	}

	uc := branchreview.New(wsMock, noopThreads(), mocks.NewRepositoryStore(), enginegit.New(), fixedNow)
	return uc, child, wantRef
}

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

func TestGetFiles_UnknownWorkspaceIsNotFound(
	t *testing.T,
) {
	uc, _, _ := newDivergedWorkspaceFixture(t)

	_, err := uc.GetFiles(context.Background(), "nope", "")
	require.Error(t, err)
	require.ErrorIs(t, err, problem.ErrNotFound)
}

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
