package worktree_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
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

func (f *fakeWorkspace) SetParentFromPR(_ context.Context, _ string, _ string) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

func (f *fakeWorkspace) SetLastError(_ context.Context, _ string, _ string) (domain.Workspace, error) {
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
	calls             []gitCall
	addStartSha       string
	addErr            error
	revParseSha       string
	revParseErr       error
	mergeErr          error
	squashErr         error
	rebaseErr         error
	ffErr             error
	rebaseFFErr       error
	rebaseOnto        error
	operationAbortErr error
	removeErr         error
	deleteErr         error
	remoteExists         bool
	remoteExistsByBranch map[string]bool // overrides remoteExists per branch when non-nil
	remoteExistsErr      error
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

func (f *fakeGit) RevParse(
	_ context.Context,
	repoPath string,
	rev string,
) (string, error) {
	f.record("RevParse", repoPath, rev)
	return f.revParseSha, f.revParseErr
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
	return &domain.Repository{Path: f.path, DefaultBranch: f.defaultBranch}, nil
}

// perPathSummaryGit is a fakeGit that varies WorkingTreeSummary's hasConflicts
// (and optionally its error) by worktree path, so a ReconcileAll test can drive
// distinct per-workspace conflict/error outcomes in a single sweep.
type perPathSummaryGit struct {
	fakeGit
	conflictsByPath map[string]bool
	errByPath       map[string]error
}

func (g *perPathSummaryGit) WorkingTreeSummary(
	_ context.Context,
	repoPath string,
	forkPointSha string,
) (int, int, bool, bool, error) {
	g.record("WorkingTreeSummary", repoPath, forkPointSha)
	if err, ok := g.errByPath[repoPath]; ok && err != nil {
		return 0, 0, false, false, err
	}
	return 0, 0, g.conflictsByPath[repoPath], false, nil
}

// perRepoStore resolves a repository main path per RepoID, so a ReconcileAll
// test can map workspaces across distinct repos.
type perRepoStore struct {
	store.Store[domain.Repository, string]
	paths map[string]string
}

func (s *perRepoStore) FindByKey(
	_ context.Context,
	key string,
) (*domain.Repository, error) {
	p, ok := s.paths[key]
	if !ok {
		return nil, nil
	}
	return &domain.Repository{Path: p}, nil
}

func newNow() func() time.Time {
	return func() time.Time { return time.Unix(0, 0) }
}

func fakeHome() func() (string, error) {
	return func() (string, error) { return "/tmp/crowbar-test", nil }
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
	uc := worktree.New(ws, g, &fakeProvider{protected: []string{"main"}}, &fakeRepoStore{}, newNow(), fakeHome())

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
	// Parent check (develop → not on remote) + child check (feature/x → not on remote).
	assert.Equal(t, []string{"RemoteBranchExists", "RemoteBranchExists", "WorktreeAddBranch"}, g.ops())
	assert.Equal(t, []string{"/repo", created.WorktreePath, "feature/x", "develop"}, g.calls[2].args)
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
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

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
	uc := worktree.New(ws, g, &fakeProvider{protected: []string{"feature/x"}}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoPath: "/repo", RemoteURL: "https://github.com/test/repo.git", Branch: "feature/x", ParentBranch: "develop",
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
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoID:       "r1",
		ProjectID:    "p1",
		RepoPath:     "/repo",
		Branch:       "feature/x",
		ParentID:     "w-parent",
		ParentBranch: "develop",
	})
	require.NoError(t, err)
	// Parent check (develop → not on remote) + child check (feature/x → not on remote).
	assert.Equal(t, []string{"RemoteBranchExists", "RemoteBranchExists", "WorktreeAddBranch"}, g.ops())
	assert.Equal(t, []string{"/repo", created.WorktreePath, "feature/x", "develop"}, g.calls[2].args)
	// Fork point comes from the create-local startSha.
	assert.Equal(t, "localfork", created.ForkPointSha)
}

// TestCreateChild_RemoteBranchExists_ChecksOut verifies the spec-§3 decision:
// when the branch already exists on the remote, CreateChild fast-forwards it
// and checks out the existing remote branch (WorktreeAdd), and the fork point
// comes from RevParse of the resolved origin/<branch> ref — NOT WorktreeAddBranch.
// The parent branch is also fast-forwarded first (best-effort) before the child
// remote-exists check, so the child inherits a fresh parent tip.
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
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoID:       "r1",
		ProjectID:    "p1",
		RepoPath:     "/repo",
		Branch:       "feature/x",
		ParentID:     "w-parent",
		ParentBranch: "develop",
	})
	require.NoError(t, err)
	// Op sequence: parent fast-forward check + fast-forward, then child exists
	// check + fast-forward + rev-parse + worktree-add.
	assert.Equal(t, []string{
		"RemoteBranchExists",  // parent exists?
		"FastForwardBranch",   // fast-forward parent
		"RemoteBranchExists",  // child exists?
		"FastForwardBranch",   // fast-forward child
		"RevParse",
		"WorktreeAdd",
	}, g.ops())
	assert.Equal(t, []string{"/repo", "develop"}, g.calls[0].args)
	assert.Equal(t, []string{"/repo", "develop"}, g.calls[1].args)
	assert.Equal(t, []string{"/repo", "feature/x"}, g.calls[2].args)
	assert.Equal(t, []string{"/repo", "feature/x"}, g.calls[3].args)
	assert.Equal(t, []string{"/repo", "origin/feature/x"}, g.calls[4].args)
	assert.Equal(t, []string{"/repo", created.WorktreePath, "feature/x"}, g.calls[5].args)
	// WorktreeAddBranch must NOT be called on the checkout path.
	assert.NotContains(t, g.ops(), "WorktreeAddBranch")
	assert.Equal(t, "remotefork", created.ForkPointSha)
}

// TestCreateChild_NewBranch_ParentFastForwarded verifies that when creating a
// brand-new branch (not on the remote), the parent is fast-forwarded first so
// the child starts at the latest parent tip, not a stale local ref.
func TestCreateChild_NewBranch_ParentFastForwarded(t *testing.T) {
	// Parent exists on remote; child does NOT — use per-branch map.
	g := &fakeGit{
		remoteExistsByBranch: map[string]bool{"develop": true},
		addStartSha:          "newsha",
	}
	uc := worktree.New(&fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			return domain.Workspace{ID: in.ID}, nil
		},
	}, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

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
		"RemoteBranchExists", // parent exists? → true
		"FastForwardBranch",  // fast-forward parent
		"RemoteBranchExists", // child exists? → false
		"WorktreeAddBranch",  // create new branch from fast-forwarded parent
	}, g.ops())
	assert.Equal(t, []string{"/repo", "develop"}, g.calls[0].args)
	assert.Equal(t, []string{"/repo", "develop"}, g.calls[1].args)
	assert.Equal(t, []string{"/repo", "my-feature"}, g.calls[2].args)
}

func TestCreateChild_RemoteBranchExistsError(t *testing.T) {
	g := &fakeGit{remoteExistsErr: errBoom}
	uc := worktree.New(&fakeWorkspace{}, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoPath: "/r", Branch: "b", ParentBranch: "develop",
	})
	require.ErrorIs(t, err, errBoom)
}

func TestCreateChild_FastForwardBranchError(t *testing.T) {
	// FastForwardBranch failing on the CHILD branch (checkoutRemoteBranch path) is fatal.
	// remoteExists=true means both parent and child are "on remote", so both
	// fast-forward calls run; the child fast-forward error surfaces.
	g := &fakeGit{remoteExists: true, fastForwardBranchErr: errBoom}
	uc := worktree.New(&fakeWorkspace{}, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoPath: "/r", Branch: "b", ParentBranch: "develop",
	})
	require.ErrorIs(t, err, errBoom)
}

func TestCreateChild_CheckoutWorktreeAddError(t *testing.T) {
	g := &fakeGit{remoteExists: true, revParseSha: "s", worktreeAddErr: errBoom}
	uc := worktree.New(&fakeWorkspace{}, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoPath: "/r", Branch: "b", ParentBranch: "develop",
	})
	require.ErrorIs(t, err, errBoom)
}

func TestCreateChild_CheckoutRevParseError(t *testing.T) {
	g := &fakeGit{remoteExists: true, revParseErr: errBoom}
	uc := worktree.New(&fakeWorkspace{}, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoPath: "/r", Branch: "b", ParentBranch: "develop",
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
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

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
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
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
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

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
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo", RemoteURL: "https://github.com/test/repo.git",
		Branch: "develop", ParentID: "", ParentBranch: "develop",
	})

	require.NoError(t, err, "importing the default branch must succeed by detaching the main folder")
	assert.Contains(t, g.ops(), "DetachWorktree", "the main folder must be detached to free the branch")
	var addCount int
	for _, op := range g.ops() {
		if op == "WorktreeAdd" {
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
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

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
	uc := worktree.New(ws, g, &fakeProvider{}, repos, newNow(), fakeHome())

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
	uc := worktree.New(ws, g, &fakeProvider{}, repos, newNow(), fakeHome())

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
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

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
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo", Branch: "develop", ParentBranch: "develop",
	})
	require.NoError(t, err)
	assert.Equal(t, "/repo", created.WorktreePath)
}

func TestCreateChild_WorktreeAddError(t *testing.T) {
	g := &fakeGit{addErr: errBoom}
	ws := &fakeWorkspace{}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{RepoPath: "/r", RemoteURL: "https://github.com/test/repo.git", Branch: "b"})
	require.ErrorIs(t, err, errBoom)
}

func TestCreateChild_ProviderError(t *testing.T) {
	g := &fakeGit{addStartSha: "s"}
	ws := &fakeWorkspace{}
	uc := worktree.New(ws, g, &fakeProvider{err: errBoom}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{RepoPath: "/r", RemoteURL: "https://github.com/test/repo.git", Branch: "b"})
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
	uc := worktree.New(mergeWS(child, parent, nil), g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.ErrorIs(t, err, worktree.ErrParentLocked)
	assert.Empty(t, g.calls)
}

func TestMergeIntoParent_RejectsRebaseForNonLeafChild(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop"}
	grandchild := domain.Workspace{ID: "gc", ParentID: "c"}
	g := &fakeGit{}
	uc := worktree.New(mergeWS(child, parent, []domain.Workspace{grandchild}), g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
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
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

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
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

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
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
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
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

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
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

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
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

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
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
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
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
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
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
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
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
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
	uc := worktree.New(mergeWS(child, parent, nil), g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.ErrorIs(t, err, errBoom)
}

func TestMergeIntoParent_GetChildError(t *testing.T) {
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, _ string) (domain.Workspace, error) {
			return domain.Workspace{}, errBoom
		},
	}
	uc := worktree.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
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
	uc := worktree.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
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
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
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
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
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
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.Reparent(context.Background(), "c", "np")
	require.NoError(t, err) // not blocked by a phantom self-child
}

func TestReparent_RejectsLockedNewParent(t *testing.T) {
	child := domain.Workspace{ID: "c"}
	newParent := domain.Workspace{ID: "np", Status: domain.WorkspaceStatusLocked, WorktreePath: "/np"}
	ws := reparentWS(child, newParent, nil)
	g := &fakeGit{}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.Reparent(context.Background(), "c", "np")
	require.ErrorIs(t, err, worktree.ErrNewParentLocked)
	assert.Empty(t, g.calls)
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
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

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
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.Reparent(context.Background(), "c", "np")
	require.ErrorIs(t, err, errBoom)
}

func TestReparent_GetChildError(t *testing.T) {
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, _ string) (domain.Workspace, error) {
			return domain.Workspace{}, errBoom
		},
	}
	uc := worktree.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
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
	uc := worktree.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
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
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome(),
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
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())

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
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())
	require.ErrorIs(t, uc.DeleteCascade(context.Background(), "root"), worktree.ErrWorkspaceLocked)
	assert.Empty(t, g.calls)
}

func TestDeleteCascade_ListError(t *testing.T) {
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return nil, errBoom },
	}
	uc := worktree.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
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
	uc := worktree.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{err: errBoom}, newNow(), fakeHome())
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
	uc := worktree.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{missing: true}, newNow(), fakeHome())
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
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())
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
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())
	require.NoError(t, uc.DeleteCascade(context.Background(), "root"))
	assert.Equal(t, []string{"root"}, deleted)
}

// --- ReconcileAll (H19 startup recovery sweep) ---

// TestReconcileAll_ClearsStalePRConflicts proves the core of H19: a workspace
// persisted as pr-conflicts whose worktree no longer has real conflict markers
// (fakeGit reports hasConflicts=false) gets ResolveConflicts'd, dropping the
// stale warning. A workspace whose worktree STILL has conflicts is left alone.
func TestReconcileAll_ClearsStalePRConflicts(t *testing.T) {
	stale := domain.Workspace{ID: "stale", RepoID: "r", WorktreePath: "/wt/stale", Status: domain.WorkspaceStatusPRConflicts}
	real := domain.Workspace{ID: "real", RepoID: "r", WorktreePath: "/wt/real", Status: domain.WorkspaceStatusPRConflicts}
	all := []domain.Workspace{stale, real}

	// fakeGit reports NO conflicts for every workspace, so only the resync drives
	// the decision: with hasConflicts=false both stale and real are candidates to
	// clear. To distinguish, drive per-path conflict state via a custom git double.
	g := &perPathSummaryGit{conflictsByPath: map[string]bool{
		"/wt/stale": false,
		"/wt/real":  true,
	}}
	var resolved []string
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
		ResolveConflictsFn: func(_ context.Context, id string, _ time.Time) (domain.Workspace, error) {
			resolved = append(resolved, id)
			return domain.Workspace{ID: id}, nil
		},
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())

	require.NoError(t, uc.ReconcileAll(context.Background()))
	assert.Equal(t, []string{"stale"}, resolved,
		"only the workspace with no real conflicts must be ResolveConflicts'd")
}

// TestReconcileAll_KeepsPRConflictsWhenOpInProgress proves R13: a workspace that
// is MID conflict-resolution — the conflict markers were resolved and staged so
// WorkingTreeSummary reports hasConflicts=false, but the rebase/merge was never
// continued so an in-progress op is still on disk — must KEEP its pr-conflicts.
// Clearing it would drop the signal that the operation is unfinished.
func TestReconcileAll_KeepsPRConflictsWhenOpInProgress(t *testing.T) {
	midRes := domain.Workspace{ID: "midres", RepoID: "r", WorktreePath: "/wt/midres", Status: domain.WorkspaceStatusPRConflicts}
	// No unresolved markers in the tree (hasConflicts=false) but a rebase is still
	// in progress (the user staged the resolution and stopped).
	g := &fakeGit{opInProgress: "rebase"}
	var resolved []string
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{midRes}, nil
		},
		ResolveConflictsFn: func(_ context.Context, id string, _ time.Time) (domain.Workspace, error) {
			resolved = append(resolved, id)
			return domain.Workspace{ID: id}, nil
		},
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())

	require.NoError(t, uc.ReconcileAll(context.Background()))
	assert.Empty(t, resolved,
		"a workspace mid conflict-resolution (in-progress rebase) must keep pr-conflicts, not be cleared")
}

// TestReconcileAll_ResyncsSummaryAndSkipsNoPathRows proves ReconcileAll resyncs
// each worktree-backed workspace's summary from git and SKIPS rows with no
// on-disk worktree (virtual/no-path), which have nothing to read from disk.
func TestReconcileAll_ResyncsSummaryAndSkipsNoPathRows(t *testing.T) {
	withPath := domain.Workspace{ID: "wp", RepoID: "r", WorktreePath: "/wt/wp", ForkPointSha: "fork"}
	noPath := domain.Workspace{ID: "np", RepoID: "r", WorktreePath: ""}
	all := []domain.Workspace{withPath, noPath}

	g := &fakeGit{summaryAdded: 2, summaryDeleted: 1, summaryHasCommits: true}
	var synced []workspace.SyncInput
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
		SyncFn: func(_ context.Context, in workspace.SyncInput, _ time.Time) (domain.Workspace, error) {
			synced = append(synced, in)
			return domain.Workspace{}, nil
		},
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())

	require.NoError(t, uc.ReconcileAll(context.Background()))
	// Only the worktree-backed row is resynced; the no-path row never reads git.
	require.Len(t, synced, 1)
	assert.Equal(t, "wp", synced[0].ID)
	assert.Equal(t, 2, synced[0].Added)
	assert.Equal(t, 1, synced[0].Deleted)
	assert.True(t, synced[0].HasCommits)
	// WorkingTreeSummary must run only for the worktree-backed row.
	assert.Equal(t, []string{"/wt/wp", "fork"}, g.calls[0].args)
}

// TestReconcileAll_PrunesEachDistinctRepoOnce proves orphaned worktrees are
// reaped: WorktreePrune runs once per DISTINCT repo main path among the
// workspaces, never per-workspace.
func TestReconcileAll_PrunesEachDistinctRepoOnce(t *testing.T) {
	all := []domain.Workspace{
		{ID: "a", RepoID: "r1", WorktreePath: "/wt/a"},
		{ID: "b", RepoID: "r1", WorktreePath: "/wt/b"},
		{ID: "c", RepoID: "r2", WorktreePath: "/wt/c"},
	}
	g := &fakeGit{}
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
	}
	// repoPathFor resolves per RepoID; map both repos to distinct main paths.
	repos := &perRepoStore{paths: map[string]string{"r1": "/repo1", "r2": "/repo2"}}
	uc := worktree.New(ws, g, &fakeProvider{}, repos, newNow(), fakeHome())

	require.NoError(t, uc.ReconcileAll(context.Background()))
	var pruned []string
	for _, c := range g.calls {
		if c.op == "WorktreePrune" {
			pruned = append(pruned, c.args[0])
		}
	}
	assert.ElementsMatch(t, []string{"/repo1", "/repo2"}, pruned,
		"prune must run exactly once per distinct repo main path")
}

// TestReconcileAll_BestEffort proves a per-workspace summary failure does NOT
// abort the sweep: the second workspace is still reconciled and pruning still
// runs, and ReconcileAll returns nil (boot is never failed).
func TestReconcileAll_BestEffort(t *testing.T) {
	all := []domain.Workspace{
		{ID: "boom", RepoID: "r", WorktreePath: "/wt/boom", Status: domain.WorkspaceStatusPRConflicts},
		{ID: "ok", RepoID: "r", WorktreePath: "/wt/ok", Status: domain.WorkspaceStatusPRConflicts},
	}
	// First workspace's summary errors; second succeeds with no conflicts.
	g := &perPathSummaryGit{
		errByPath: map[string]error{"/wt/boom": errBoom},
	}
	var resolved []string
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
		ResolveConflictsFn: func(_ context.Context, id string, _ time.Time) (domain.Workspace, error) {
			resolved = append(resolved, id)
			return domain.Workspace{ID: id}, nil
		},
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())

	require.NoError(t, uc.ReconcileAll(context.Background()),
		"a per-workspace failure must not fail boot")
	assert.Equal(t, []string{"ok"}, resolved,
		"the failing workspace is skipped but the rest are still reconciled")
}

// TestReconcileAll_ListError_ReturnsNil proves a List failure is non-fatal: the
// sweep logs and returns nil rather than failing boot.
func TestReconcileAll_ListError_ReturnsNil(t *testing.T) {
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return nil, errBoom },
	}
	uc := worktree.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	require.NoError(t, uc.ReconcileAll(context.Background()),
		"a list failure must not fail boot")
}

func TestMergeIntoParent_SetPRConflictsError(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop"}
	g := &fakeGit{mergeErr: enginegit.ErrConflict}
	ws := mergeWS(child, parent, nil)
	ws.SyncFn = func(_ context.Context, _ workspace.SyncInput, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{}, errBoom
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
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
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
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
	uc := worktree.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
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
	uc := worktree.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.Reparent(context.Background(), "c", "np")
	require.ErrorIs(t, err, errBoom)
}

func TestMergeIntoParent_RevParseError(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop"}
	g := &fakeGit{revParseErr: errBoom}
	uc := worktree.New(mergeWS(child, parent, nil), g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.ErrorIs(t, err, errBoom)
}

func TestReparent_RevParseError(t *testing.T) {
	child := domain.Workspace{ID: "c", Branch: "feat", WorktreePath: "/cw"}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np"}
	g := &fakeGit{revParseErr: errBoom}
	uc := worktree.New(reparentWS(child, newParent, nil), g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
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
	uc := worktree.New(mergeWS(child, parent, nil), g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
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
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
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
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
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
	uc := worktree.New(ws, g, &fakeProvider{}, repos, newNow(), fakeHome())

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
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

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
			return domain.Workspace{ID: id, RepoID: "r1", ProjectID: "p1", Branch: "develop",
				Status: domain.WorkspaceStatusLocked, HeldByPath: "/repo"}, nil
		},
		ProvisionInPlaceFn: func(id, path, sha string) (domain.Workspace, error) {
			gotID, gotPath, gotSha = id, path, sha
			return domain.Workspace{ID: id, WorktreePath: path, ForkPointSha: sha, Status: domain.WorkspaceStatusLocked}, nil
		},
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())

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
			return domain.Workspace{ID: id, RepoID: "r1", Branch: "develop",
				Status: domain.WorkspaceStatusLocked, HeldByPath: "/repo"}, nil
		},
		ProvisionInPlaceFn: func(_, _, _ string) (domain.Workspace, error) {
			provisionCalled = true
			return domain.Workspace{}, nil
		},
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())

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
			return domain.Workspace{ID: id, RepoID: "r1", ProjectID: "p1", Branch: "develop",
				Status: domain.WorkspaceStatusLocked, HeldByPath: "/repo"}, nil
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
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())

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
			return domain.Workspace{ID: id, RepoID: "r1", Branch: "develop",
				Status: domain.WorkspaceStatusLocked, HeldByPath: "/repo"}, nil
		},
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{{ID: "home", RepoID: "r1", IsDefault: true}}, nil
		},
		ClearBranchFn:      func(_ string) (domain.Workspace, error) { cleared = true; return domain.Workspace{}, nil },
		ProvisionInPlaceFn: func(_, _, _ string) (domain.Workspace, error) { provisioned = true; return domain.Workspace{}, nil },
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())

	_, err := uc.DetachHolder(context.Background(), "ph")
	require.Error(t, err)
	assert.False(t, cleared, "no ClearBranch after a failed detach")
	assert.False(t, provisioned, "no provision after a failed detach")
}
