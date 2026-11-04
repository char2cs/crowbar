package mocks

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// ProviderSyncWorkspaceRepo is a fake of the workspace.Workspace surface used
// by ProviderSyncUsecase.
type ProviderSyncWorkspaceRepo struct {
	GetFn             func(ctx context.Context, id string) (domain.Workspace, error)
	SyncProviderFn    func(ctx context.Context, in workspace.ProviderInput, now time.Time) (domain.Workspace, error)
	ListFn            func(ctx context.Context) ([]domain.Workspace, error)
	SetParentFromPRFn func(ctx context.Context, id, parentID string) (domain.Workspace, error)
}

// NewProviderSyncWorkspaceRepo returns an empty ProviderSyncWorkspaceRepo.
func NewProviderSyncWorkspaceRepo() *ProviderSyncWorkspaceRepo {
	return &ProviderSyncWorkspaceRepo{}
}

func (r *ProviderSyncWorkspaceRepo) Get(
	ctx context.Context,
	id string,
) (domain.Workspace, error) {
	return r.GetFn(ctx, id)
}

func (r *ProviderSyncWorkspaceRepo) SyncProviderState(
	ctx context.Context,
	in workspace.ProviderInput,
	now time.Time,
) (domain.Workspace, error) {
	return r.SyncProviderFn(ctx, in, now)
}

func (r *ProviderSyncWorkspaceRepo) List(ctx context.Context) ([]domain.Workspace, error) {
	if r.ListFn != nil {
		return r.ListFn(ctx)
	}
	return nil, nil
}

func (r *ProviderSyncWorkspaceRepo) SetParentFromPR(ctx context.Context, id, parentID string) (domain.Workspace, error) {
	if r.SetParentFromPRFn != nil {
		return r.SetParentFromPRFn(ctx, id, parentID)
	}
	return domain.Workspace{}, nil
}
