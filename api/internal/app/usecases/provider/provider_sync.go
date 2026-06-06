package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineprovider "github.com/char2cs/crowbar/api/internal/engine/provider"
)

// WorkspaceRepo is the workspace surface Usecase needs.
type WorkspaceRepo interface {
	Get(
		ctx context.Context,
		id string,
	) (domain.Workspace, error)
	SyncProviderState(
		ctx context.Context,
		in workspace.ProviderInput,
		now time.Time,
	) (domain.Workspace, error)
}

// Engine is the provider-engine surface Usecase needs.
type Engine interface {
	PollOnView(
		ctx context.Context,
		wsID string,
		repoPath string,
		branch string,
	) (engineprovider.ProviderState, error)
}

// Usecase polls the provider for a workspace and applies the result
// via SyncProviderState. PollWorkspace is the on-view trigger; SyncFromState is
// the background-sweep callback (08 §5).
type Usecase interface {
	// PollWorkspace loads the workspace, calls PollOnView, and applies the result.
	PollWorkspace(
		ctx context.Context,
		wsID string,
	) error

	// SyncFromState maps a provider.ProviderState to a workspace.ProviderInput and
	// issues SyncProviderState. This is the callback the background sweep invokes
	// (wired in Task 29).
	SyncFromState(
		ctx context.Context,
		wsID string,
		state engineprovider.ProviderState,
		now time.Time,
	) error
}

type providerSyncUsecase struct {
	workspaces WorkspaceRepo
	engine     Engine
}

// New builds a Usecase from the workspace repo and provider engine.
func New(
	workspaces WorkspaceRepo,
	engine Engine,
) Usecase {
	return &providerSyncUsecase{
		workspaces: workspaces,
		engine:     engine,
	}
}

// PollWorkspace loads the workspace, calls PollOnView, and applies the result.
func (u *providerSyncUsecase) PollWorkspace(
	ctx context.Context,
	wsID string,
) error {
	ws, err := u.workspaces.Get(ctx, wsID)
	if err != nil {
		return fmt.Errorf("provider sync: poll workspace: get: %w", err)
	}
	state, err := u.engine.PollOnView(ctx, wsID, ws.WorktreePath, ws.Branch)
	if err != nil {
		return fmt.Errorf("provider sync: poll workspace: poll: %w", err)
	}
	return u.SyncFromState(ctx, wsID, state, time.Now())
}

// SyncFromState maps a provider.ProviderState to a workspace.ProviderInput and
// issues SyncProviderState. This is the callback the background sweep invokes
// (wired in Task 29).
func (u *providerSyncUsecase) SyncFromState(
	ctx context.Context,
	wsID string,
	state engineprovider.ProviderState,
	now time.Time,
) error {
	in := workspace.ProviderInput{
		ID:        wsID,
		Protected: state.Protected,
	}
	if state.PR != nil {
		in.HasPR = true
		in.PRStatus = state.PR.Status
		in.PRUrl = state.PR.URL
		in.PRTitle = state.PR.Title
		in.PRTargetBranch = state.PR.TargetBranch
	}
	if _, err := u.workspaces.SyncProviderState(ctx, in, now); err != nil {
		return fmt.Errorf("provider sync: sync from state: %w", err)
	}
	return nil
}
