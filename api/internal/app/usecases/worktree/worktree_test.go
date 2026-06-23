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
	CreateFn          func(ctx context.Context, in workspace.CreateInput, now time.Time) (domain.Workspace, error)
	GetFn             func(ctx context.Context, id string) (domain.Workspace, error)
	ListFn            func(ctx context.Context) ([]domain.Workspace, error)
	ReparentFn        func(ctx context.Context, id, parentID, forkPointSha string, now time.Time) (domain.Workspace, error)
	UpdateForkPointFn func(ctx context.Context, id, forkPointSha string) (domain.Workspace, error)
	DeleteFn          func(ctx context.Context, id string) error
	SyncFn            func(ctx context.Context, in workspace.SyncInput, now time.Time) (domain.Workspace, error)
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
	return f.ListFn(ctx)
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

func (f *fakeWorkspace) UpdateForkPoint(
	ctx context.Context,
	id string,
	forkPointSha string,
) (domain.Workspace, error) {
	return f.UpdateForkPointFn(ctx, id, forkPointSha)
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

var _ workspace.Workspace = (*fakeWorkspace)(nil)

// --- fakes ---

type gitCall struct {
	op   string
	args []string
}

type fakeGit struct {
	enginegit.Engine
	calls           []gitCall
	addStartSha     string
	addErr          error
	revParseSha     string
	revParseErr     error
	mergeErr        error
	squashErr       error
	rebaseErr       error
	ffErr           error
	rebaseFFErr     error
	rebaseOnto      error
	removeErr       error
	deleteErr       error
	remoteExists    bool
	remoteExistsErr error
	fetchRefErr     error
	worktreeAddErr  error

	summaryAdded      int
	summaryDeleted    int
	summaryHasCommits bool
	summaryErr        error
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

func (f *fakeGit) WorktreeAdd(
	_ context.Context,
	repoPath string,
	worktreePath string,
	branch string,
) error {
	f.record("WorktreeAdd", repoPath, worktreePath, branch)
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

func (f *fakeGit) WorkingTreeSummary(
	_ context.Context,
	repoPath string,
	forkPointSha string,
) (int, int, bool, bool, error) {
	f.record("WorkingTreeSummary", repoPath, forkPointSha)
	return f.summaryAdded, f.summaryDeleted, false, f.summaryHasCommits, f.summaryErr
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
	path    string
	err     error
	missing bool
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
	return &domain.Repository{Path: f.path}, nil
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
	assert.Equal(t, []string{"RemoteBranchExists", "WorktreeAddBranch"}, g.ops())
	assert.Equal(t, []string{"/repo", created.WorktreePath, "feature/x", "develop"}, g.calls[1].args)
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
	assert.Equal(t, []string{"RemoteBranchExists", "WorktreeAddBranch"}, g.ops())
	assert.Equal(t, []string{"/repo", created.WorktreePath, "feature/x", "develop"}, g.calls[1].args)
	// Fork point comes from the create-local startSha.
	assert.Equal(t, "localfork", created.ForkPointSha)
}

// TestCreateChild_RemoteBranchExists_ChecksOut verifies the spec-§3 decision:
// when the branch already exists on the remote, CreateChild fetches it and
// checks out the existing remote branch (WorktreeAdd), and the fork point comes
// from RevParse of the resolved origin/<branch> ref — NOT from WorktreeAddBranch.
func TestCreateChild_RemoteBranchExists_ChecksOut(t *testing.T) {
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
	assert.Equal(t, []string{"RemoteBranchExists", "FetchRef", "RevParse", "WorktreeAdd"}, g.ops())
	assert.Equal(t, []string{"/repo", "feature/x"}, g.calls[0].args)
	assert.Equal(t, []string{"/repo", "feature/x"}, g.calls[1].args)
	assert.Equal(t, []string{"/repo", "origin/feature/x"}, g.calls[2].args)
	assert.Equal(t, []string{"/repo", created.WorktreePath, "feature/x"}, g.calls[3].args)
	// WorktreeAddBranch must NOT be called on the checkout path.
	assert.NotContains(t, g.ops(), "WorktreeAddBranch")
	assert.Equal(t, "remotefork", created.ForkPointSha)
}

func TestCreateChild_RemoteBranchExistsError(t *testing.T) {
	g := &fakeGit{remoteExistsErr: errBoom}
	uc := worktree.New(&fakeWorkspace{}, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoPath: "/r", Branch: "b", ParentBranch: "develop",
	})
	require.ErrorIs(t, err, errBoom)
}

func TestCreateChild_FetchRefError(t *testing.T) {
	g := &fakeGit{remoteExists: true, fetchRefErr: errBoom}
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
	uc := worktree.New(&fakeWorkspace{}, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoPath: "/repo", Branch: "main", ParentBranch: "main",
	})
	require.ErrorIs(t, err, errBoom)
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
// SyncWorkingTreeState HasConflicts write) instead of recording a pending merge.
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
	assert.Equal(t, []string{"Merge"}, g.ops())
}

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
	assert.Equal(t, []string{"RebaseThenFFMerge"}, g.ops())
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
	newParent := domain.Workspace{ID: "np", Status: domain.WorkspaceStatusLocked}
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

func TestDeleteCascade_RepoPathError(t *testing.T) {
	all := []domain.Workspace{{ID: "root", RepoID: "r"}}
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
	}
	uc := worktree.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{err: errBoom}, newNow(), fakeHome())
	require.ErrorIs(t, uc.DeleteCascade(context.Background(), "root"), errBoom)
}

func TestDeleteCascade_MissingRepoRow_ErrorsNoPanic(t *testing.T) {
	all := []domain.Workspace{{ID: "root", RepoID: "r", WorktreePath: "/wt", Branch: "b"}}
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
	}
	// FindByKey returns (nil, nil) for a missing repo row; dereferencing it
	// previously panicked. The usecase must surface a plain error instead.
	uc := worktree.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{missing: true}, newNow(), fakeHome())
	err := uc.DeleteCascade(context.Background(), "root")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "repo r not found")
}

func TestDeleteCascade_WorktreeRemoveError(t *testing.T) {
	all := []domain.Workspace{{ID: "root", RepoID: "r", WorktreePath: "/wt", Branch: "b"}}
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
	}
	g := &fakeGit{removeErr: errBoom}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())
	require.ErrorIs(t, uc.DeleteCascade(context.Background(), "root"), errBoom)
}

func TestDeleteCascade_BranchDeleteError(t *testing.T) {
	all := []domain.Workspace{{ID: "root", RepoID: "r", WorktreePath: "/wt", Branch: "b"}}
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
	}
	g := &fakeGit{deleteErr: errBoom}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())
	require.ErrorIs(t, uc.DeleteCascade(context.Background(), "root"), errBoom)
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
