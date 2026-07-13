package worktree_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
	engineprovider "github.com/char2cs/crowbar/api/internal/engine/provider"
	"github.com/char2cs/crowbar/api/internal/engine/provider/poll"
)

// stubProvider is a controllable engineprovider.Engine for the integration
// matrix. Only ProtectedBranches participates in the usecase under test; the
// remaining methods satisfy the interface and are unused.
type stubProvider struct {
	protected []string
}

func (s *stubProvider) Capability(
	ctx context.Context,
	repoPath string,
) (engineprovider.ProviderCapability, error) {
	return engineprovider.ProviderCapability{Kind: "none", Enabled: false}, nil
}

func (s *stubProvider) ProtectedBranches(
	ctx context.Context,
	repoPath string,
) ([]string, error) {
	return s.protected, nil
}

func (s *stubProvider) PollOnView(
	ctx context.Context,
	wsID string,
	repoPath string,
	branch string,
) (engineprovider.ProviderState, error) {
	return engineprovider.ProviderState{}, nil
}

func (s *stubProvider) StartBackgroundSweep(
	ctx context.Context,
	workspacesFn func() []poll.SweepTarget,
	onStateChange func(wsID string, state engineprovider.ProviderState),
) {
}

func (s *stubProvider) OwnerAvatarURL(ctx context.Context, repoPath string) (string, error) {
	return "", nil
}

// realHarness bundles the wired-up usecase plus the handles a scenario needs to
// drive REAL git and inspect REAL read-model rows.
type realHarness struct {
	uc         worktree.Usecase
	workspaces workspace.Workspace
	provider   *stubProvider
	repoPath   string
	baseBranch string
	parentID   string
	repoID     string
	projectID  string

	// quiesce blocks until the workspace repo's asynx has drained its dispatch
	// queue and run every projection handler (WaitPublish). Workspace mutations are
	// Sends, so the read model lands asynchronously: this is the read-your-writes
	// barrier, and the ONLY correct thing to block on before asserting over the
	// projection. No polling, no deadline.
	quiesce func()
}

func gitRun(
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

func writeFile(
	t *testing.T,
	path string,
	content string,
) {
	t.Helper()
	cmd := exec.Command("sh", "-c", "printf '%s' "+shellQuote(content)+" > "+shellQuote(path))
	require.NoError(t, cmd.Run())
}

func shellQuote(
	s string,
) string {
	out := "'"
	for _, r := range s {
		if r == '\'' {
			out += `'\''`
			continue
		}
		out += string(r)
	}
	return out + "'"
}

func newRealUsecase(
	t *testing.T,
) *realHarness {
	t.Helper()
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	workspaces, quiesce := newWorkspaceRepo(t, adapters)

	repos, err := storesqlite.NewFromDB[domain.Repository, string](adapters.GlobalView())
	require.NoError(t, err)

	repoPath := initRepo(t)
	repoID := "r1"
	projectID := "p1"
	baseBranch := branchName(t, repoPath)

	require.NoError(t, repos.Save(context.Background(), domain.Repository{
		ID:        repoID,
		ProjectID: projectID,
		Name:      "repo",
		Path:      repoPath,
		RemoteURL: "https://github.com/test/integration-repo.git",
	}))

	prov := &stubProvider{}
	uc := worktree.New(
		workspaces,
		enginegit.New(),
		prov,
		repos,
		func() time.Time { return time.Unix(1000, 0).UTC() },
		func() (string, error) { return t.TempDir(), nil },
	)

	parentID := "w-parent"
	_, err = workspaces.Create(context.Background(), workspace.CreateInput{
		ID:           parentID,
		RepoID:       repoID,
		ProjectID:    projectID,
		Branch:       baseBranch,
		WorktreePath: repoPath,
	}, time.Unix(1000, 0).UTC())
	require.NoError(t, err)

	return &realHarness{
		uc:         uc,
		workspaces: workspaces,
		provider:   prov,
		repoPath:   repoPath,
		baseBranch: baseBranch,
		parentID:   parentID,
		repoID:     repoID,
		projectID:  projectID,
		quiesce:    quiesce,
	}
}

func initRepo(
	t *testing.T,
) string {
	t.Helper()
	dir := t.TempDir()
	gitRun(t, dir, "init")
	gitRun(t, dir, "config", "user.email", "t@t")
	gitRun(t, dir, "config", "user.name", "t")
	gitRun(t, dir, "commit", "--allow-empty", "-m", "init")
	// Wire a bare `origin` remote so the usecase's RemoteBranchExists check has
	// something to query. Feature branches are never pushed here, so every
	// CreateChild in the integration matrix takes the create-local path.
	bare := filepath.Join(t.TempDir(), "origin.git")
	gitRun(t, t.TempDir(), "init", "--bare", bare)
	gitRun(t, dir, "remote", "add", "origin", bare)
	return dir
}

func branchName(
	t *testing.T,
	dir string,
) string {
	t.Helper()
	out := gitRun(t, dir, "rev-parse", "--abbrev-ref", "HEAD")
	return trimNewline(out)
}

func trimNewline(
	s string,
) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

func revParse(
	t *testing.T,
	dir string,
	rev string,
) string {
	t.Helper()
	return trimNewline(gitRun(t, dir, "rev-parse", rev))
}

func (h *realHarness) createChild(
	t *testing.T,
	branch string,
	parentID string,
	parentBranch string,
) domain.Workspace {
	t.Helper()
	in := worktree.CreateChildInput{
		RepoID:       h.repoID,
		ProjectID:    h.projectID,
		RepoPath:     h.repoPath,
		RemoteURL:    "https://github.com/test/integration-repo.git",
		Branch:       branch,
		ParentID:     parentID,
		ParentBranch: parentBranch,
	}
	child, err := h.uc.CreateChild(context.Background(), in)
	require.NoError(t, err)

	// The store projection is async (Send, not SendWait). Block on the asynx
	// publish barrier — dispatch queue drained, every projection handler run — so
	// the new child is guaranteed visible in the read model and a later
	// DeleteCascade (which lists to build the tree) sees the full tree. Polling on
	// a 2s deadline used to stand in for this; the barrier is the real signal.
	h.quiesce()

	rows, listErr := h.workspaces.List(context.Background())
	require.NoError(t, listErr)
	require.Contains(t, workspaceIDs(rows), child.ID,
		"the child must be in the read model once the projection has quiesced")
	return child
}

// workspaceIDs projects a workspace slice down to its IDs for set assertions.
func workspaceIDs(
	rows []domain.Workspace,
) []string {
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	return ids
}

func commitInWorktree(
	t *testing.T,
	worktree string,
	file string,
	content string,
	msg string,
) {
	t.Helper()
	writeFile(t, filepath.Join(worktree, file), content)
	gitRun(t, worktree, "add", file)
	gitRun(t, worktree, "commit", "-m", msg)
}

func TestIntegration_MergeStrategyAdvancesParent(t *testing.T) {
	h := newRealUsecase(t)
	ctx := context.Background()

	child := h.createChild(t, "feature/x", h.parentID, h.baseBranch)
	commitInWorktree(t, child.WorktreePath, "a.txt", "child change\n", "child work")
	childTip := revParse(t, child.WorktreePath, "HEAD")

	res, err := h.uc.MergeIntoParent(ctx, child.ID, gitdomain.MergeStrategyMerge)
	require.NoError(t, err)
	assert.False(t, res.ConflictsPending)

	parentTip := revParse(t, h.repoPath, "HEAD")
	assert.Equal(t, parentTip, res.ParentTipSha)

	contains := gitRun(t, h.repoPath, "merge-base", "--is-ancestor", childTip, "HEAD")
	assert.Empty(t, contains)

	reloaded, err := h.workspaces.Get(ctx, child.ID)
	require.NoError(t, err)
	assert.Equal(t, parentTip, reloaded.ForkPointSha)
}

// TestIntegration_MergeResyncsParentReadModel proves FIX 3: after a merge the
// PARENT worktree gains commits, so its read-model diff summary must be resynced
// rather than going stale. The parent here is itself a child (so it has a real
// fork point against the root), making the +N diff stats observable.
func TestIntegration_MergeResyncsParentReadModel(t *testing.T) {
	h := newRealUsecase(t)
	ctx := context.Background()

	mid := h.createChild(t, "feature/mid", h.parentID, h.baseBranch)
	leaf := h.createChild(t, "feature/leaf", mid.ID, mid.Branch)
	commitInWorktree(t, leaf.WorktreePath, "leaf.txt", "leaf change\nsecond\n", "leaf work")

	midBefore, err := h.workspaces.Get(ctx, mid.ID)
	require.NoError(t, err)
	require.Equal(t, 0, midBefore.Added, "parent has no diff before the merge")

	res, err := h.uc.MergeIntoParent(ctx, leaf.ID, gitdomain.MergeStrategyMerge)
	require.NoError(t, err)
	require.False(t, res.ConflictsPending)

	midAfter, err := h.workspaces.Get(ctx, mid.ID)
	require.NoError(t, err)
	assert.Greater(t, midAfter.Added, 0, "parent read-model diff stats reflect the merged commit")
}

func TestIntegration_SquashStrategyAdvancesParent(t *testing.T) {
	h := newRealUsecase(t)
	ctx := context.Background()

	baseParentTip := revParse(t, h.repoPath, "HEAD")
	child := h.createChild(t, "feature/x", h.parentID, h.baseBranch)
	commitInWorktree(t, child.WorktreePath, "a.txt", "child change\n", "child work")

	res, err := h.uc.MergeIntoParent(ctx, child.ID, gitdomain.MergeStrategySquash)
	require.NoError(t, err)
	assert.False(t, res.ConflictsPending)

	parentTip := revParse(t, h.repoPath, "HEAD")
	assert.Equal(t, parentTip, res.ParentTipSha)
	assert.NotEqual(t, baseParentTip, parentTip)

	assert.Equal(t, baseParentTip, revParse(t, h.repoPath, "HEAD~1"))
	body := gitRun(t, h.repoPath, "show", "--name-only", "--format=", "HEAD")
	assert.Contains(t, body, "a.txt")

	reloaded, err := h.workspaces.Get(ctx, child.ID)
	require.NoError(t, err)
	assert.Equal(t, parentTip, reloaded.ForkPointSha)
}

func TestIntegration_RebaseStrategyReplaysChildThenFFMerges(t *testing.T) {
	h := newRealUsecase(t)
	ctx := context.Background()

	child := h.createChild(t, "feature/x", h.parentID, h.baseBranch)
	commitInWorktree(t, child.WorktreePath, "a.txt", "child change\n", "child work")
	childTipBefore := revParse(t, child.WorktreePath, "HEAD")

	commitInWorktree(t, h.repoPath, "p.txt", "parent change\n", "parent work")
	parentTipBefore := revParse(t, h.repoPath, "HEAD")

	res, err := h.uc.MergeIntoParent(ctx, child.ID, gitdomain.MergeStrategyRebase)
	require.NoError(t, err)
	assert.False(t, res.ConflictsPending)

	parentTip := revParse(t, h.repoPath, "HEAD")
	assert.Equal(t, parentTip, res.ParentTipSha)

	childTipAfter := revParse(t, child.WorktreePath, "HEAD")
	assert.NotEqual(t, childTipBefore, childTipAfter, "rebase must rewrite child SHA")
	assert.Equal(t, parentTip, childTipAfter, "ff-merge means parent == child tip")

	assert.Empty(t, gitRun(t, h.repoPath, "merge-base", "--is-ancestor", parentTipBefore, "HEAD"))

	reloaded, err := h.workspaces.Get(ctx, child.ID)
	require.NoError(t, err)
	assert.Equal(t, parentTip, reloaded.ForkPointSha)
}

func TestIntegration_ReparentLeafReplaysOnlyChildCommits(t *testing.T) {
	h := newRealUsecase(t)
	ctx := context.Background()

	parentB := h.createChild(t, "feature/parent-b", h.parentID, h.baseBranch)
	commitInWorktree(t, parentB.WorktreePath, "b.txt", "parent-b only\n", "parent-b unique")
	parentBTip := revParse(t, parentB.WorktreePath, "HEAD")

	child := h.createChild(t, "feature/x", h.parentID, h.baseBranch)
	commitInWorktree(t, child.WorktreePath, "x.txt", "child only\n", "child unique")

	_, err := h.uc.Reparent(ctx, child.ID, parentB.ID)
	require.NoError(t, err)

	assert.Empty(t, gitRun(t, child.WorktreePath, "merge-base", "--is-ancestor", parentBTip, "HEAD"),
		"child must now sit atop parent-b's tip")

	assert.True(t, fileExists(t, child.WorktreePath, "x.txt"),
		"child's own commit replays onto the new parent")
	assert.True(t, fileExists(t, child.WorktreePath, "b.txt"),
		"new parent's history is now part of the child branch")

	subjects := gitRun(t, child.WorktreePath, "log", "--format=%s", parentBTip+"..HEAD")
	assert.Equal(t, "child unique", trimNewline(subjects),
		"only the child's own commit is replayed atop parent-b; old-parent history is not duplicated")

	reloaded, err := h.workspaces.Get(ctx, child.ID)
	require.NoError(t, err)
	assert.Equal(t, parentB.ID, reloaded.ParentID)
	assert.Equal(t, parentBTip, reloaded.ForkPointSha)
}

func TestIntegration_ReparentWithChildrenRejected(t *testing.T) {
	h := newRealUsecase(t)
	ctx := context.Background()

	parentB := h.createChild(t, "feature/parent-b", h.parentID, h.baseBranch)
	child := h.createChild(t, "feature/x", h.parentID, h.baseBranch)
	commitInWorktree(t, child.WorktreePath, "x.txt", "child\n", "child work")
	childTipBefore := revParse(t, child.WorktreePath, "HEAD")

	_ = h.createChild(t, "feature/grandchild", child.ID, child.Branch)

	_, err := h.uc.Reparent(ctx, child.ID, parentB.ID)
	require.ErrorIs(t, err, worktree.ErrChildHasChildren)

	assert.Equal(t, childTipBefore, revParse(t, child.WorktreePath, "HEAD"),
		"rejected reparent must not mutate git")

	reloaded, err := h.workspaces.Get(ctx, child.ID)
	require.NoError(t, err)
	assert.Equal(t, h.parentID, reloaded.ParentID)
}

func TestIntegration_DeleteCascadeSkipsLockedChild(t *testing.T) {
	h := newRealUsecase(t)
	ctx := context.Background()

	root := h.createChild(t, "feature/root", h.parentID, h.baseBranch)

	h.provider.protected = []string{"feature/locked"}
	locked := h.createChild(t, "feature/locked", root.ID, root.Branch)
	require.Equal(t, domain.WorkspaceStatusLocked, locked.Status)
	h.provider.protected = nil

	descendant := h.createChild(t, "feature/desc", locked.ID, locked.Branch)

	require.NoError(t, h.uc.DeleteCascade(ctx, root.ID))

	assert.True(t, dirExists(t, locked.WorktreePath), "locked child worktree survives")
	assert.True(t, branchExists(t, h.repoPath, locked.Branch), "locked child branch survives")

	assert.False(t, dirExists(t, root.WorktreePath), "unlocked root worktree gone")
	assert.False(t, branchExists(t, h.repoPath, root.Branch), "unlocked root branch gone")
	assert.False(t, dirExists(t, descendant.WorktreePath), "unlocked descendant worktree gone")
	assert.False(t, branchExists(t, h.repoPath, descendant.Branch), "unlocked descendant branch gone")

	// Task 7: workspace Delete is a pure Send that tombstones the row (Status =
	// deleted); the async purge reactor that Forgets the row is Task 8. So the
	// deleted nodes linger in the read model as deleted-status tombstones (their
	// worktrees/branches are already gone above), while the locked child stays
	// active. The store projection is async — block on the asynx publish barrier
	// (WaitPublish) and then assert ONCE. The old polling loop could not tell "the
	// projection has not landed yet" from "the projection landed wrong": whichever
	// came first within 2s decided the verdict.
	h.quiesce()

	all, err := h.workspaces.List(ctx)
	require.NoError(t, err)
	status := map[string]domain.WorkspaceStatus{}
	for _, ws := range all {
		status[ws.ID] = ws.Status
	}

	require.Contains(t, status, locked.ID, "the locked child must survive in the read model")
	assert.NotEqual(t, domain.WorkspaceStatusDeleted, status[locked.ID],
		"the locked child must not be tombstoned")
	assert.Equal(t, domain.WorkspaceStatusDeleted, status[root.ID],
		"the unlocked root must be tombstoned")
	assert.Equal(t, domain.WorkspaceStatusDeleted, status[descendant.ID],
		"the unlocked descendant must be tombstoned")
}

// TestIntegration_MergeConflictSetsPRConflicts proves the try-then-warn model
// (H6/H7 guard): a conflicting merge-into-parent flags the CHILD pr-conflicts
// AND leaves the PARENT worktree clean — the in-progress merge is aborted
// automatically, so no manual abort is needed and the parent is never stuck.
func TestIntegration_MergeConflictSetsPRConflicts(t *testing.T) {
	h := newRealUsecase(t)
	ctx := context.Background()

	child := h.createChild(t, "feature/x", h.parentID, h.baseBranch)
	commitInWorktree(t, child.WorktreePath, "c.txt", "child line\n", "child edit")

	commitInWorktree(t, h.repoPath, "c.txt", "parent line\n", "parent edit")

	res, err := h.uc.MergeIntoParent(ctx, child.ID, gitdomain.MergeStrategyMerge)
	require.NoError(t, err)
	assert.True(t, res.ConflictsPending)
	assert.Empty(t, res.ParentTipSha)

	reloaded, err := h.workspaces.Get(ctx, child.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.WorkspaceStatusPRConflicts, reloaded.Status,
		"a local merge conflict surfaces as Status=pr-conflicts")

	// The parent is ALREADY clean: the conflicting merge was aborted internally,
	// so there is no in-progress op to abort and no conflict markers remain.
	require.ErrorContains(t, enginegit.New().OperationAbort(ctx, h.repoPath),
		"no in-progress operation",
		"the parent merge was already aborted; nothing left to abort")
	status := gitRun(t, h.repoPath, "status", "--porcelain")
	assert.Empty(t, trimNewline(status), "the parent worktree is clean after a conflicting merge")
}

// TestRegression_SquashMergeConflictDoesNotBrickParent encodes the core H6
// guarantee: a conflicting `git merge --squash` writes AUTO_MERGE but neither
// MERGE_HEAD nor SQUASH_HEAD, so a naive in-progress detector would miss it and
// leave the parent's conflicted index permanently blocking commits/merges. The
// fix detects AUTO_MERGE as a squash op and aborts it; this test proves the
// parent worktree is clean afterward and a subsequent commit is NOT blocked.
func TestRegression_SquashMergeConflictDoesNotBrickParent(t *testing.T) {
	h := newRealUsecase(t)
	ctx := context.Background()

	child := h.createChild(t, "feature/sq", h.parentID, h.baseBranch)
	commitInWorktree(t, child.WorktreePath, "c.txt", "child line\n", "child edit")
	commitInWorktree(t, h.repoPath, "c.txt", "parent line\n", "parent edit")

	res, err := h.uc.MergeIntoParent(ctx, child.ID, gitdomain.MergeStrategySquash)
	require.NoError(t, err)
	assert.True(t, res.ConflictsPending)

	// Parent worktree is clean: no AUTO_MERGE, no unmerged paths.
	gitDir := trimNewline(gitRun(t, h.repoPath, "rev-parse", "--git-dir"))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(h.repoPath, gitDir)
	}
	assert.False(t, fileExists(t, gitDir, "AUTO_MERGE"), "AUTO_MERGE marker must be gone")
	assert.Empty(t, trimNewline(gitRun(t, h.repoPath, "status", "--porcelain")),
		"parent must have no unmerged paths after the conflicting squash")

	// The parent is NOT bricked: a fresh commit on it succeeds.
	commitInWorktree(t, h.repoPath, "after.txt", "after\n", "post-conflict commit")
	assert.True(t, fileExists(t, h.repoPath, "after.txt"),
		"a subsequent commit on the parent must succeed (not blocked by a stuck index)")
}

func dirExists(
	t *testing.T,
	path string,
) bool {
	t.Helper()
	err := exec.Command("test", "-d", path).Run()
	return err == nil
}

func fileExists(
	t *testing.T,
	dir string,
	file string,
) bool {
	t.Helper()
	err := exec.Command("test", "-f", filepath.Join(dir, file)).Run()
	return err == nil
}

func branchExists(
	t *testing.T,
	repoPath string,
	branch string,
) bool {
	t.Helper()
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	cmd.Dir = repoPath
	return cmd.Run() == nil
}
