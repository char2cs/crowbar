package usecases_test

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
	"github.com/char2cs/crowbar/api/internal/app/usecases"
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
	SetPendingMergeFn func(ctx context.Context, id string, s gitdomain.MergeStrategy, target string) (domain.Workspace, error)
	DeleteFn          func(ctx context.Context, id string) error
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

func (f *fakeWorkspace) SetPendingMerge(
	ctx context.Context,
	id string,
	s gitdomain.MergeStrategy,
	target string,
) (domain.Workspace, error) {
	return f.SetPendingMergeFn(ctx, id, s, target)
}

func (f *fakeWorkspace) Delete(
	ctx context.Context,
	id string,
) error {
	return f.DeleteFn(ctx, id)
}

func (f *fakeWorkspace) SyncWorkingTreeState(
	_ context.Context,
	_ workspace.SyncInput,
	_ time.Time,
) (domain.Workspace, error) {
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

func (f *fakeWorkspace) ClearPendingMerge(
	_ context.Context,
	_ string,
) (domain.Workspace, error) {
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
	calls       []gitCall
	addStartSha string
	addErr      error
	revParseSha string
	revParseErr error
	mergeErr    error
	squashErr   error
	rebaseErr   error
	ffErr       error
	rebaseOnto  error
	removeErr   error
	deleteErr   error
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
	path string
	err  error
}

func (f *fakeRepoStore) FindByKey(
	_ context.Context,
	_ string,
) (*domain.Repository, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &domain.Repository{Path: f.path}, nil
}

func newNow() func() time.Time {
	return func() time.Time { return time.Unix(0, 0) }
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
	uc := usecases.NewWorktreeUsecase(ws, g, &fakeProvider{protected: []string{"main"}}, &fakeRepoStore{}, newNow())

	out, err := uc.CreateChild(context.Background(), usecases.CreateChildInput{
		RepoID:       "r1",
		ProjectID:    "p1",
		RepoPath:     "/repo",
		Branch:       "feature/x",
		ParentID:     "w-parent",
		ParentBranch: "develop",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, out.ID)
	assert.Equal(t, "sha123", created.ForkPointSha)
	assert.False(t, created.Locked)
	assert.Equal(t, "w-parent", created.ParentID)
	assert.Equal(t, []string{"WorktreeAddBranch"}, g.ops())
	assert.Equal(t, []string{"/repo", created.WorktreePath, "feature/x", "develop"}, g.calls[0].args)
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
	uc := usecases.NewWorktreeUsecase(ws, g, &fakeProvider{protected: []string{"feature/x"}}, &fakeRepoStore{}, newNow())

	_, err := uc.CreateChild(context.Background(), usecases.CreateChildInput{
		RepoPath: "/repo", Branch: "feature/x", ParentBranch: "develop",
	})
	require.NoError(t, err)
	assert.True(t, created.Locked)
}

func TestCreateChild_WorktreeAddError(t *testing.T) {
	g := &fakeGit{addErr: errBoom}
	ws := &fakeWorkspace{}
	uc := usecases.NewWorktreeUsecase(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow())
	_, err := uc.CreateChild(context.Background(), usecases.CreateChildInput{RepoPath: "/r", Branch: "b"})
	require.ErrorIs(t, err, errBoom)
}

func TestCreateChild_ProviderError(t *testing.T) {
	g := &fakeGit{addStartSha: "s"}
	ws := &fakeWorkspace{}
	uc := usecases.NewWorktreeUsecase(ws, g, &fakeProvider{err: errBoom}, &fakeRepoStore{}, newNow())
	_, err := uc.CreateChild(context.Background(), usecases.CreateChildInput{RepoPath: "/r", Branch: "b"})
	require.ErrorIs(t, err, errBoom)
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
	parent := domain.Workspace{ID: "p", Locked: true, WorktreePath: "/pw"}
	g := &fakeGit{}
	uc := usecases.NewWorktreeUsecase(mergeWS(child, parent, nil), g, &fakeProvider{}, &fakeRepoStore{}, newNow())
	_, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.ErrorIs(t, err, usecases.ErrParentLocked)
	assert.Empty(t, g.calls)
}

func TestMergeIntoParent_RejectsRebaseForNonLeafChild(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop"}
	grandchild := domain.Workspace{ID: "gc", ParentID: "c"}
	g := &fakeGit{}
	uc := usecases.NewWorktreeUsecase(mergeWS(child, parent, []domain.Workspace{grandchild}), g, &fakeProvider{}, &fakeRepoStore{}, newNow())
	_, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyRebase)
	require.ErrorIs(t, err, usecases.ErrRebaseNonLeaf)
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
	uc := usecases.NewWorktreeUsecase(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow())

	res, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.NoError(t, err)
	assert.Equal(t, "ptip", res.ParentTipSha)
	assert.False(t, res.ConflictsPending)
	assert.Equal(t, []string{"Merge", "RevParse"}, g.ops())
	assert.Equal(t, []string{"/pw", "feat"}, g.calls[0].args)
	assert.Equal(t, []string{"/pw", "HEAD"}, g.calls[1].args)
	assert.Equal(t, "c", updatedID)
	assert.Equal(t, "ptip", updatedSha)
}

func TestMergeIntoParent_SquashStrategy_RunsInParent(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop"}
	g := &fakeGit{revParseSha: "ptip"}
	ws := mergeWS(child, parent, nil)
	ws.UpdateForkPointFn = func(_ context.Context, _, _ string) (domain.Workspace, error) {
		return domain.Workspace{}, nil
	}
	uc := usecases.NewWorktreeUsecase(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow())

	_, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategySquash)
	require.NoError(t, err)
	assert.Equal(t, []string{"MergeSquash", "RevParse"}, g.ops())
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
	uc := usecases.NewWorktreeUsecase(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow())

	_, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyRebase)
	require.NoError(t, err)
	assert.Equal(t, []string{"Rebase", "MergeFFOnly", "RevParse"}, g.ops())
	assert.Equal(t, []string{"/cw", "develop"}, g.calls[0].args)
	assert.Equal(t, []string{"/pw", "feat"}, g.calls[1].args)
}

func TestMergeIntoParent_Conflict_SetsPendingMerge(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop"}
	g := &fakeGit{mergeErr: enginegit.ErrConflict}
	ws := mergeWS(child, parent, nil)
	var pendID, pendTarget string
	var pendStrat gitdomain.MergeStrategy
	ws.SetPendingMergeFn = func(_ context.Context, id string, s gitdomain.MergeStrategy, target string) (domain.Workspace, error) {
		pendID, pendStrat, pendTarget = id, s, target
		return domain.Workspace{}, nil
	}
	uc := usecases.NewWorktreeUsecase(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow())

	res, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.NoError(t, err)
	assert.True(t, res.ConflictsPending)
	assert.Empty(t, res.ParentTipSha)
	assert.Equal(t, "c", pendID)
	assert.Equal(t, gitdomain.MergeStrategyMerge, pendStrat)
	assert.Equal(t, "p", pendTarget)
	assert.Equal(t, []string{"Merge"}, g.ops())
}

func TestMergeIntoParent_RebaseConflict_SetsPendingMerge(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat", WorktreePath: "/cw"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop"}
	g := &fakeGit{rebaseErr: enginegit.ErrConflict}
	ws := mergeWS(child, parent, nil)
	ws.SetPendingMergeFn = func(_ context.Context, _ string, _ gitdomain.MergeStrategy, _ string) (domain.Workspace, error) {
		return domain.Workspace{}, nil
	}
	uc := usecases.NewWorktreeUsecase(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow())
	res, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyRebase)
	require.NoError(t, err)
	assert.True(t, res.ConflictsPending)
	assert.Equal(t, []string{"Rebase"}, g.ops())
}

func TestMergeIntoParent_NonConflictError_Propagates(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop"}
	g := &fakeGit{mergeErr: errBoom}
	uc := usecases.NewWorktreeUsecase(mergeWS(child, parent, nil), g, &fakeProvider{}, &fakeRepoStore{}, newNow())
	_, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.ErrorIs(t, err, errBoom)
}

func TestMergeIntoParent_GetChildError(t *testing.T) {
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, _ string) (domain.Workspace, error) {
			return domain.Workspace{}, errBoom
		},
	}
	uc := usecases.NewWorktreeUsecase(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow())
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
	uc := usecases.NewWorktreeUsecase(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow())
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
	uc := usecases.NewWorktreeUsecase(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow())
	_, err := uc.Reparent(context.Background(), "c", "np")
	require.ErrorIs(t, err, usecases.ErrChildHasChildren)
	assert.Empty(t, g.calls)
}

func TestReparent_RejectsLockedNewParent(t *testing.T) {
	child := domain.Workspace{ID: "c"}
	newParent := domain.Workspace{ID: "np", Locked: true}
	ws := reparentWS(child, newParent, nil)
	g := &fakeGit{}
	uc := usecases.NewWorktreeUsecase(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow())
	_, err := uc.Reparent(context.Background(), "c", "np")
	require.ErrorIs(t, err, usecases.ErrNewParentLocked)
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
	uc := usecases.NewWorktreeUsecase(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow())

	_, err := uc.Reparent(context.Background(), "c", "np")
	require.NoError(t, err)
	assert.Equal(t, []string{"RevParse", "RebaseOnto"}, g.ops())
	assert.Equal(t, []string{"/np", "HEAD"}, g.calls[0].args)
	assert.Equal(t, []string{"/cw", "ntip", "fork", "feat"}, g.calls[1].args)
	assert.Equal(t, "c", rID)
	assert.Equal(t, "np", rParent)
	assert.Equal(t, "ntip", rSha)
}

func TestReparent_RebaseOntoError(t *testing.T) {
	child := domain.Workspace{ID: "c", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "fork"}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np"}
	ws := reparentWS(child, newParent, nil)
	g := &fakeGit{revParseSha: "ntip", rebaseOnto: errBoom}
	uc := usecases.NewWorktreeUsecase(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow())
	_, err := uc.Reparent(context.Background(), "c", "np")
	require.ErrorIs(t, err, errBoom)
}

func TestReparent_GetChildError(t *testing.T) {
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, _ string) (domain.Workspace, error) {
			return domain.Workspace{}, errBoom
		},
	}
	uc := usecases.NewWorktreeUsecase(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow())
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
	uc := usecases.NewWorktreeUsecase(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow())
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

func TestDeleteCascade_DeepestFirstSkippingLocked(t *testing.T) {
	all := []domain.Workspace{
		{ID: "root", RepoID: "r", Branch: "b-root", WorktreePath: "/wt/root"},
		{ID: "a", ParentID: "root", RepoID: "r", Branch: "b-a", WorktreePath: "/wt/a"},
		{ID: "b", ParentID: "a", Locked: true, RepoID: "r", Branch: "b-b", WorktreePath: "/wt/b"},
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
	uc := usecases.NewWorktreeUsecase(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow())

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

func TestDeleteCascade_ListError(t *testing.T) {
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return nil, errBoom },
	}
	uc := usecases.NewWorktreeUsecase(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow())
	require.ErrorIs(t, uc.DeleteCascade(context.Background(), "root"), errBoom)
}

func TestDeleteCascade_RepoPathError(t *testing.T) {
	all := []domain.Workspace{{ID: "root", RepoID: "r"}}
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
	}
	uc := usecases.NewWorktreeUsecase(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{err: errBoom}, newNow())
	require.ErrorIs(t, uc.DeleteCascade(context.Background(), "root"), errBoom)
}

func TestDeleteCascade_WorktreeRemoveError(t *testing.T) {
	all := []domain.Workspace{{ID: "root", RepoID: "r", WorktreePath: "/wt", Branch: "b"}}
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
	}
	g := &fakeGit{removeErr: errBoom}
	uc := usecases.NewWorktreeUsecase(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow())
	require.ErrorIs(t, uc.DeleteCascade(context.Background(), "root"), errBoom)
}

func TestDeleteCascade_BranchDeleteError(t *testing.T) {
	all := []domain.Workspace{{ID: "root", RepoID: "r", WorktreePath: "/wt", Branch: "b"}}
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
	}
	g := &fakeGit{deleteErr: errBoom}
	uc := usecases.NewWorktreeUsecase(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow())
	require.ErrorIs(t, uc.DeleteCascade(context.Background(), "root"), errBoom)
}

func TestMergeIntoParent_SetPendingMergeError(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop"}
	g := &fakeGit{mergeErr: enginegit.ErrConflict}
	ws := mergeWS(child, parent, nil)
	ws.SetPendingMergeFn = func(_ context.Context, _ string, _ gitdomain.MergeStrategy, _ string) (domain.Workspace, error) {
		return domain.Workspace{}, errBoom
	}
	uc := usecases.NewWorktreeUsecase(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow())
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
	uc := usecases.NewWorktreeUsecase(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow())
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
	uc := usecases.NewWorktreeUsecase(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow())
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
	uc := usecases.NewWorktreeUsecase(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow())
	_, err := uc.Reparent(context.Background(), "c", "np")
	require.ErrorIs(t, err, errBoom)
}

func TestMergeIntoParent_RevParseError(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat"}
	parent := domain.Workspace{ID: "p", WorktreePath: "/pw", Branch: "develop"}
	g := &fakeGit{revParseErr: errBoom}
	uc := usecases.NewWorktreeUsecase(mergeWS(child, parent, nil), g, &fakeProvider{}, &fakeRepoStore{}, newNow())
	_, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.ErrorIs(t, err, errBoom)
}

func TestReparent_RevParseError(t *testing.T) {
	child := domain.Workspace{ID: "c", Branch: "feat", WorktreePath: "/cw"}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np"}
	g := &fakeGit{revParseErr: errBoom}
	uc := usecases.NewWorktreeUsecase(reparentWS(child, newParent, nil), g, &fakeProvider{}, &fakeRepoStore{}, newNow())
	_, err := uc.Reparent(context.Background(), "c", "np")
	require.ErrorIs(t, err, errBoom)
}
