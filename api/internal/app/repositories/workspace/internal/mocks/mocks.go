package mocks

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// MockWorkspace is a test double for workspace.Workspace. Each method delegates
// to the corresponding function-field, allowing callers to inject behaviour
// per-test without subclassing or code generation.
type MockWorkspace struct {
	CreateFn              func(ctx context.Context, in workspace.CreateInput, now time.Time) (domain.Workspace, error)
	SyncWorkingFn         func(ctx context.Context, in workspace.SyncInput, now time.Time) (domain.Workspace, error)
	SyncProviderFn        func(ctx context.Context, in workspace.ProviderInput, now time.Time) (domain.Workspace, error)
	SetMergeStrategyFn    func(ctx context.Context, id string, s gitdomain.MergeStrategy) (domain.Workspace, error)
	TouchActivityFn       func(ctx context.Context, id string, now time.Time) (domain.Workspace, error)
	ReparentFn            func(ctx context.Context, id, parentID, forkPointSha string, now time.Time) (domain.Workspace, error)
	ResolveConflictsFn    func(ctx context.Context, id string, now time.Time) (domain.Workspace, error)
	UpdateForkPointFn     func(ctx context.Context, id, forkPointSha string) (domain.Workspace, error)
	SetParentFromPRFn     func(ctx context.Context, id, parentID string) (domain.Workspace, error)
	SetLastErrorFn        func(ctx context.Context, id, message string) (domain.Workspace, error)
	DeleteFn              func(ctx context.Context, id string) error
	GetFn                 func(ctx context.Context, id string) (domain.Workspace, error)
	ListFn                func(ctx context.Context) ([]domain.Workspace, error)
	GetHomeForProjectFn   func(ctx context.Context, projectID string) (domain.Workspace, error)
	CreateHomeFn          func(ctx context.Context, projectID, worktreePath string, now time.Time) (domain.Workspace, error)
}

func (m *MockWorkspace) Create(
	ctx context.Context,
	in workspace.CreateInput,
	now time.Time,
) (domain.Workspace, error) {
	return m.CreateFn(ctx, in, now)
}

func (m *MockWorkspace) SyncWorkingTreeState(
	ctx context.Context,
	in workspace.SyncInput,
	now time.Time,
) (domain.Workspace, error) {
	return m.SyncWorkingFn(ctx, in, now)
}

func (m *MockWorkspace) SyncProviderState(
	ctx context.Context,
	in workspace.ProviderInput,
	now time.Time,
) (domain.Workspace, error) {
	return m.SyncProviderFn(ctx, in, now)
}

func (m *MockWorkspace) SetMergeStrategy(
	ctx context.Context,
	id string,
	s gitdomain.MergeStrategy,
) (domain.Workspace, error) {
	return m.SetMergeStrategyFn(ctx, id, s)
}

func (m *MockWorkspace) TouchActivity(
	ctx context.Context,
	id string,
	now time.Time,
) (domain.Workspace, error) {
	return m.TouchActivityFn(ctx, id, now)
}

func (m *MockWorkspace) Reparent(
	ctx context.Context,
	id string,
	parentID string,
	forkPointSha string,
	now time.Time,
) (domain.Workspace, error) {
	return m.ReparentFn(ctx, id, parentID, forkPointSha, now)
}

func (m *MockWorkspace) ResolveConflicts(
	ctx context.Context,
	id string,
	now time.Time,
) (domain.Workspace, error) {
	if m.ResolveConflictsFn != nil {
		return m.ResolveConflictsFn(ctx, id, now)
	}
	return domain.Workspace{ID: id}, nil
}

func (m *MockWorkspace) UpdateForkPoint(
	ctx context.Context,
	id string,
	forkPointSha string,
) (domain.Workspace, error) {
	return m.UpdateForkPointFn(ctx, id, forkPointSha)
}

func (m *MockWorkspace) ProvisionInPlace(
	_ context.Context,
	id string,
	worktreePath string,
	forkPointSha string,
) (domain.Workspace, error) {
	return domain.Workspace{ID: id, WorktreePath: worktreePath, ForkPointSha: forkPointSha}, nil
}

func (m *MockWorkspace) ClearBranch(
	_ context.Context,
	id string,
) (domain.Workspace, error) {
	return domain.Workspace{ID: id}, nil
}

func (m *MockWorkspace) SetParentFromPR(
	ctx context.Context,
	id string,
	parentID string,
) (domain.Workspace, error) {
	return m.SetParentFromPRFn(ctx, id, parentID)
}

func (m *MockWorkspace) SetLastError(
	ctx context.Context,
	id string,
	message string,
) (domain.Workspace, error) {
	return m.SetLastErrorFn(ctx, id, message)
}

func (m *MockWorkspace) Delete(
	ctx context.Context,
	id string,
) error {
	return m.DeleteFn(ctx, id)
}

func (m *MockWorkspace) Get(
	ctx context.Context,
	id string,
) (domain.Workspace, error) {
	return m.GetFn(ctx, id)
}

func (m *MockWorkspace) List(
	ctx context.Context,
) ([]domain.Workspace, error) {
	return m.ListFn(ctx)
}

func (m *MockWorkspace) GetHomeForProject(
	ctx context.Context,
	projectID string,
) (domain.Workspace, error) {
	return m.GetHomeForProjectFn(ctx, projectID)
}

func (m *MockWorkspace) CreateHome(
	ctx context.Context,
	projectID string,
	worktreePath string,
	now time.Time,
) (domain.Workspace, error) {
	if m.CreateHomeFn != nil {
		return m.CreateHomeFn(ctx, projectID, worktreePath, now)
	}
	return domain.Workspace{}, nil
}

var _ workspace.Workspace = (*MockWorkspace)(nil)
