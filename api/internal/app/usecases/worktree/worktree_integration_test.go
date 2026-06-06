package worktree_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
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
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Workspace]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	workspaces, err := workspace.New(ax, db, func(domain.Workspace) {})
	require.NoError(t, err)

	repos, err := storesqlite.NewFromDB[domain.Repository, string](db)
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
	}))

	prov := &stubProvider{}
	uc := worktree.New(
		workspaces,
		enginegit.New(),
		prov,
		repos,
		func() time.Time { return time.Unix(1000, 0).UTC() },
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
		Branch:       branch,
		ParentID:     parentID,
		ParentBranch: parentBranch,
	}
	child, err := h.uc.CreateChild(context.Background(), in)
	require.NoError(t, err)
	return child
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
	require.True(t, locked.Locked)
	h.provider.protected = nil

	descendant := h.createChild(t, "feature/desc", locked.ID, locked.Branch)

	require.NoError(t, h.uc.DeleteCascade(ctx, root.ID))

	assert.True(t, dirExists(t, locked.WorktreePath), "locked child worktree survives")
	assert.True(t, branchExists(t, h.repoPath, locked.Branch), "locked child branch survives")

	assert.False(t, dirExists(t, root.WorktreePath), "unlocked root worktree gone")
	assert.False(t, branchExists(t, h.repoPath, root.Branch), "unlocked root branch gone")
	assert.False(t, dirExists(t, descendant.WorktreePath), "unlocked descendant worktree gone")
	assert.False(t, branchExists(t, h.repoPath, descendant.Branch), "unlocked descendant branch gone")

	all, err := h.workspaces.List(ctx)
	require.NoError(t, err)
	ids := map[string]bool{}
	for _, ws := range all {
		ids[ws.ID] = true
	}
	assert.True(t, ids[locked.ID], "locked row remains")
	assert.False(t, ids[root.ID], "root row removed")
	assert.False(t, ids[descendant.ID], "descendant row removed")
}

func TestIntegration_MergeConflictSetsPendingMerge(t *testing.T) {
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
	require.NotNil(t, reloaded.PendingMerge)
	assert.Equal(t, gitdomain.MergeStrategyMerge, reloaded.PendingMerge.Strategy)
	assert.Equal(t, h.parentID, reloaded.PendingMerge.TargetParentID)

	require.NoError(t, enginegit.New().OperationAbort(ctx, h.repoPath))
	cleared, err := h.workspaces.ClearPendingMerge(ctx, child.ID)
	require.NoError(t, err)
	assert.Nil(t, cleared.PendingMerge)

	status := gitRun(t, h.repoPath, "status", "--porcelain")
	assert.Empty(t, trimNewline(status), "abort rolls the parent worktree back clean")
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
