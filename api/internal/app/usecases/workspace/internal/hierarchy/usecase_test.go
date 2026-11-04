package hierarchy_test

import (
	"context"
	"errors"
	"time"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
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
	return domain.Workspace{ID: id, WorktreePath: worktreePath, ForkPointSha: forkPointSha, Provisioning: domain.WorkspaceProvisioned}, nil
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

// ObserveWorkingTreeState is the watcher's; the hierarchy's syncs are awaited.
func (f *fakeWorkspace) ObserveWorkingTreeState(context.Context, workspace.SyncInput, time.Time) (domain.Workspace, error) {
	panic("hierarchy: a working-tree sync must be awaited")
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

func (f *fakeWorkspace) SyncProviderState(context.Context, workspace.ProviderInput, time.Time) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

func (f *fakeWorkspace) SetMergeStrategy(context.Context, string, gitdomain.MergeStrategy) (domain.Workspace, error) {
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

func (f *fakeWorkspace) SetProject(_ context.Context, id, projectID string) (domain.Workspace, error) {
	return domain.Workspace{ID: id, ProjectID: projectID}, nil
}

func (f *fakeWorkspace) GetHomeForProject(_ context.Context, _ string) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

func (f *fakeWorkspace) ListInRepo(_ context.Context, _, _ string) ([]domain.Workspace, error) {
	return nil, nil
}

var _ workspace.Workspace = (*fakeWorkspace)(nil)

type fakeProvider struct {
	engineprovider.Engine
	protected []string
	err       error
	prLinks   []engineprovider.PRLink
	prErr     error
}

func (f *fakeProvider) ProtectedBranches(
	_ context.Context,
	_ string,
) ([]string, error) {
	return f.protected, f.err
}

func (f *fakeProvider) OpenPullRequests(
	_ context.Context,
	_ string,
) ([]engineprovider.PRLink, error) {
	return f.prLinks, f.prErr
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

func fakeHome() func() (string, error) {
	return func() (string, error) { return "/tmp/crowbar-test", nil }
}

// fakeChatObserver is a hierarchy.ChatWorkObserver double: chats lists every
// chat row across every workspace, and working reports Working per chat id
// (absent means idle).
type fakeChatObserver struct {
	chats   []domain.Chat
	working map[string]bool
}

func (f *fakeChatObserver) ListChatsByWorkspace(
	_ context.Context,
	workspaceID string,
) ([]domain.Chat, error) {
	var out []domain.Chat
	for _, c := range f.chats {
		if c.WorkspaceID == workspaceID {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *fakeChatObserver) ChatBusy(_ context.Context, chatID string) (bool, error) {
	return f.working[chatID], nil
}

func (f *fakeWorkspace) SetLock(
	_ context.Context,
	id string,
	locked *bool,
	_ bool,
) (domain.Workspace, error) {
	return domain.Workspace{ID: id, LockOverride: locked}, nil
}
