package mocks

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// WorkspaceLifecycleRepo is a fake of the workspace repo surface used by the
// workspace, file, and git usecases.
type WorkspaceLifecycleRepo struct {
	ListFn       func(ctx context.Context) ([]domain.Workspace, error)
	ListInRepoFn func(ctx context.Context, projectID, repoID string) ([]domain.Workspace, error)
	GetFn        func(ctx context.Context, id string) (domain.Workspace, error)

	SetLockFn func(
		ctx context.Context,
		id string,
		locked *bool,
		protected bool,
	) (domain.Workspace, error)

	SetMergeStrategyFn func(
		ctx context.Context,
		id string,
		strategy gitdomain.MergeStrategy,
	) (domain.Workspace, error)
	SyncWorkingTreeFn func(
		ctx context.Context,
		in workspace.SyncInput,
		now time.Time,
	) (domain.Workspace, error)
	ResolveConflictsFn func(
		ctx context.Context,
		id string,
		now time.Time,
	) (domain.Workspace, error)
}

// NewWorkspaceLifecycleRepo returns an empty WorkspaceLifecycleRepo.
func NewWorkspaceLifecycleRepo() *WorkspaceLifecycleRepo {
	return &WorkspaceLifecycleRepo{}
}

func (r *WorkspaceLifecycleRepo) List(
	ctx context.Context,
) ([]domain.Workspace, error) {
	return r.ListFn(ctx)
}

func (r *WorkspaceLifecycleRepo) ListInRepo(
	ctx context.Context,
	projectID string,
	repoID string,
) ([]domain.Workspace, error) {
	return r.ListInRepoFn(ctx, projectID, repoID)
}

func (r *WorkspaceLifecycleRepo) Get(
	ctx context.Context,
	id string,
) (domain.Workspace, error) {
	return r.GetFn(ctx, id)
}

func (r *WorkspaceLifecycleRepo) SetMergeStrategy(
	ctx context.Context,
	id string,
	strategy gitdomain.MergeStrategy,
) (domain.Workspace, error) {
	return r.SetMergeStrategyFn(ctx, id, strategy)
}

// SetLock records the lock decision the usecase passed down, so a test can read
// back what it decided to persist. It answers with the row it was handed rather
// than a stored one — the usecase resolves the status itself via the command.
func (r *WorkspaceLifecycleRepo) SetLock(
	ctx context.Context,
	id string,
	locked *bool,
	protected bool,
) (domain.Workspace, error) {
	if r.SetLockFn != nil {
		return r.SetLockFn(ctx, id, locked, protected)
	}
	return domain.Workspace{ID: id, LockOverride: locked}, nil
}

func (r *WorkspaceLifecycleRepo) SyncWorkingTreeState(
	ctx context.Context,
	in workspace.SyncInput,
	now time.Time,
) (domain.Workspace, error) {
	return r.SyncWorkingTreeFn(ctx, in, now)
}

func (r *WorkspaceLifecycleRepo) ResolveConflicts(
	ctx context.Context,
	id string,
	now time.Time,
) (domain.Workspace, error) {
	if r.ResolveConflictsFn != nil {
		return r.ResolveConflictsFn(ctx, id, now)
	}
	return domain.Workspace{ID: id}, nil
}
