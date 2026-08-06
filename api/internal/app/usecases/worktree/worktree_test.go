package worktree_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
	engineprovider "github.com/char2cs/crowbar/api/internal/engine/provider"
)

var errBoom = errors.New("boom")

// fakeWorkspace is a full workspace.Workspace double driven by function fields;
// only the fields a given test sets are exercised by the usecase under test.
type fakeWorkspace struct {
	CreateFn           func(ctx context.Context, in workspace.CreateInput, now time.Time) (domain.Workspace, error)
	GetFn              func(ctx context.Context, id string) (domain.Workspace, error)
	ListFn             func(ctx context.Context) ([]domain.Workspace, error)
	ReparentFn         func(ctx context.Context, id, parentID, forkPointSha string, now time.Time) (domain.Workspace, error)
	ResolveConflictsFn func(ctx context.Context, id string, now time.Time) (domain.Workspace, error)
	UpdateForkPointFn  func(ctx context.Context, id, forkPointSha string) (domain.Workspace, error)
	DeleteFn           func(ctx context.Context, id string) error
	SyncFn             func(ctx context.Context, in workspace.SyncInput, now time.Time) (domain.Workspace, error)
	ProvisionInPlaceFn func(id, worktreePath, forkPointSha string) (domain.Workspace, error)
	ClearBranchFn      func(id string) (domain.Workspace, error)
	RenameBranchFn     func(id, branch string) (domain.Workspace, error)
}

func (f *fakeWorkspace) Create(
	ctx context.Context,
	in workspace.CreateInput,
	now time.Time,
) (domain.Workspace, error) {
	return f.CreateFn(ctx, in, now)
}

func (f *fakeWorkspace) Get(
	ctx context.Context,
	id string,
) (domain.Workspace, error) {
	return f.GetFn(ctx, id)
}

func (f *fakeWorkspace) List(
	ctx context.Context,
) ([]domain.Workspace, error) {
	if f.ListFn != nil {
		return f.ListFn(ctx)
	}
	return nil, nil
}

func (f *fakeWorkspace) Reparent(
	ctx context.Context,
	id string,
	parentID string,
	forkPointSha string,
	now time.Time,
) (domain.Workspace, error) {
	return f.ReparentFn(ctx, id, parentID, forkPointSha, now)
}

func (f *fakeWorkspace) ResolveConflicts(
	ctx context.Context,
	id string,
	now time.Time,
) (domain.Workspace, error) {
	if f.ResolveConflictsFn != nil {
		return f.ResolveConflictsFn(ctx, id, now)
	}
	return domain.Workspace{ID: id}, nil
}

func (f *fakeWorkspace) UpdateForkPoint(
	ctx context.Context,
	id string,
	forkPointSha string,
) (domain.Workspace, error) {
	return f.UpdateForkPointFn(ctx, id, forkPointSha)
}

func (f *fakeWorkspace) ProvisionInPlace(
	_ context.Context,
	id string,
	worktreePath string,
	forkPointSha string,
) (domain.Workspace, error) {
	if f.ProvisionInPlaceFn != nil {
		return f.ProvisionInPlaceFn(id, worktreePath, forkPointSha)
	}
	return domain.Workspace{ID: id, WorktreePath: worktreePath, ForkPointSha: forkPointSha}, nil
}

func (f *fakeWorkspace) ClearBranch(
	_ context.Context,
	id string,
) (domain.Workspace, error) {
	if f.ClearBranchFn != nil {
		return f.ClearBranchFn(id)
	}
	return domain.Workspace{ID: id}, nil
}

func (f *fakeWorkspace) Relocate(
	_ context.Context,
	id string,
	worktreePath string,
) (domain.Workspace, error) {
	return domain.Workspace{ID: id, WorktreePath: worktreePath}, nil
}

func (f *fakeWorkspace) RenameBranch(
	_ context.Context,
	id string,
	branch string,
) (domain.Workspace, error) {
	if f.RenameBranchFn != nil {
		return f.RenameBranchFn(id, branch)
	}
	return domain.Workspace{ID: id, Branch: branch}, nil
}

func (f *fakeWorkspace) Delete(
	ctx context.Context,
	id string,
) error {
	return f.DeleteFn(ctx, id)
}

func (f *fakeWorkspace) SyncWorkingTreeState(
	ctx context.Context,
	in workspace.SyncInput,
	now time.Time,
) (domain.Workspace, error) {
	if f.SyncFn != nil {
		return f.SyncFn(ctx, in, now)
	}
	return domain.Workspace{}, nil
}

func (f *fakeWorkspace) SyncProviderState(
	_ context.Context,
	_ workspace.ProviderInput,
	_ time.Time,
) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

func (f *fakeWorkspace) SetMergeStrategy(
	_ context.Context,
	_ string,
	_ gitdomain.MergeStrategy,
) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

func (f *fakeWorkspace) TouchActivity(
	_ context.Context,
	_ string,
	_ time.Time,
) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

func (f *fakeWorkspace) SetParentFromPR(_ context.Context, _, _ string) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

func (f *fakeWorkspace) SetPlacement(_ context.Context, id, folderID string, order int) (domain.Workspace, error) {
	return domain.Workspace{ID: id, FolderID: folderID, Order: order}, nil
}

func (f *fakeWorkspace) SetProject(_ context.Context, id, projectID string) (domain.Workspace, error) {
	return domain.Workspace{ID: id, ProjectID: projectID}, nil
}

func (f *fakeWorkspace) SetLastError(_ context.Context, _, _ string) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

func (f *fakeWorkspace) GetHomeForProject(_ context.Context, _ string) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

func (f *fakeWorkspace) CreateHome(_ context.Context, _, _ string, _ time.Time) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

func (f *fakeWorkspace) ListInRepo(_ context.Context, _, _ string) ([]domain.Workspace, error) {
	return nil, nil
}

var _ workspace.Workspace = (*fakeWorkspace)(nil)

// --- fakes ---

type gitCall struct {
	op   string
	args []string
}

type fakeGit struct {
	enginegit.Engine
	calls                  []gitCall
	addStartSha            string
	addErr                 error
	revParseSha            string
	revParseErr            error
	mergeBaseSha           string
	mergeBaseErr           error
	mergeErr               error
	squashErr              error
	rebaseErr              error
	ffErr                  error
	rebaseFFErr            error
	rebaseOnto             error
	operationAbortErr      error
	removeErr              error
	deleteErr              error
	remoteExists           bool
	remoteExistsByBranch   map[string]bool // overrides remoteExists per branch when non-nil
	remoteExistsErr        error
	trackingExists         bool
	trackingExistsByBranch map[string]bool // overrides trackingExists per branch when non-nil
	trackingExistsErr      error
	setUpstreamErr         error
	fetchRefErr            error
	fastForwardBranchErr   error
	worktreeAddErr         error

	summaryAdded        int
	summaryDeleted      int
	summaryHasCommits   bool
	summaryHasConflicts bool
	summaryErr          error
	pruneErr            error

	opInProgress    string
	opInProgressErr error

	worktrees       []enginegit.WorktreeEntry
	worktreeListErr error
	detachErr       error
	checkoutErr     error
	// addConflictUntilDetach, when set, makes WorktreeAdd/WorktreeAddBranch fail
	// with this error until DetachWorktree is called — simulating git's
	// "branch already checked out by the main worktree" conflict that a detach
	// clears.
	addConflictUntilDetach error
	detachCalled           bool

	// branchNow is the fake repo's ACTUAL branch. It advances only when a
	// RenameBranch call SUCCEEDS, so a test can tell "git renamed it" from "git
	// was asked to and the call died".
	branchNow string
	// onBranchRenamed fires once, immediately after a successful RenameBranch.
	// It is the client-disconnect seam: the rename tests use it to cancel the
	// request context at the exact instant `git branch -m` has landed.
	onBranchRenamed func()
}

func (f *fakeGit) WorktreeList(
	_ context.Context,
	repoPath string,
) ([]enginegit.WorktreeEntry, error) {
	f.record("WorktreeList", repoPath)
	if f.detachCalled {
		// After a detach the holder is freed: the branch is no longer checked out
		// anywhere, so the next resolution classifies it Free.
		return nil, f.worktreeListErr
	}
	return f.worktrees, f.worktreeListErr
}

func (f *fakeGit) DetachWorktree(
	_ context.Context,
	worktreePath string,
) error {
	f.record("DetachWorktree", worktreePath)
	f.detachCalled = true
	return f.detachErr
}

func (f *fakeGit) CheckoutBranch(
	_ context.Context,
	worktreePath string,
	branch string,
) error {
	f.record("CheckoutBranch", worktreePath, branch)
	return f.checkoutErr
}

func (f *fakeGit) OperationInProgress(
	_ context.Context,
	repoPath string,
) (string, error) {
	f.record("OperationInProgress", repoPath)
	return f.opInProgress, f.opInProgressErr
}

func (f *fakeGit) record(
	op string,
	args ...string,
) {
	f.calls = append(f.calls, gitCall{op: op, args: args})
}

func (f *fakeGit) ops() []string {
	out := make([]string, 0, len(f.calls))
	for _, c := range f.calls {
		out = append(out, c.op)
	}
	return out
}

func (f *fakeGit) WorktreeAddBranch(
	_ context.Context,
	repoPath string,
	worktreePath string,
	branch string,
	startPoint string,
) (string, error) {
	f.record("WorktreeAddBranch", repoPath, worktreePath, branch, startPoint)
	return f.addStartSha, f.addErr
}

func (f *fakeGit) RemoteBranchExists(
	_ context.Context,
	repoPath string,
	branch string,
) (bool, error) {
	f.record("RemoteBranchExists", repoPath, branch)
	if f.remoteExistsByBranch != nil {
		return f.remoteExistsByBranch[branch], f.remoteExistsErr
	}
	return f.remoteExists, f.remoteExistsErr
}

func (f *fakeGit) RemoteTrackingBranchExists(
	_ context.Context,
	repoPath string,
	branch string,
) (bool, error) {
	f.record("RemoteTrackingBranchExists", repoPath, branch)
	if f.trackingExistsByBranch != nil {
		return f.trackingExistsByBranch[branch], f.trackingExistsErr
	}
	return f.trackingExists, f.trackingExistsErr
}

func (f *fakeGit) SetUpstream(
	_ context.Context,
	repoPath string,
	branch string,
) error {
	f.record("SetUpstream", repoPath, branch)
	return f.setUpstreamErr
}

func (f *fakeGit) FetchRef(
	_ context.Context,
	repoPath string,
	branch string,
) error {
	f.record("FetchRef", repoPath, branch)
	return f.fetchRefErr
}

func (f *fakeGit) FastForwardBranch(
	_ context.Context,
	repoPath string,
	branch string,
) error {
	f.record("FastForwardBranch", repoPath, branch)
	return f.fastForwardBranchErr
}

func (f *fakeGit) WorktreeAdd(
	_ context.Context,
	repoPath string,
	worktreePath string,
	branch string,
) error {
	f.record("WorktreeAdd", repoPath, worktreePath, branch)
	if f.addConflictUntilDetach != nil && !f.detachCalled {
		return f.addConflictUntilDetach
	}
	return f.worktreeAddErr
}

func (f *fakeGit) WorktreeAddAtRef(
	_ context.Context,
	repoPath string,
	worktreePath string,
	branch string,
	startRef string,
) (string, error) {
	f.record("WorktreeAddAtRef", repoPath, worktreePath, branch, startRef)
	if f.addConflictUntilDetach != nil && !f.detachCalled {
		return "", f.addConflictUntilDetach
	}
	if f.worktreeAddErr != nil {
		return "", f.worktreeAddErr
	}
	// The real engine resolves startRef and returns that SHA; the fake's
	// revParseSha stands in for it so the fork-point assertions keep working.
	return f.revParseSha, f.revParseErr
}

func (f *fakeGit) RevParse(
	_ context.Context,
	repoPath string,
	rev string,
) (string, error) {
	f.record("RevParse", repoPath, rev)
	return f.revParseSha, f.revParseErr
}

func (f *fakeGit) MergeBase(
	_ context.Context,
	repoPath string,
	a string,
	b string,
) (string, error) {
	f.record("MergeBase", repoPath, a, b)
	return f.mergeBaseSha, f.mergeBaseErr
}

func (f *fakeGit) Merge(
	_ context.Context,
	repoPath string,
	branch string,
) error {
	f.record("Merge", repoPath, branch)
	return f.mergeErr
}

func (f *fakeGit) MergeSquash(
	_ context.Context,
	repoPath string,
	branch string,
	subject string,
) error {
	f.record("MergeSquash", repoPath, branch, subject)
	return f.squashErr
}

func (f *fakeGit) Rebase(
	_ context.Context,
	repoPath string,
	onto string,
) error {
	f.record("Rebase", repoPath, onto)
	return f.rebaseErr
}

func (f *fakeGit) MergeFFOnly(
	_ context.Context,
	repoPath string,
	branch string,
) error {
	f.record("MergeFFOnly", repoPath, branch)
	return f.ffErr
}

func (f *fakeGit) RebaseThenFFMerge(
	_ context.Context,
	childWorktree string,
	parentBranch string,
	parentWorktree string,
	childBranch string,
) error {
	f.record("RebaseThenFFMerge", childWorktree, parentBranch, parentWorktree, childBranch)
	return f.rebaseFFErr
}

func (f *fakeGit) RebaseOnto(
	_ context.Context,
	repoPath string,
	newTip string,
	forkPoint string,
	branch string,
) error {
	f.record("RebaseOnto", repoPath, newTip, forkPoint, branch)
	return f.rebaseOnto
}

func (f *fakeGit) OperationAbort(
	_ context.Context,
	repoPath string,
) error {
	f.record("OperationAbort", repoPath)
	return f.operationAbortErr
}

func (f *fakeGit) WorkingTreeSummary(
	_ context.Context,
	repoPath string,
	forkPointSha string,
) (int, int, bool, bool, error) {
	f.record("WorkingTreeSummary", repoPath, forkPointSha)
	return f.summaryAdded, f.summaryDeleted, f.summaryHasConflicts, f.summaryHasCommits, f.summaryErr
}

func (f *fakeGit) WorktreePrune(
	_ context.Context,
	repoPath string,
) error {
	f.record("WorktreePrune", repoPath)
	return f.pruneErr
}

func (f *fakeGit) WorktreeRemove(
	_ context.Context,
	repoPath string,
	worktreePath string,
) error {
	f.record("WorktreeRemove", repoPath, worktreePath)
	return f.removeErr
}

func (f *fakeGit) ForceDeleteBranch(
	_ context.Context,
	repoPath string,
	name string,
) error {
	f.record("ForceDeleteBranch", repoPath, name)
	return f.deleteErr
}

type fakeProvider struct {
	engineprovider.Engine
	protected []string
	err       error
}

func (f *fakeProvider) ProtectedBranches(
	_ context.Context,
	_ string,
) ([]string, error) {
	return f.protected, f.err
}

type fakeRepoStore struct {
	store.Store[domain.Repository, string]
	path          string
	defaultBranch string
	remoteURL     string
	name          string
	err           error
	missing       bool
}

func (f *fakeRepoStore) FindByKey(
	_ context.Context,
	_ string,
) (*domain.Repository, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.missing {
		return nil, nil
	}
	// Give the fallback slug a stable identity when neither remote nor name is set,
	// so worktree-path derivation (spec §3.9) has a non-empty leaf in tests that
	// don't exercise slug specifics.
	name := f.name
	if name == "" && f.remoteURL == "" {
		name = "repo"
	}
	return &domain.Repository{
		Path:          f.path,
		DefaultBranch: f.defaultBranch,
		RemoteURL:     f.remoteURL,
		Name:          name,
	}, nil
}

func newNow() func() time.Time {
	return func() time.Time { return time.Unix(0, 0) }
}

// A home PER TEST. It used to be a fixed /tmp/crowbar-test shared by every test
// in the package, which was invisible only for as long as nothing wrote to it —
// creates now publish a real alias symlink there, and a leftover from one test
// (or from a previous run of the suite) then collided with the next.
func fakeHome(t *testing.T) func() (string, error) {
	t.Helper()
	dir := t.TempDir()
	// RESOLVED: on macOS t.TempDir() hands back /var/folders/... while the real
	// path is /private/var/folders/..., and the under-the-home checks resolve
	// symlinks before comparing. An unresolved home makes a managed worktree look
	// external.
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	return func() (string, error) { return dir, nil }
}

// --- CreateChild ---

func TestCreateChild_RecordsForkPointAndLocked(t *testing.T) {
	g := &fakeGit{addStartSha: "sha123"}
	var created workspace.CreateInput
	ws := &fakeWorkspace{
		CreateFn: func(
			_ context.Context,
			in workspace.CreateInput,
			_ time.Time,
		) (domain.Workspace, error) {
			created = in
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	uc := worktree.New(ws, g, &fakeProvider{protected: []string{"main"}}, &fakeRepoStore{}, newNow(), fakeHome(t))

	out, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoID:       "r1",
		ProjectID:    "p1",
		RepoPath:     "/repo",
		RemoteURL:    "https://github.com/test/repo.git",
		Branch:       "feature/x",
		ParentID:     "w-parent",
		ParentBranch: "develop",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, out.ID)
	assert.Equal(t, "sha123", created.ForkPointSha)
	assert.False(t, created.Protected)
	assert.Equal(t, "w-parent", created.ParentID)
	// Child decision first: local remote-tracking ref (absent) then the live
	// remote query (absent) → create local. Only then is the parent start point
	// resolved (develop → not on remote → fork from the local ref).
	assert.Equal(t, []string{"RemoteTrackingBranchExists", "RemoteBranchExists", "RemoteBranchExists", "WorktreeAddBranch"}, g.ops())
	assert.Equal(t, []string{"/repo", created.WorktreePath, "feature/x", "develop"}, g.calls[3].args)
}

// TestCreateChild_DerivesHumanReadableWorktreePath proves the managed child
// worktree lands at the human-readable
// <home>/projects/<project>/<slug>/<branch>/worktree path (spec §3.5/§3.9),
// with the slug resolved from the repo remote URL.
func TestCreateChild_DerivesHumanReadableWorktreePath(t *testing.T) {
	g := &fakeGit{addStartSha: "sha"}
	var created workspace.CreateInput
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			created = in
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	home := t.TempDir()
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(),
		func() (string, error) { return home, nil })

	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoID:       "r1",
		ProjectID:    "p1",
		RepoPath:     "/repo",
		RemoteURL:    "https://github.com/test/repo.git",
		Branch:       "feature/x",
		ParentID:     "w-parent",
		ParentBranch: "develop",
	})
	require.NoError(t, err)
	// The worktree lands at the workspace's IDENTITY, so no later rename has to
	// move a live git worktree.
	assert.Equal(t,
		filepath.Join(home, "projects", "p1", "workspaces", created.ID, "worktree"),
		created.WorktreePath)

	// The human-readable name survives as a symlink, which is what keeps the
	// layout navigable and keeps every path recorded against it resolving.
	alias := filepath.Join(home, "projects", "p1", "github.com", "test", "repo", "feature", "x")
	info, lstatErr := os.Lstat(alias)
	require.NoError(t, lstatErr, "the navigable alias must be published")
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "the alias is a symlink, never a real directory")
	target, readErr := os.Readlink(alias)
	require.NoError(t, readErr)
	assert.Equal(t, filepath.Join(home, "projects", "p1", "workspaces", created.ID), target)
}

// TestCreateChild_RejectsCaseOnlyClash proves a create whose derived worktree
// path collides case-insensitively with an existing sibling is rejected at
// creation (spec §3.9, decision 13) rather than disambiguated.
func TestCreateChild_RejectsCaseOnlyClash(t *testing.T) {
	home := t.TempDir()
	// Pre-create a sibling branch leaf that differs only by case from the
	// candidate the create below derives (main vs Main) under the same slug dir.
	slugDir := filepath.Join(home, "projects", "p1", "github.com", "test", "repo")
	require.NoError(t, os.MkdirAll(filepath.Join(slugDir, "Main"), 0o755))

	g := &fakeGit{addStartSha: "sha"}
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(),
		func() (string, error) { return home, nil })

	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoID:       "r1",
		ProjectID:    "p1",
		RepoPath:     "/repo",
		RemoteURL:    "https://github.com/test/repo.git",
		Branch:       "main",
		ParentBranch: "develop",
	})
	require.ErrorIs(t, err, apperr.ErrInvalidArgument)
	assert.NotContains(t, g.ops(), "WorktreeAdd", "a rejected clash must not touch the worktree")
	assert.NotContains(t, g.ops(), "WorktreeAddBranch")
}

// TestCreateChild_CleansUpWorktreeOnCreateFailure proves H17: if the workspace
// row fails to persist after the worktree + branch were created on disk, both are
// cleaned up best-effort so a fresh-wsID retry is not blocked by the orphaned
// branch and the worktree dir does not dangle.
func TestCreateChild_CleansUpWorktreeOnCreateFailure(t *testing.T) {
	g := &fakeGit{addStartSha: "sha"}
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, _ workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			return domain.Workspace{}, errBoom
		},
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))

	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoID:       "r1",
		ProjectID:    "p1",
		RepoPath:     "/repo",
		RemoteURL:    "https://github.com/test/repo.git",
		Branch:       "feature/x",
		ParentID:     "w-parent",
		ParentBranch: "develop",
	})
	require.ErrorIs(t, err, errBoom)
	ops := g.ops()
	assert.Contains(t, ops, "WorktreeRemove", "failed create must remove the orphaned worktree")
	assert.Contains(t, ops, "ForceDeleteBranch", "failed create must delete the orphaned branch")
}

func TestCreateChild_LocksProtectedBranch(t *testing.T) {
	g := &fakeGit{addStartSha: "sha"}
	var created workspace.CreateInput
	ws := &fakeWorkspace{
		CreateFn: func(
			_ context.Context,
			in workspace.CreateInput,
			_ time.Time,
		) (domain.Workspace, error) {
			created = in
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	uc := worktree.New(ws, g, &fakeProvider{protected: []string{"feature/x"}}, &fakeRepoStore{}, newNow(), fakeHome(t))

	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoPath: "/repo", ProjectID: "p1", RemoteURL: "https://github.com/test/repo.git", Branch: "feature/x", ParentBranch: "develop",
	})
	require.NoError(t, err)
	assert.True(t, created.Protected)
}

// TestCreateChild_RemoteBranchAbsent_CreatesLocal verifies the spec-§3 decision:
// when the branch does NOT exist on the remote, CreateChild creates it locally
// from ParentBranch via WorktreeAddBranch and never fetches/checks out.
func TestCreateChild_RemoteBranchAbsent_CreatesLocal(t *testing.T) {
	g := &fakeGit{remoteExists: false, addStartSha: "localfork"}
	var created workspace.CreateInput
	ws := &fakeWorkspace{
		CreateFn: func(
			_ context.Context,
			in workspace.CreateInput,
			_ time.Time,
		) (domain.Workspace, error) {
			created = in
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))

	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoID:       "r1",
		ProjectID:    "p1",
		RepoPath:     "/repo",
		Branch:       "feature/x",
		ParentID:     "w-parent",
		ParentBranch: "develop",
	})
	require.NoError(t, err)
	// Child decision first: local remote-tracking ref (absent) then the live
	// remote query (absent) → create local. Only then is the parent start point
	// resolved (develop → not on remote → fork from the local ref).
	assert.Equal(t, []string{"RemoteTrackingBranchExists", "RemoteBranchExists", "RemoteBranchExists", "WorktreeAddBranch"}, g.ops())
	assert.Equal(t, []string{"/repo", created.WorktreePath, "feature/x", "develop"}, g.calls[3].args)
	// Fork point comes from the create-local startSha.
	assert.Equal(t, "localfork", created.ForkPointSha)
}

// TestCreateChild_RemoteBranchExists_ChecksOut verifies the spec-§3 decision:
// when the branch already exists on the remote, CreateChild refreshes
// origin/<branch> (FetchRef, which git never refuses) and checks the branch out
// AT that ref (WorktreeAddAtRef, i.e. `git worktree add -B`), so the worktree
// holds origin's commits even when a diverged local branch of the same name
// exists — NOT WorktreeAddBranch, and not a plain WorktreeAdd of the local ref.
// The fork point is the SHA that checkout resolved, so it is by construction the
// commit the worktree is on. The parent is never touched on this path: its start
// point is resolved past the checkout early-return, so the checkout pays no
// parent query and no parent fetch.
func TestCreateChild_RemoteBranchExists_ChecksOut(t *testing.T) {
	// Both the child branch AND the parent branch exist on the remote.
	g := &fakeGit{remoteExists: true, revParseSha: "remotefork"}
	var created workspace.CreateInput
	ws := &fakeWorkspace{
		CreateFn: func(
			_ context.Context,
			in workspace.CreateInput,
			_ time.Time,
		) (domain.Workspace, error) {
			created = in
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))

	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoID:       "r1",
		ProjectID:    "p1",
		RepoPath:     "/repo",
		Branch:       "feature/x",
		ParentID:     "w-parent",
		ParentBranch: "develop",
	})
	require.NoError(t, err)
	// Op sequence: the child decision (local remote-tracking ref absent → falls
	// back to the live query, which says true), then fetch the child's remote ref,
	// read the local tip about to be reset, and check out AT origin/<branch>.
	assert.Equal(t, []string{
		"RemoteTrackingBranchExists", // child local remote-tracking ref? → false
		"RemoteBranchExists",         // child exists live? → true
		"FetchRef",                   // refresh origin/<child>; never refused by git
		"RevParse",                   // local tip, read before the -B reset moves it
		"WorktreeAddAtRef",
		"SetUpstream", // link the checked-out branch to origin/<branch>
	}, g.ops())
	assert.Equal(t, []string{"/repo", "feature/x"}, g.calls[0].args)
	assert.Equal(t, []string{"/repo", "feature/x"}, g.calls[1].args)
	assert.Equal(t, []string{"/repo", "feature/x"}, g.calls[2].args)
	assert.Equal(t, []string{"/repo", "refs/heads/feature/x"}, g.calls[3].args)
	assert.Equal(t, []string{"/repo", created.WorktreePath, "feature/x", "origin/feature/x"}, g.calls[4].args,
		"the checkout must start AT origin's ref, not at whatever the local branch points to")
	assert.Equal(t, []string{"/repo", "feature/x"}, g.calls[5].args,
		"the imported branch is linked back to origin/feature/x")
	// WorktreeAddBranch must NOT be called on the checkout path, and neither may
	// a plain WorktreeAdd, which would check out the LOCAL ref.
	assert.NotContains(t, g.ops(), "WorktreeAddBranch")
	assert.NotContains(t, g.ops(), "WorktreeAdd")
	// The local branch ref must never be moved by a fetch on this path: `git fetch
	// origin <b>:<b>` is refused whenever <b> is checked out anywhere.
	assert.NotContains(t, g.ops(), "FastForwardBranch")
	assert.Equal(t, "remotefork", created.ForkPointSha)
}

// TestCreateChild_NewBranch_ForksFromOriginParentTip is the direct regression for
// the field bug: a brand-new branch (not on the remote) whose parent IS on the
// remote must fork from ORIGIN's fresh parent tip, resolved via FetchRef +
// RevParse(origin/<parent>). It must NOT fast-forward the local parent ref
// (`git fetch origin <parent>:<parent>` is refused whenever the parent is checked
// out — which, for a protected parent, it permanently is in its locked managed
// worktree), and the resolved origin SHA — not the local parent name — must be
// handed to WorktreeAddBranch as the start point.
func TestCreateChild_NewBranch_ForksFromOriginParentTip(t *testing.T) {
	// Parent exists on remote; child does NOT — use per-branch map. RevParse of
	// origin/develop yields the fresh remote tip the child must fork from.
	g := &fakeGit{
		remoteExistsByBranch: map[string]bool{"develop": true},
		revParseSha:          "origin-develop-tip",
		addStartSha:          "newsha",
	}
	uc := worktree.New(&fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			return domain.Workspace{ID: in.ID}, nil
		},
	}, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))

	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoID:       "r1",
		ProjectID:    "p1",
		RepoPath:     "/repo",
		Branch:       "my-feature",
		ParentID:     "w-parent",
		ParentBranch: "develop",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"RemoteTrackingBranchExists", // child local remote-tracking ref? → false
		"RemoteBranchExists",         // child exists live? → false
		"RemoteBranchExists",         // parent exists on origin? → true
		"FetchRef",                   // fetch origin/develop (remote-tracking ref only)
		"RevParse",                   // resolve origin/develop's tip
		"WorktreeAddBranch",          // create the branch forked from that tip
	}, g.ops())
	assert.Equal(t, []string{"/repo", "my-feature"}, g.calls[0].args)
	assert.Equal(t, []string{"/repo", "my-feature"}, g.calls[1].args)
	assert.Equal(t, []string{"/repo", "develop"}, g.calls[2].args)
	assert.Equal(t, []string{"/repo", "develop"}, g.calls[3].args)
	assert.Equal(t, []string{"/repo", "origin/develop"}, g.calls[4].args)
	// The new branch forks from the RESOLVED origin tip, not the stale local ref.
	assert.Equal(t, "origin-develop-tip", g.calls[5].args[3])
	// Never fast-forward the local parent ref — that hits git's checked-out refusal.
	assert.NotContains(t, g.ops(), "FastForwardBranch")
}

// TestCreateChild_ParentFetchFails_FallsBackToLocalTip proves a fetch failure on
// the parent (offline, dead remote) never blocks branch creation: the child is
// created from the LOCAL parent ref and the flow succeeds.
func TestCreateChild_ParentFetchFails_FallsBackToLocalTip(t *testing.T) {
	g := &fakeGit{
		remoteExistsByBranch: map[string]bool{"develop": true},
		fetchRefErr:          errBoom,
		addStartSha:          "localfork",
	}
	var created workspace.CreateInput
	uc := worktree.New(&fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			created = in
			return domain.Workspace{ID: in.ID}, nil
		},
	}, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))

	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoID:       "r1",
		ProjectID:    "p1",
		RepoPath:     "/repo",
		Branch:       "my-feature",
		ParentID:     "w-parent",
		ParentBranch: "develop",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"RemoteTrackingBranchExists",
		"RemoteBranchExists", // child exists live? → false
		"RemoteBranchExists", // parent exists on origin? → true
		"FetchRef",           // …which fails
		"WorktreeAddBranch",  // still creates, from the local parent ref
	}, g.ops())
	// A failed fetch must not leave a resolved-tip attempt behind either.
	assert.NotContains(t, g.ops(), "RevParse")
	assert.Equal(t, []string{"/repo", created.WorktreePath, "my-feature", "develop"}, g.calls[4].args)
	assert.Equal(t, "localfork", created.ForkPointSha)
}

// TestCreateChild_LocalRemoteTrackingRef_ChecksOutDespiteLiveMiss is the unit
// encoding of the import bug fix: when a LOCAL remote-tracking ref for the branch
// exists, the checkout path is taken EVEN IF the live `ls-remote`
// (RemoteBranchExists) would answer false — the swallowed-live-failure that used
// to silently fork a same-named branch off the default branch. The live query is
// not even consulted for the child (short-circuit), and WorktreeAddBranch (the
// create-new-off-parent path) is never reached.
func TestCreateChild_LocalRemoteTrackingRef_ChecksOutDespiteLiveMiss(t *testing.T) {
	g := &fakeGit{
		trackingExistsByBranch: map[string]bool{"feature/x": true},
		remoteExists:           false, // live ls-remote spuriously says "not on remote"
		revParseSha:            "remotefork",
	}
	var created workspace.CreateInput
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			created = in
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))

	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoID:       "r1",
		ProjectID:    "p1",
		RepoPath:     "/repo",
		Branch:       "feature/x",
		ParentID:     "w-parent",
		ParentBranch: "develop",
	})
	require.NoError(t, err)
	// The child decision resolves to checkout via the LOCAL tracking ref — the
	// live query is short-circuited, and the parent is never consulted at all.
	assert.Equal(t, []string{
		"RemoteTrackingBranchExists", // child local remote-tracking ref? → true
		"FetchRef",                   // refresh origin/<child>
		"RevParse",                   // local tip, read before the -B reset moves it
		"WorktreeAddAtRef",
		"SetUpstream", // link the checked-out branch to origin/<branch>
	}, g.ops())
	assert.NotContains(t, g.ops(), "WorktreeAddBranch",
		"a branch with a local remote-tracking ref must never be forked off the parent")
	assert.Equal(t, "remotefork", created.ForkPointSha)
}

// TestCreateChild_CheckoutBestEffortWhenFetchFailsButTrackingRefPresent proves
// the checkout is resilient: when the fetch (FetchRef) fails but the local
// origin/<branch> ref is present, the branch is still checked out AT that ref
// rather than aborting the import.
func TestCreateChild_CheckoutBestEffortWhenFetchFailsButTrackingRefPresent(t *testing.T) {
	g := &fakeGit{
		trackingExistsByBranch: map[string]bool{"feature/x": true},
		fetchRefErr:            errBoom, // offline fetch failure
		revParseSha:            "remotefork",
	}
	var created workspace.CreateInput
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			created = in
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))

	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoID:       "r1",
		ProjectID:    "p1",
		RepoPath:     "/repo",
		Branch:       "feature/x",
		ParentID:     "w-parent",
		ParentBranch: "develop",
	})
	require.NoError(t, err, "a fetch failure must be best-effort when origin/<branch> is present locally")
	assert.Equal(t, []string{
		"RemoteTrackingBranchExists", // child local remote-tracking ref? → true → checkout
		"FetchRef",                   // fetch → fails
		"RemoteTrackingBranchExists", // re-check: local ref present → continue best-effort
		"RevParse",                   // local tip, read before the -B reset moves it
		"WorktreeAddAtRef",
		"SetUpstream", // link the checked-out branch to origin/<branch>
	}, g.ops())
	assert.Equal(t, "remotefork", created.ForkPointSha)
}

// TestCreateChild_RemoteTrackingBranchExistsError proves a genuine failure of the
// local remote-tracking check (a broken repo) is surfaced, not swallowed.
func TestCreateChild_RemoteTrackingBranchExistsError(t *testing.T) {
	g := &fakeGit{trackingExistsErr: errBoom}
	uc := worktree.New(&fakeWorkspace{}, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))
	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoPath: "/r", ProjectID: "p1", Branch: "b", ParentBranch: "develop",
	})
	require.ErrorIs(t, err, errBoom)
}

func TestCreateChild_RemoteBranchExistsError(t *testing.T) {
	g := &fakeGit{remoteExistsErr: errBoom}
	uc := worktree.New(&fakeWorkspace{}, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))
	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoPath: "/r", ProjectID: "p1", Branch: "b", ParentBranch: "develop",
	})
	require.ErrorIs(t, err, errBoom)
}

func TestCreateChild_FetchRefError(t *testing.T) {
	// FetchRef failing on the CHILD branch (checkoutRemoteBranch path) is fatal
	// when there is no local origin/<branch> to fall back on: without either,
	// there is no remote content to import.
	g := &fakeGit{remoteExists: true, fetchRefErr: errBoom}
	uc := worktree.New(&fakeWorkspace{}, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))
	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoPath: "/r", ProjectID: "p1", Branch: "b", ParentBranch: "develop",
	})
	require.ErrorIs(t, err, errBoom)
}

func TestCreateChild_CheckoutWorktreeAddError(t *testing.T) {
	g := &fakeGit{remoteExists: true, revParseSha: "s", worktreeAddErr: errBoom}
	uc := worktree.New(&fakeWorkspace{}, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))
	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoPath: "/r", ProjectID: "p1", Branch: "b", ParentBranch: "develop",
	})
	require.ErrorIs(t, err, errBoom)
}

func TestCreateChild_CheckoutRevParseError(t *testing.T) {
	g := &fakeGit{remoteExists: true, revParseErr: errBoom}
	uc := worktree.New(&fakeWorkspace{}, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))
	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoPath: "/r", ProjectID: "p1", Branch: "b", ParentBranch: "develop",
	})
	require.ErrorIs(t, err, errBoom)
}

// TestCreateChild_AdoptMainWorktreeUnchanged proves the main-worktree adoption
// path (empty parent + branch == parent branch) is untouched by the spec-§3
// remote decision: it resolves HEAD directly and never queries the remote.
func TestCreateChild_AdoptMainWorktreeUnchanged(t *testing.T) {
	g := &fakeGit{revParseSha: "headsha"}
	var created workspace.CreateInput
	ws := &fakeWorkspace{
		CreateFn: func(
			_ context.Context,
			in workspace.CreateInput,
			_ time.Time,
		) (domain.Workspace, error) {
			created = in
			return domain.Workspace{ID: in.ID}, nil
		},
		// No workspace yet adopts the main worktree, so the first adoption proceeds.
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return nil, nil },
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))

	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoID:       "r1",
		ProjectID:    "p1",
		RepoPath:     "/repo",
		Branch:       "main",
		ParentID:     "",
		ParentBranch: "main",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"RevParse"}, g.ops())
	assert.NotContains(t, g.ops(), "RemoteBranchExists")
	assert.Equal(t, "/repo", created.WorktreePath, "adopts the repo path as the worktree")
	assert.Equal(t, "headsha", created.ForkPointSha)
}

func TestCreateChild_AdoptMainWorktree_RevParseError(t *testing.T) {
	g := &fakeGit{revParseErr: errBoom}
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return nil, nil },
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))
	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoPath: "/repo", Branch: "main", ParentBranch: "main",
	})
	require.ErrorIs(t, err, errBoom)
}

// TestCreateChild_RejectsDuplicateBranch_AdoptPath proves a second workspace on
// the repo's default branch (empty parent + branch == default) is rejected with
// ErrBranchWorkspaceExists before any git work — no phantom duplicate row.
// TestCreateChild_DefaultWorkspaceDoesNotBlockImport pins the user-required rule:
// the default workspace (the imported repo folder) is unmanaged and must NOT
// count for the one-managed-workspace-per-branch guard — so it never blocks
// importing its own branch as a real managed workspace. With the default already
// adopting the repo path, creating develop again is allowed and goes to the
// managed-worktree path; it does NOT persist a second default.
func TestCreateChild_DefaultWorkspaceDoesNotBlockImport(t *testing.T) {
	g := &fakeGit{addStartSha: "sha"}
	var created workspace.CreateInput
	createCalls := 0
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{
				{ID: "def", RepoID: "r1", Branch: "develop", WorktreePath: "/repo", IsDefault: true},
			}, nil
		},
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			created = in
			createCalls++
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))

	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo", RemoteURL: "https://github.com/test/repo.git",
		Branch: "develop", ParentID: "", ParentBranch: "develop",
	})

	require.NoError(t, err, "the default workspace must not block importing its branch")
	require.Equal(t, 1, createCalls)
	assert.False(t, created.IsDefault, "a repeat develop create must NOT adopt a second default")
	assert.NotEqual(t, "/repo", created.WorktreePath,
		"it goes to the managed-worktree path, not re-adopting the repo folder")
}

// TestCreateChild_DetachesMainToFreeDefaultBranch proves importing the default
// branch (held by the unmanaged main folder) detaches the main folder to free
// the branch, then retries the worktree add — yielding a real managed worktree.
func TestCreateChild_DetachesMainToFreeDefaultBranch(t *testing.T) {
	inUse := errors.New("fatal: 'develop' is already used by worktree at '/repo'")
	g := &fakeGit{
		remoteExists:           true,
		revParseSha:            "forksha",
		addConflictUntilDetach: inUse,
		worktrees:              []enginegit.WorktreeEntry{{Path: "/repo", Branch: "develop"}},
	}
	var created workspace.CreateInput
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{
				{ID: "def", RepoID: "r1", Branch: "develop", WorktreePath: "/repo", IsDefault: true},
			}, nil
		},
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			created = in
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))

	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo", RemoteURL: "https://github.com/test/repo.git",
		Branch: "develop", ParentID: "", ParentBranch: "develop",
	})

	require.NoError(t, err, "importing the default branch must succeed by detaching the main folder")
	assert.Contains(t, g.ops(), "DetachWorktree", "the main folder must be detached to free the branch")
	var addCount int
	for _, op := range g.ops() {
		if op == "WorktreeAddAtRef" {
			addCount++
		}
	}
	assert.Equal(t, 2, addCount, "worktree add runs once (conflict), then again after the detach")
	assert.NotEqual(t, "/repo", created.WorktreePath, "the managed worktree lives outside the repo folder")
	assert.Equal(t, "forksha", created.ForkPointSha)
}

// TestCreateChild_RollsBackDetachWhenRetryFails proves that if the worktree add
// still fails after the detach, the main folder is re-attached to its branch so
// the repo folder is never left stranded on a detached HEAD.
func TestCreateChild_RollsBackDetachWhenRetryFails(t *testing.T) {
	inUse := errors.New("fatal: 'develop' is already used by worktree at '/repo'")
	g := &fakeGit{
		remoteExists:           true,
		revParseSha:            "forksha",
		addConflictUntilDetach: inUse,
		worktreeAddErr:         errBoom, // even after the detach, the add fails
		worktrees:              []enginegit.WorktreeEntry{{Path: "/repo", Branch: "develop"}},
	}
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{
				{ID: "def", RepoID: "r1", Branch: "develop", WorktreePath: "/repo", IsDefault: true},
			}, nil
		},
		CreateFn: func(_ context.Context, _ workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			t.Fatal("Create must not run when the worktree add fails")
			return domain.Workspace{}, nil
		},
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))

	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo", RemoteURL: "https://github.com/test/repo.git",
		Branch: "develop", ParentID: "", ParentBranch: "develop",
	})

	require.Error(t, err, "a persistent add failure must surface as an error")
	assert.Contains(t, g.ops(), "DetachWorktree", "the detach was attempted")
	assert.Contains(t, g.ops(), "CheckoutBranch", "the main folder must be re-attached after the failed retry")
}

// TestRemoveOne_DefaultBranchWorkspace_ReattachesMainAndKeepsBranch proves that
// removing a managed workspace on the repo's default branch re-attaches the main
// folder to that branch and NEVER force-deletes it (the shared integration
// branch must survive).
func TestRemoveOne_DefaultBranchWorkspace_ReattachesMainAndKeepsBranch(t *testing.T) {
	g := &fakeGit{}
	repos := &fakeRepoStore{path: "/repo", defaultBranch: "develop"}
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{
				{ID: "w1", RepoID: "r1", Branch: "develop", WorktreePath: "/managed"},
			}, nil
		},
		DeleteFn: func(_ context.Context, _ string) error { return nil },
	}
	uc := worktree.New(ws, g, &fakeProvider{}, repos, newNow(), fakeHome(t))

	require.NoError(t, uc.DeleteCascade(context.Background(), "w1"))

	assert.Contains(t, g.ops(), "CheckoutBranch", "the main folder must be re-attached to the default branch")
	assert.NotContains(t, g.ops(), "ForceDeleteBranch", "the default branch must never be force-deleted")
}

// TestRemoveOne_FeatureBranchWorkspace_ForceDeletesBranch proves removing a
// managed workspace on a non-default branch still force-deletes that branch (the
// default-branch protection must not leak into ordinary teardown).
func TestRemoveOne_FeatureBranchWorkspace_ForceDeletesBranch(t *testing.T) {
	g := &fakeGit{}
	repos := &fakeRepoStore{path: "/repo", defaultBranch: "develop"}
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{
				{ID: "w1", RepoID: "r1", Branch: "feature/x", WorktreePath: "/managed"},
			}, nil
		},
		DeleteFn: func(_ context.Context, _ string) error { return nil },
	}
	uc := worktree.New(ws, g, &fakeProvider{}, repos, newNow(), fakeHome(t))

	require.NoError(t, uc.DeleteCascade(context.Background(), "w1"))

	assert.Contains(t, g.ops(), "ForceDeleteBranch", "a feature branch is force-deleted on teardown")
	assert.NotContains(t, g.ops(), "CheckoutBranch", "no re-attach for a non-default branch")
}

// TestCreateChild_RejectsDuplicateBranch_ChildPath proves a NON-default branch
// that already has a workspace is rejected cleanly (not a raw git "branch
// exists" error) before git worktree add runs.
func TestCreateChild_RejectsDuplicateBranch_ChildPath(t *testing.T) {
	g := &fakeGit{addStartSha: "sha"}
	createCalls := 0
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{{ID: "ws-x", RepoID: "r1", Branch: "feature/x"}}, nil
		},
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			createCalls++
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))

	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo", RemoteURL: "https://github.com/test/repo.git",
		Branch: "feature/x", ParentID: "w-parent", ParentBranch: "develop",
	})

	require.ErrorIs(t, err, worktree.ErrBranchWorkspaceExists)
	assert.Equal(t, 0, createCalls)
	assert.NotContains(t, g.ops(), "WorktreeAddBranch", "guard rejects before git worktree add")
}

// A deleted prior adoption must NOT block re-adopting the main worktree (e.g.
// the default workspace was removed and is being recreated).
func TestCreateChild_AdoptMainWorktree_IgnoresDeletedAdoption(t *testing.T) {
	g := &fakeGit{revParseSha: "headsha"}
	var created workspace.CreateInput
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{
				{ID: "old", RepoID: "r1", Branch: "develop", Status: domain.WorkspaceStatusDeleted},
			}, nil
		},
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			created = in
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))

	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo", Branch: "develop", ParentBranch: "develop",
	})
	require.NoError(t, err)
	assert.Equal(t, "/repo", created.WorktreePath)
}

func TestCreateChild_WorktreeAddError(t *testing.T) {
	g := &fakeGit{addErr: errBoom}
	ws := &fakeWorkspace{}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))
	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{RepoPath: "/r", ProjectID: "p1", RemoteURL: "https://github.com/test/repo.git", Branch: "b"})
	require.ErrorIs(t, err, errBoom)
}

func TestCreateChild_ProviderError(t *testing.T) {
	g := &fakeGit{addStartSha: "s"}
	ws := &fakeWorkspace{}
	uc := worktree.New(ws, g, &fakeProvider{err: errBoom}, &fakeRepoStore{}, newNow(), fakeHome(t))
	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{RepoPath: "/r", ProjectID: "p1", RemoteURL: "https://github.com/test/repo.git", Branch: "b"})
	require.ErrorIs(t, err, errBoom)
}

func TestCreateChild_CrowbarHomeError(t *testing.T) {
	g := &fakeGit{}
	badHome := func() (string, error) { return "", errBoom }
	uc := worktree.New(&fakeWorkspace{}, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), badHome)
	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoPath: "/r", Branch: "feature/x", ParentBranch: "develop",
	})
	require.ErrorIs(t, err, errBoom)
	assert.Empty(t, g.calls, "no git work runs when the home path cannot be resolved")
}

// --- MergeIntoParent ---

func mergeWS(
	child domain.Workspace,
	parent domain.Workspace,
	children []domain.Workspace,
) *fakeWorkspace {
	list := append([]domain.Workspace{child, parent}, children...)
	return &fakeWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			if id == child.ID {
				return child, nil
			}
			if id == parent.ID {
				return parent, nil
			}
			return domain.Workspace{}, fmt.Errorf("not found: %s", id)
		},
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return list, nil
		},
	}
}

func TestMergeIntoParent_RejectsLockedParent(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat"}
	parent := domain.Workspace{ID: "p", Status: domain.WorkspaceStatusLocked, WorktreePath: "/pw"}
	g := &fakeGit{}
	uc := worktree.New(mergeWS(child, parent, nil), g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))
	_, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.ErrorIs(t, err, worktree.ErrParentLocked)
	assert.Empty(t, g.calls)
}

func TestMergeIntoParent_RejectsRebaseForNonLeafChild(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop"}
	grandchild := domain.Workspace{ID: "gc", ParentID: "c"}
	g := &fakeGit{}
	uc := worktree.New(mergeWS(child, parent, []domain.Workspace{grandchild}), g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))
	_, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyRebase)
	require.ErrorIs(t, err, worktree.ErrRebaseNonLeaf)
	assert.Empty(t, g.calls)
}

func TestMergeIntoParent_MergeStrategy_RunsInParentThenUpdatesForkPoint(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat", WorktreePath: "/cw"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop"}
	g := &fakeGit{revParseSha: "ptip"}
	ws := mergeWS(child, parent, nil)
	var updatedID, updatedSha string
	ws.UpdateForkPointFn = func(_ context.Context, id, sha string) (domain.Workspace, error) {
		updatedID, updatedSha = id, sha
		return domain.Workspace{}, nil
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))

	res, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.NoError(t, err)
	assert.Equal(t, "ptip", res.ParentTipSha)
	assert.False(t, res.ConflictsPending)
	assert.Equal(t, []string{"Merge", "RevParse", "WorkingTreeSummary", "WorkingTreeSummary"}, g.ops())
	assert.Equal(t, []string{"/pw", "feat"}, g.calls[0].args)
	assert.Equal(t, []string{"/pw", "HEAD"}, g.calls[1].args)
	assert.Equal(t, "c", updatedID)
	assert.Equal(t, "ptip", updatedSha)
}

func TestMergeIntoParent_ResyncsParentAndChildSummaries(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "cfork"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop", ForkPointSha: "pfork"}
	g := &fakeGit{revParseSha: "ptip", summaryAdded: 3, summaryDeleted: 1, summaryHasCommits: true}
	ws := mergeWS(child, parent, nil)
	ws.UpdateForkPointFn = func(_ context.Context, _, _ string) (domain.Workspace, error) {
		return domain.Workspace{}, nil
	}
	var synced []workspace.SyncInput
	ws.SyncFn = func(_ context.Context, in workspace.SyncInput, _ time.Time) (domain.Workspace, error) {
		synced = append(synced, in)
		return domain.Workspace{}, nil
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))

	_, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.NoError(t, err)

	// Parent resyncs from its own fork point; child resyncs from the new tip.
	require.Len(t, synced, 2)
	assert.Equal(t, "p", synced[0].ID)
	assert.Equal(t, 3, synced[0].Added)
	assert.Equal(t, 1, synced[0].Deleted)
	assert.True(t, synced[0].HasCommits)
	assert.Equal(t, "c", synced[1].ID)
	assert.Equal(t, []string{"/pw", "pfork"}, g.calls[2].args, "parent summary uses parent fork point")
	assert.Equal(t, []string{"/cw", "ptip"}, g.calls[3].args, "child summary uses new tip")
}

// TestMergeIntoParent_ResyncSummaryError verifies that a WorkingTreeSummary
// failure during finalize does NOT fail MergeIntoParent — the git merge already
// committed durably and the read-model self-corrects on the next watcher event.
// UpdateForkPoint must still have been called (it is correctness-critical).
func TestMergeIntoParent_ResyncSummaryError(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat", WorktreePath: "/cw"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop"}
	g := &fakeGit{revParseSha: "ptip", summaryErr: errBoom}
	ws := mergeWS(child, parent, nil)
	var forkUpdated bool
	ws.UpdateForkPointFn = func(_ context.Context, id, sha string) (domain.Workspace, error) {
		forkUpdated = true
		return domain.Workspace{}, nil
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))
	res, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.NoError(t, err, "summary resync failure must not fail a durable merge")
	assert.Equal(t, "ptip", res.ParentTipSha)
	assert.False(t, res.ConflictsPending)
	assert.True(t, forkUpdated, "UpdateForkPoint must still be called")
}

func TestMergeIntoParent_SquashStrategy_RunsInParent(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop"}
	g := &fakeGit{revParseSha: "ptip"}
	ws := mergeWS(child, parent, nil)
	ws.UpdateForkPointFn = func(_ context.Context, _, _ string) (domain.Workspace, error) {
		return domain.Workspace{}, nil
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))

	_, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategySquash)
	require.NoError(t, err)
	assert.Equal(t, []string{"MergeSquash", "RevParse", "WorkingTreeSummary", "WorkingTreeSummary"}, g.ops())
	assert.Equal(t, "/pw", g.calls[0].args[0])
	assert.Equal(t, "feat", g.calls[0].args[1])
}

func TestMergeIntoParent_RebaseStrategy_RebasesChildThenFFMerges(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat", WorktreePath: "/cw"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop"}
	g := &fakeGit{revParseSha: "ptip"}
	ws := mergeWS(child, parent, nil)
	ws.UpdateForkPointFn = func(_ context.Context, _, _ string) (domain.Workspace, error) {
		return domain.Workspace{}, nil
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))

	_, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyRebase)
	require.NoError(t, err)
	assert.Equal(t, []string{"RebaseThenFFMerge", "RevParse", "WorkingTreeSummary", "WorkingTreeSummary"}, g.ops())
	assert.Equal(t, []string{"/cw", "develop", "/pw", "feat"}, g.calls[0].args)
}

// TestMergeIntoParent_Conflict_SetsPRConflicts proves that a local merge
// conflict transitions the child to Status=pr-conflicts (via a
// SyncWorkingTreeState HasConflicts write) instead of recording a pending merge,
// AND aborts the in-progress merge in the PARENT worktree (where the plain
// merge runs) so neither worktree is left stuck (try-then-warn, H6/H7 guard).
func TestMergeIntoParent_Conflict_SetsPRConflicts(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop"}
	g := &fakeGit{mergeErr: enginegit.ErrConflict}
	ws := mergeWS(child, parent, nil)
	var synced []workspace.SyncInput
	ws.SyncFn = func(_ context.Context, in workspace.SyncInput, _ time.Time) (domain.Workspace, error) {
		synced = append(synced, in)
		return domain.Workspace{}, nil
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))

	res, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.NoError(t, err)
	assert.True(t, res.ConflictsPending)
	assert.Empty(t, res.ParentTipSha)
	require.Len(t, synced, 1)
	assert.Equal(t, "c", synced[0].ID)
	assert.True(t, synced[0].HasConflicts)
	// The plain merge runs in the parent worktree, so the abort targets the parent.
	assert.Equal(t, []string{"Merge", "OperationAbort"}, g.ops())
	assert.Equal(t, []string{"/pw"}, g.calls[1].args, "abort must run in the PARENT worktree")
}

// TestMergeIntoParent_RebaseConflict_SetsPRConflicts proves the rebase strategy
// aborts in the CHILD worktree (where RebaseThenFFMerge rebases the child) on a
// conflict, so the child is never left mid-rebase and the parent is untouched.
func TestMergeIntoParent_RebaseConflict_SetsPRConflicts(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat", WorktreePath: "/cw"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop"}
	g := &fakeGit{rebaseFFErr: enginegit.ErrConflict}
	ws := mergeWS(child, parent, nil)
	var synced []workspace.SyncInput
	ws.SyncFn = func(_ context.Context, in workspace.SyncInput, _ time.Time) (domain.Workspace, error) {
		synced = append(synced, in)
		return domain.Workspace{}, nil
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))
	res, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyRebase)
	require.NoError(t, err)
	assert.True(t, res.ConflictsPending)
	require.Len(t, synced, 1)
	assert.True(t, synced[0].HasConflicts)
	// The rebase runs in the child worktree, so the abort targets the child.
	assert.Equal(t, []string{"RebaseThenFFMerge", "OperationAbort"}, g.ops())
	assert.Equal(t, []string{"/cw"}, g.calls[1].args, "abort must run in the CHILD worktree")
}

// TestMergeIntoParent_SquashConflict_AbortsInParent proves the squash strategy
// aborts in the PARENT worktree (where MergeSquash runs) on a conflict. This is
// the core H6 guard: a conflicting squash merge must not brick the parent.
func TestMergeIntoParent_SquashConflict_AbortsInParent(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop"}
	g := &fakeGit{squashErr: enginegit.ErrConflict}
	ws := mergeWS(child, parent, nil)
	var synced []workspace.SyncInput
	ws.SyncFn = func(_ context.Context, in workspace.SyncInput, _ time.Time) (domain.Workspace, error) {
		synced = append(synced, in)
		return domain.Workspace{}, nil
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))
	res, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategySquash)
	require.NoError(t, err)
	assert.True(t, res.ConflictsPending)
	require.Len(t, synced, 1)
	assert.True(t, synced[0].HasConflicts)
	assert.Equal(t, []string{"MergeSquash", "OperationAbort"}, g.ops())
	assert.Equal(t, []string{"/pw"}, g.calls[1].args, "squash abort must run in the PARENT worktree")
}

// TestMergeIntoParent_Conflict_AbortFailure_FlagsParentAndChild proves R7: if
// OperationAbort fails (the worktree stays mid-merge), the op is still best-effort
// (no error returned) but the STUCK worktree — the PARENT for the merge/squash
// strategies — is ALSO flagged pr-conflicts, so the brick is VISIBLE and
// recoverable instead of a silent merge-pending success.
func TestMergeIntoParent_Conflict_AbortFailure_FlagsParentAndChild(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop"}
	g := &fakeGit{mergeErr: enginegit.ErrConflict, operationAbortErr: errBoom}
	ws := mergeWS(child, parent, nil)
	var synced []workspace.SyncInput
	ws.SyncFn = func(_ context.Context, in workspace.SyncInput, _ time.Time) (domain.Workspace, error) {
		synced = append(synced, in)
		return domain.Workspace{}, nil
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))
	res, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.NoError(t, err)
	assert.True(t, res.ConflictsPending)
	flagged := map[string]bool{}
	for _, s := range synced {
		if s.HasConflicts {
			flagged[s.ID] = true
		}
	}
	assert.True(t, flagged["p"], "the stuck PARENT worktree must be flagged pr-conflicts on abort failure (R7)")
	assert.True(t, flagged["c"], "the child must be flagged pr-conflicts")
}

// TestMergeIntoParent_Conflict_AbortSuccess_FlagsOnlyChild proves that when the
// abort SUCCEEDS (the normal case) the parent worktree is clean again, so only
// the child is flagged — the parent is NOT spuriously marked conflicted.
func TestMergeIntoParent_Conflict_AbortSuccess_FlagsOnlyChild(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop"}
	g := &fakeGit{mergeErr: enginegit.ErrConflict} // abort succeeds
	ws := mergeWS(child, parent, nil)
	var synced []workspace.SyncInput
	ws.SyncFn = func(_ context.Context, in workspace.SyncInput, _ time.Time) (domain.Workspace, error) {
		synced = append(synced, in)
		return domain.Workspace{}, nil
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))
	res, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.NoError(t, err)
	assert.True(t, res.ConflictsPending)
	require.Len(t, synced, 1, "a clean abort flags only the child")
	assert.Equal(t, "c", synced[0].ID)
	assert.True(t, synced[0].HasConflicts)
}

func TestMergeIntoParent_NonConflictError_Propagates(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop"}
	g := &fakeGit{mergeErr: errBoom}
	uc := worktree.New(mergeWS(child, parent, nil), g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))
	_, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.ErrorIs(t, err, errBoom)
}

func TestMergeIntoParent_GetChildError(t *testing.T) {
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, _ string) (domain.Workspace, error) {
			return domain.Workspace{}, errBoom
		},
	}
	uc := worktree.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))
	_, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.ErrorIs(t, err, errBoom)
}

func TestMergeIntoParent_GetParentError(t *testing.T) {
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			if id == "c" {
				return domain.Workspace{ID: "c", ParentID: "p"}, nil
			}
			return domain.Workspace{}, errBoom
		},
	}
	uc := worktree.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))
	_, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.ErrorIs(t, err, errBoom)
}

// --- Reparent ---

func TestReparent_RejectsNonLeafChild(t *testing.T) {
	child := domain.Workspace{ID: "c"}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np"}
	grandchild := domain.Workspace{ID: "gc", ParentID: "c"}
	ws := reparentWS(child, newParent, []domain.Workspace{grandchild})
	g := &fakeGit{}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))
	_, err := uc.Reparent(context.Background(), "c", "np")
	require.ErrorIs(t, err, worktree.ErrChildHasChildren)
	assert.Empty(t, g.calls)
}

func TestReparent_RejectsSelfParent(t *testing.T) {
	// A workspace must never become its own parent: the self-loop detaches the
	// node in the tree and (via childHasChildren) makes it permanently
	// unreparentable. The guard rejects it before any git work.
	child := domain.Workspace{ID: "c", Branch: "feat", WorktreePath: "/cw"}
	ws := reparentWS(child, child, nil)
	g := &fakeGit{}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))
	_, err := uc.Reparent(context.Background(), "c", "c")
	require.ErrorIs(t, err, worktree.ErrSelfParent)
	assert.Empty(t, g.calls)
}

func TestReparent_SelfLoopedChildIsStillALeaf(t *testing.T) {
	// A workspace already corrupted into a self-loop (ParentID == ID) must not
	// count as its own child, so the leaf check passes and it can be reparented
	// out of the bad state onto a real parent.
	child := domain.Workspace{ID: "c", ParentID: "c", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "fork"}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np"}
	ws := reparentWS(child, newParent, nil)
	ws.ReparentFn = func(_ context.Context, id, parentID, _ string, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{ID: id, ParentID: parentID}, nil
	}
	g := &fakeGit{revParseSha: "ntip"}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))
	_, err := uc.Reparent(context.Background(), "c", "np")
	require.NoError(t, err) // not blocked by a phantom self-child
}

func TestReparent_AllowsLockedNewParent(t *testing.T) {
	// A locked (protected) branch is a valid re-parent target: it already adopts
	// children via create, so reparent must be consistent — the old "07 §4
	// new-parent-locked" block was incoherent and has been removed.
	child := domain.Workspace{ID: "c", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "fork"}
	newParent := domain.Workspace{ID: "np", Status: domain.WorkspaceStatusLocked, WorktreePath: "/np"}
	ws := reparentWS(child, newParent, nil)
	ws.ReparentFn = func(_ context.Context, id, _, _ string, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{ID: id}, nil
	}
	g := &fakeGit{revParseSha: "ntip"}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))
	_, err := uc.Reparent(context.Background(), "c", "np")
	require.NoError(t, err)
	assert.Equal(t, []string{"RevParse", "RebaseOnto", "WorkingTreeSummary"}, g.ops())
}

func TestReparent_RebasesOntoNewTipAndUpdatesAggregate(t *testing.T) {
	child := domain.Workspace{ID: "c", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "fork"}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np"}
	ws := reparentWS(child, newParent, nil)
	var rID, rParent, rSha string
	ws.ReparentFn = func(_ context.Context, id, parentID, forkPointSha string, _ time.Time) (domain.Workspace, error) {
		rID, rParent, rSha = id, parentID, forkPointSha
		return domain.Workspace{ID: id}, nil
	}
	g := &fakeGit{revParseSha: "ntip"}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))

	_, err := uc.Reparent(context.Background(), "c", "np")
	require.NoError(t, err)
	// A clean rebase settles the move and resyncs the child's working-tree summary
	// against the new base, so a prior predicted-conflict status clears and the
	// diff stats reflect the new parent.
	assert.Equal(t, []string{"RevParse", "RebaseOnto", "WorkingTreeSummary"}, g.ops())
	assert.Equal(t, []string{"/np", "HEAD"}, g.calls[0].args)
	assert.Equal(t, []string{"/cw", "ntip", "fork", "feat"}, g.calls[1].args)
	assert.Equal(t, []string{"/cw", "ntip"}, g.calls[2].args)
	assert.Equal(t, "c", rID)
	assert.Equal(t, "np", rParent)
	assert.Equal(t, "ntip", rSha)
}

func TestReparent_RebaseOntoError(t *testing.T) {
	child := domain.Workspace{ID: "c", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "fork"}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np"}
	ws := reparentWS(child, newParent, nil)
	g := &fakeGit{revParseSha: "ntip", rebaseOnto: errBoom}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))
	_, err := uc.Reparent(context.Background(), "c", "np")
	require.ErrorIs(t, err, errBoom)
}

func TestReparent_GetChildError(t *testing.T) {
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, _ string) (domain.Workspace, error) {
			return domain.Workspace{}, errBoom
		},
	}
	uc := worktree.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))
	_, err := uc.Reparent(context.Background(), "c", "np")
	require.ErrorIs(t, err, errBoom)
}

func TestReparent_GetNewParentError(t *testing.T) {
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			if id == "c" {
				return domain.Workspace{ID: "c"}, nil
			}
			return domain.Workspace{}, errBoom
		},
	}
	uc := worktree.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))
	_, err := uc.Reparent(context.Background(), "c", "np")
	require.ErrorIs(t, err, errBoom)
}

func reparentWS(
	child domain.Workspace,
	newParent domain.Workspace,
	children []domain.Workspace,
) *fakeWorkspace {
	list := append([]domain.Workspace{child, newParent}, children...)
	return &fakeWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			if id == child.ID {
				return child, nil
			}
			if id == newParent.ID {
				return newParent, nil
			}
			return domain.Workspace{}, fmt.Errorf("not found: %s", id)
		},
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return list, nil
		},
	}
}

// --- DeleteCascade ---

// TestDeleteCascade_SkipsLockedStatus proves the cascade planner derives the
// skip-locked rule from Status==locked (not the legacy Locked bool): the
// locked-status node 'b' is preserved while its unlocked descendant 'c' is
// still deleted.
type fakeTerminalReaper struct {
	byWorkspace map[string][]string
	killed      []string
}

func (f *fakeTerminalReaper) ListSessionsForWorkspace(wsID string) []string {
	return f.byWorkspace[wsID]
}

func (f *fakeTerminalReaper) Kill(_ context.Context, sid string) error {
	f.killed = append(f.killed, sid)
	return nil
}

// TestDeleteCascade_KillsTerminalSessions proves pass-7: every cascade-deleted
// workspace's live PTY sessions are terminated, so deleting a workspace (or a
// whole subtree) never leaks shell processes, fds, or per-session ring buffers.
func TestDeleteCascade_KillsTerminalSessions(t *testing.T) {
	all := []domain.Workspace{
		{ID: "root", RepoID: "r", Branch: "b-root", WorktreePath: "/wt/root"},
		{ID: "child", ParentID: "root", RepoID: "r", Branch: "b-child", WorktreePath: "/wt/child"},
	}
	g := &fakeGit{}
	ws := &fakeWorkspace{
		ListFn:   func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
		DeleteFn: func(_ context.Context, _ string) error { return nil },
	}
	reaper := &fakeTerminalReaper{byWorkspace: map[string][]string{
		"root":  {"root-sess"},
		"child": {"child-sess-1", "child-sess-2"},
	}}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome(t),
		worktree.WithTerminalReaper(reaper))

	require.NoError(t, uc.DeleteCascade(context.Background(), "root"))
	assert.ElementsMatch(t, []string{"root-sess", "child-sess-1", "child-sess-2"}, reaper.killed,
		"every cascade-deleted workspace's terminal sessions must be killed")
}

func TestDeleteCascade_SkipsLockedStatus(t *testing.T) {
	all := []domain.Workspace{
		{ID: "root", RepoID: "r", Branch: "b-root", WorktreePath: "/wt/root"},
		{ID: "a", ParentID: "root", RepoID: "r", Branch: "b-a", WorktreePath: "/wt/a"},
		{ID: "b", ParentID: "a", Status: domain.WorkspaceStatusLocked, RepoID: "r", Branch: "b-b", WorktreePath: "/wt/b"},
		{ID: "c", ParentID: "b", RepoID: "r", Branch: "b-c", WorktreePath: "/wt/c"},
	}
	g := &fakeGit{}
	var deleted []string
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
		DeleteFn: func(_ context.Context, id string) error {
			deleted = append(deleted, id)
			return nil
		},
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome(t))

	require.NoError(t, uc.DeleteCascade(context.Background(), "root"))
	assert.Equal(t, []string{"c", "a", "root"}, deleted)
	assert.Equal(t, []string{
		"WorktreeRemove", "ForceDeleteBranch",
		"WorktreeRemove", "ForceDeleteBranch",
		"WorktreeRemove", "ForceDeleteBranch",
	}, g.ops())
	assert.Equal(t, []string{"/repo", "/wt/c"}, g.calls[0].args)
	assert.Equal(t, []string{"/repo", "b-c"}, g.calls[1].args)
}

// TestDeleteCascade_RejectsLockedRootStatus proves the root-level guard now
// derives from Status==locked rather than the legacy Locked bool.
func TestDeleteCascade_RejectsLockedRootStatus(t *testing.T) {
	all := []domain.Workspace{
		{ID: "root", RepoID: "r", Status: domain.WorkspaceStatusLocked, Branch: "b", WorktreePath: "/wt"},
	}
	g := &fakeGit{}
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome(t))
	require.ErrorIs(t, uc.DeleteCascade(context.Background(), "root"), worktree.ErrWorkspaceLocked)
	assert.Empty(t, g.calls)
}

func TestDeleteCascade_ListError(t *testing.T) {
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return nil, errBoom },
	}
	uc := worktree.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))
	require.ErrorIs(t, uc.DeleteCascade(context.Background(), "root"), errBoom)
}

// H17: removeOne is best-effort — a repo-path / worktree / branch teardown
// failure must drop the read-model row anyway (the user asked to delete it),
// never abort the cascade and never leave a ghost row pointing at a gone
// worktree. Orphaned worktrees on disk are reaped by `git worktree prune`.

func TestDeleteCascade_RepoPathError_DropsRowBestEffort(t *testing.T) {
	all := []domain.Workspace{{ID: "root", RepoID: "r"}}
	var deleted []string
	ws := &fakeWorkspace{
		ListFn:   func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
		DeleteFn: func(_ context.Context, id string) error { deleted = append(deleted, id); return nil },
	}
	uc := worktree.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{err: errBoom}, newNow(), fakeHome(t))
	require.NoError(t, uc.DeleteCascade(context.Background(), "root"))
	assert.Equal(t, []string{"root"}, deleted)
}

func TestDeleteCascade_MissingRepoRow_DropsRowNoPanic(t *testing.T) {
	all := []domain.Workspace{{ID: "root", RepoID: "r", WorktreePath: "/wt", Branch: "b"}}
	var deleted []string
	ws := &fakeWorkspace{
		ListFn:   func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
		DeleteFn: func(_ context.Context, id string) error { deleted = append(deleted, id); return nil },
	}
	// FindByKey returns (nil, nil) for a missing repo row (must not panic); the
	// repo is gone so the worktree is unreachable, so the row is dropped best-effort.
	uc := worktree.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{missing: true}, newNow(), fakeHome(t))
	require.NoError(t, uc.DeleteCascade(context.Background(), "root"))
	assert.Equal(t, []string{"root"}, deleted)
}

func TestDeleteCascade_WorktreeRemoveError_DropsRowBestEffort(t *testing.T) {
	all := []domain.Workspace{{ID: "root", RepoID: "r", WorktreePath: "/wt", Branch: "b"}}
	var deleted []string
	ws := &fakeWorkspace{
		ListFn:   func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
		DeleteFn: func(_ context.Context, id string) error { deleted = append(deleted, id); return nil },
	}
	g := &fakeGit{removeErr: errBoom}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome(t))
	require.NoError(t, uc.DeleteCascade(context.Background(), "root"))
	assert.Equal(t, []string{"root"}, deleted)
}

func TestDeleteCascade_BranchDeleteError_DropsRowBestEffort(t *testing.T) {
	all := []domain.Workspace{{ID: "root", RepoID: "r", WorktreePath: "/wt", Branch: "b"}}
	var deleted []string
	ws := &fakeWorkspace{
		ListFn:   func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
		DeleteFn: func(_ context.Context, id string) error { deleted = append(deleted, id); return nil },
	}
	g := &fakeGit{deleteErr: errBoom}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome(t))
	require.NoError(t, uc.DeleteCascade(context.Background(), "root"))
	assert.Equal(t, []string{"root"}, deleted)
}

func TestMergeIntoParent_SetPRConflictsError(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop"}
	g := &fakeGit{mergeErr: enginegit.ErrConflict}
	ws := mergeWS(child, parent, nil)
	ws.SyncFn = func(_ context.Context, _ workspace.SyncInput, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{}, errBoom
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))
	_, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.ErrorIs(t, err, errBoom)
}

func TestMergeIntoParent_UpdateForkPointError(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop"}
	g := &fakeGit{revParseSha: "ptip"}
	ws := mergeWS(child, parent, nil)
	ws.UpdateForkPointFn = func(_ context.Context, _, _ string) (domain.Workspace, error) {
		return domain.Workspace{}, errBoom
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))
	_, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.ErrorIs(t, err, errBoom)
}

func TestMergeIntoParent_GuardListError(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop"}
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			if id == "c" {
				return child, nil
			}
			return parent, nil
		},
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return nil, errBoom },
	}
	uc := worktree.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))
	_, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyRebase)
	require.ErrorIs(t, err, errBoom)
}

func TestReparent_GuardListError(t *testing.T) {
	child := domain.Workspace{ID: "c"}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np"}
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			if id == "c" {
				return child, nil
			}
			return newParent, nil
		},
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return nil, errBoom },
	}
	uc := worktree.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))
	_, err := uc.Reparent(context.Background(), "c", "np")
	require.ErrorIs(t, err, errBoom)
}

func TestMergeIntoParent_RevParseError(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop"}
	g := &fakeGit{revParseErr: errBoom}
	uc := worktree.New(mergeWS(child, parent, nil), g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))
	_, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.ErrorIs(t, err, errBoom)
}

func TestReparent_RevParseError(t *testing.T) {
	child := domain.Workspace{ID: "c", Branch: "feat", WorktreePath: "/cw"}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np"}
	g := &fakeGit{revParseErr: errBoom}
	uc := worktree.New(reparentWS(child, newParent, nil), g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))
	_, err := uc.Reparent(context.Background(), "c", "np")
	require.ErrorIs(t, err, errBoom)
}

// TestMergeIntoParent_RejectsUnprovisionedParent proves a placeholder parent
// (locked + empty WorktreePath) is rejected before any git runs — no
// RevParse("", "HEAD"). It is ALSO rejected as locked; the empty-path guard is
// the explicit backstop the spec adds (§3.4/B2).
func TestMergeIntoParent_RejectsUnprovisionedParent(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat", WorktreePath: "/cw"}
	parent := domain.Workspace{ID: "p", Status: domain.WorkspaceStatusLocked, WorktreePath: ""}
	g := &fakeGit{}
	uc := worktree.New(mergeWS(child, parent, nil), g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))
	_, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.Error(t, err)
	assert.Empty(t, g.calls, "no git runs against an unprovisioned parent")
}

// TestReparent_RejectsUnprovisionedNewParent: reparenting onto a placeholder
// parent is rejected before RevParse.
func TestReparent_RejectsUnprovisionedNewParent(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "old", Branch: "feat", WorktreePath: "/cw"}
	newParent := domain.Workspace{ID: "np", Status: domain.WorkspaceStatusLocked, WorktreePath: ""}
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			if id == "c" {
				return child, nil
			}
			return newParent, nil
		},
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{child, newParent}, nil
		},
	}
	g := &fakeGit{}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))
	_, err := uc.Reparent(context.Background(), "c", "np")
	require.ErrorIs(t, err, worktree.ErrParentUnprovisioned)
	assert.Empty(t, g.calls)
}

// TestRebaseOntoParent_RejectsUnprovisionedParent: finishing the move against a
// placeholder parent is rejected before RevParse.
func TestRebaseOntoParent_RejectsUnprovisionedParent(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "f"}
	parent := domain.Workspace{ID: "p", Status: domain.WorkspaceStatusLocked, WorktreePath: ""}
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			if id == "c" {
				return child, nil
			}
			return parent, nil
		},
	}
	g := &fakeGit{}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))
	_, err := uc.RebaseOntoParent(context.Background(), "c")
	require.ErrorIs(t, err, worktree.ErrParentUnprovisioned)
	assert.Empty(t, g.calls)
}

// TestRemoveOne_PlaceholderSkipsGitTeardown proves a placeholder (empty
// WorktreePath) whose branch is a protected NON-default branch is torn down as a
// pure read-model drop: no WorktreeRemove, no ForceDeleteBranch, no CheckoutBranch
// — the real branch is never git-touched (spec §5 defense-in-depth). The
// placeholder is created NON-locked here only to exercise removeOne directly
// (DeleteCascade's locked guard is proven separately by TestDeleteCascade).
func TestRemoveOne_PlaceholderSkipsGitTeardown(t *testing.T) {
	g := &fakeGit{}
	repos := &fakeRepoStore{path: "/repo", defaultBranch: "develop"}
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{
				{ID: "ph", RepoID: "r1", Branch: "master", WorktreePath: ""},
			}, nil
		},
		DeleteFn: func(_ context.Context, _ string) error { return nil },
	}
	uc := worktree.New(ws, g, &fakeProvider{}, repos, newNow(), fakeHome(t))

	require.NoError(t, uc.DeleteCascade(context.Background(), "ph"))

	assert.NotContains(t, g.ops(), "WorktreeRemove")
	assert.NotContains(t, g.ops(), "ForceDeleteBranch", "the real protected branch must never be force-deleted")
	assert.NotContains(t, g.ops(), "CheckoutBranch")
}

// TestCreateChild_UsesHolderResolveForDetach proves the detach path goes through
// the shared holder primitive: on the "already used by worktree" conflict it
// prunes (holder.Resolve step 1) and lists, sees the main folder holds the
// branch (held-by-home), detaches, and retries — one unified mechanism (spec §5).
func TestCreateChild_UsesHolderResolveForDetach(t *testing.T) {
	inUse := errors.New("fatal: 'develop' is already used by worktree at '/repo'")
	g := &fakeGit{
		remoteExists:           true,
		revParseSha:            "forksha",
		addConflictUntilDetach: inUse,
		worktrees:              []enginegit.WorktreeEntry{{Path: "/repo", Branch: "develop"}},
	}
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{
				{ID: "def", RepoID: "r1", Branch: "develop", WorktreePath: "/repo", IsDefault: true},
			}, nil
		},
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))

	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo", RemoteURL: "https://github.com/test/repo.git",
		Branch: "develop", ParentID: "", ParentBranch: "develop",
	})
	require.NoError(t, err)
	assert.Contains(t, g.ops(), "WorktreePrune", "holder.Resolve prunes dead regs before classifying")
	assert.Contains(t, g.ops(), "DetachWorktree")
}

// TestRetryProvision_FreeBranch_ProvisionsInPlace proves Retry on a placeholder
// (free after resolution) materialises a worktree, records the fork point, and
// provisions the SAME id in place (spec §3.3).
func TestRetryProvision_FreeBranch_ProvisionsInPlace(t *testing.T) {
	g := &fakeGit{revParseSha: "forksha", worktrees: nil} // no holder → Free
	var gotID, gotPath, gotSha string
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			return domain.Workspace{
				ID: id, RepoID: "r1", ProjectID: "p1", Branch: "develop",
				Status: domain.WorkspaceStatusLocked, HeldByPath: "/repo",
			}, nil
		},
		ProvisionInPlaceFn: func(id, path, sha string) (domain.Workspace, error) {
			gotID, gotPath, gotSha = id, path, sha
			return domain.Workspace{ID: id, WorktreePath: path, ForkPointSha: sha, Status: domain.WorkspaceStatusLocked}, nil
		},
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome(t))

	out, err := uc.RetryProvision(context.Background(), "ph")
	require.NoError(t, err)
	assert.Equal(t, "ph", gotID, "provisions the SAME id in place")
	assert.NotEmpty(t, gotPath)
	assert.Equal(t, "forksha", gotSha)
	assert.Equal(t, domain.WorkspaceStatusLocked, out.Status)
	assert.Contains(t, g.ops(), "WorktreeAdd")
}

// TestRetryProvision_StillHeld_ReturnsError proves a Retry while the branch is
// still held by the home returns ErrBranchStillHeld and does NOT provision.
func TestRetryProvision_StillHeld_ReturnsError(t *testing.T) {
	g := &fakeGit{worktrees: []enginegit.WorktreeEntry{{Path: "/repo", Branch: "develop"}}}
	provisionCalled := false
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			return domain.Workspace{
				ID: id, RepoID: "r1", Branch: "develop",
				Status: domain.WorkspaceStatusLocked, HeldByPath: "/repo",
			}, nil
		},
		ProvisionInPlaceFn: func(_, _, _ string) (domain.Workspace, error) {
			provisionCalled = true
			return domain.Workspace{}, nil
		},
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome(t))

	_, err := uc.RetryProvision(context.Background(), "ph")
	require.ErrorIs(t, err, worktree.ErrBranchStillHeld)
	assert.False(t, provisionCalled, "no provision while the branch is still held")
}

// TestDetachHolder_Home_ClearsBranchThenProvisions proves detaching a home
// holder detaches the repo folder, clears the home row's branch via ClearBranch,
// then provisions the placeholder in place (spec §3.5/B6). The second
// holder.Resolve (inside RetryProvision) sees the freed branch.
func TestDetachHolder_Home_ClearsBranchThenProvisions(t *testing.T) {
	// First Resolve: home holds develop. After DetachWorktree the fake frees it,
	// so RetryProvision's Resolve sees Free.
	g := &fakeGit{
		revParseSha:            "forksha",
		worktrees:              []enginegit.WorktreeEntry{{Path: "/repo", Branch: "develop"}},
		addConflictUntilDetach: nil,
	}
	clearedID := ""
	provisioned := false
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			return domain.Workspace{
				ID: id, RepoID: "r1", ProjectID: "p1", Branch: "develop",
				Status: domain.WorkspaceStatusLocked, HeldByPath: "/repo",
			}, nil
		},
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{
				{ID: "home", RepoID: "r1", Branch: "develop", WorktreePath: "/repo", IsDefault: true},
			}, nil
		},
		ClearBranchFn: func(id string) (domain.Workspace, error) {
			clearedID = id
			return domain.Workspace{ID: id}, nil
		},
		ProvisionInPlaceFn: func(id, path, sha string) (domain.Workspace, error) {
			provisioned = true
			return domain.Workspace{ID: id, WorktreePath: path, ForkPointSha: sha, Status: domain.WorkspaceStatusLocked}, nil
		},
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome(t))

	_, err := uc.DetachHolder(context.Background(), "ph")
	require.NoError(t, err)
	assert.Contains(t, g.ops(), "DetachWorktree")
	assert.Equal(t, "home", clearedID, "the home row's branch is cleared")
	assert.True(t, provisioned, "the placeholder is provisioned after the detach")
}

// TestDetachHolder_DetachFails_NoPartialState proves a detach failure
// (mid-merge/rebase) surfaces cleanly: no ClearBranch, no provision.
func TestDetachHolder_DetachFails_NoPartialState(t *testing.T) {
	g := &fakeGit{
		worktrees: []enginegit.WorktreeEntry{{Path: "/repo", Branch: "develop"}},
		detachErr: errors.New("fatal: cannot detach while merging"),
	}
	cleared := false
	provisioned := false
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			return domain.Workspace{
				ID: id, RepoID: "r1", Branch: "develop",
				Status: domain.WorkspaceStatusLocked, HeldByPath: "/repo",
			}, nil
		},
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{{ID: "home", RepoID: "r1", IsDefault: true}}, nil
		},
		ClearBranchFn:      func(_ string) (domain.Workspace, error) { cleared = true; return domain.Workspace{}, nil },
		ProvisionInPlaceFn: func(_, _, _ string) (domain.Workspace, error) { provisioned = true; return domain.Workspace{}, nil },
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome(t))

	_, err := uc.DetachHolder(context.Background(), "ph")
	require.Error(t, err)
	assert.False(t, cleared, "no ClearBranch after a failed detach")
	assert.False(t, provisioned, "no provision after a failed detach")
}

// TestRetryProvision_HeldByManaged_ReturnsError proves a Retry while ANOTHER
// Crowbar-managed worktree holds the branch refuses with its own sentinel rather
// than letting `git worktree add` fail deep in materialize. It is NOT
// ErrBranchStillHeld: detaching is not on offer, because freeing the branch
// would strand the workspace that owns it. This became reachable when import
// stopped silently dropping a protected branch held by a managed worktree that
// outlived its repo row — exactly the row that lands here.
func TestRetryProvision_HeldByManaged_ReturnsError(t *testing.T) {
	// The holder has to be INSIDE this test's crowbar home for holder.Resolve to
	// classify it as managed rather than external.
	home := fakeHome(t)
	homeDir, homeErr := home()
	require.NoError(t, homeErr)
	holderPath := filepath.Join(homeDir, "projects", "p1", "r1", "develop", "worktree")
	g := &fakeGit{worktrees: []enginegit.WorktreeEntry{
		{Path: holderPath, Branch: "develop"},
	}}
	provisionCalled := false
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			return domain.Workspace{
				ID: id, RepoID: "r1", ProjectID: "p1", Branch: "develop",
				Status: domain.WorkspaceStatusLocked, HeldByPath: "/repo",
			}, nil
		},
		ProvisionInPlaceFn: func(_, _, _ string) (domain.Workspace, error) {
			provisionCalled = true
			return domain.Workspace{}, nil
		},
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), home)

	_, err := uc.RetryProvision(context.Background(), "ph")

	require.ErrorIs(t, err, worktree.ErrBranchHeldByManagedWorkspace)
	assert.NotErrorIs(t, err, worktree.ErrBranchStillHeld,
		"a managed holder cannot be detached, so it must not offer that remedy")
	assert.Contains(t, err.Error(), holderPath,
		"the refusal names the worktree holding the branch")
	assert.False(t, provisionCalled, "no provision while the branch is held")
}

func (f *fakeWorkspace) SetLock(
	_ context.Context,
	id string,
	locked *bool,
	_ bool,
) (domain.Workspace, error) {
	return domain.Workspace{ID: id, LockOverride: locked}, nil
}

// DeleteRepoWorkspaces exists so a repo delete can broadcast its tombstone the
// moment the ROW is gone and tear the workspaces down behind it. That only works
// if the cascade no longer needs the row — so it takes the repo path from the
// caller, and must still run the git teardown with it.
func TestDeleteRepoWorkspaces_UsesTheCallersPathWhenTheRepoRowIsGone(t *testing.T) {
	all := []domain.Workspace{
		{ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "alpha", WorktreePath: "/wt/a/worktree"},
		{ID: "w2", RepoID: "r2", ProjectID: "p1", Branch: "other", WorktreePath: "/wt/b/worktree"},
	}
	deleted := []string{}
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			for _, w := range all {
				if w.ID == id {
					return w, nil
				}
			}
			return domain.Workspace{}, apperr.ErrNotFound
		},
		DeleteFn: func(_ context.Context, id string) error {
			deleted = append(deleted, id)
			return nil
		},
	}
	g := &fakeGit{}
	// The repo store answers NOTHING: the row is already deleted, which is the
	// whole situation this method is for.
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{missing: true}, newNow(), fakeHome(t))

	_, err := uc.DeleteRepoWorkspaces(context.Background(), "r1", "/repo")
	require.NoError(t, err)

	assert.Equal(t, []string{"w1"}, deleted, "only the repo's own workspaces are removed")
	assert.Contains(t, g.ops(), "WorktreeRemove",
		"the git teardown must still run — skipping it strands a live worktree "+
			"registration in the user's own repository")
	for _, c := range g.calls {
		if c.op == "WorktreeRemove" {
			assert.Equal(t, "/repo", c.args[0],
				"and it must use the path the caller handed over")
		}
	}
}

// A workspace must still be removable when the alias cannot be worked out — the
// alias is derived state, and a broken link is litter the boot sweep clears, not
// a reason to strand a workspace.
func TestDeleteRepoWorkspaces_RemovesEvenWhenTheAliasCannotBeResolved(t *testing.T) {
	all := []domain.Workspace{
		{ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "alpha", WorktreePath: "/wt/a/worktree"},
	}
	deleted := []string{}
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			return all[0], nil
		},
		DeleteFn: func(_ context.Context, id string) error {
			deleted = append(deleted, id)
			return nil
		},
	}
	// The repo store fails outright: no slug, so no alias can be named.
	uc := worktree.New(ws, &fakeGit{}, &fakeProvider{},
		&fakeRepoStore{err: assert.AnError}, newNow(), fakeHome(t))

	_, err := uc.DeleteRepoWorkspaces(context.Background(), "r1", "/repo")
	require.NoError(t, err)
	assert.Equal(t, []string{"w1"}, deleted)
}

// A workspace with no branch has no alias to withdraw, and a placeholder has no
// worktree to remove — both still lose their row.
func TestDeleteRepoWorkspaces_HandlesPlaceholdersAndBranchlessRows(t *testing.T) {
	all := []domain.Workspace{
		{ID: "w-placeholder", RepoID: "r1", ProjectID: "p1", Branch: "held", WorktreePath: ""},
		{ID: "w-branchless", RepoID: "r1", ProjectID: "p1", WorktreePath: "/wt/b/worktree"},
	}
	deleted := []string{}
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			for _, w := range all {
				if w.ID == id {
					return w, nil
				}
			}
			return domain.Workspace{}, apperr.ErrNotFound
		},
		DeleteFn: func(_ context.Context, id string) error {
			deleted = append(deleted, id)
			return nil
		},
	}
	g := &fakeGit{}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{missing: true}, newNow(), fakeHome(t))

	_, err := uc.DeleteRepoWorkspaces(context.Background(), "r1", "/repo")
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{"w-placeholder", "w-branchless"}, deleted)
	// The placeholder contributes no git at all: it has no worktree of its own,
	// and its branch is held by something else that must not be touched.
	assert.Equal(t, 1, countOp(g.ops(), "WorktreeRemove"),
		"only the row that actually has a worktree reaches git")
}

// A listing failure is the one thing that stops the cascade: without the list
// there is nothing to walk, and reporting success would claim work never done.
func TestDeleteRepoWorkspaces_ReportsAListingFailure(t *testing.T) {
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return nil, assert.AnError },
	}
	uc := worktree.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome(t))

	_, err := uc.DeleteRepoWorkspaces(context.Background(), "r1", "/repo")
	require.Error(t, err)
}
