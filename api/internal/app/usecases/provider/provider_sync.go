package provider

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineprovider "github.com/char2cs/crowbar/api/internal/engine/provider"
)

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
	List(ctx context.Context) ([]domain.Workspace, error)
	SetParentFromPR(ctx context.Context, id, parentID string) (domain.Workspace, error)
}

type Engine interface {
	PollOnView(
		ctx context.Context,
		wsID string,
		repoPath string,
		branch string,
	) (engineprovider.ProviderState, error)
}

type Usecase interface {
	PollWorkspace(
		ctx context.Context,
		wsID string,
	) error

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

func New(
	workspaces WorkspaceRepo,
	engine Engine,
) Usecase {
	return &providerSyncUsecase{
		workspaces: workspaces,
		engine:     engine,
	}
}

func (u *providerSyncUsecase) PollWorkspace(
	ctx context.Context,
	wsID string,
) error {
	ws, err := u.workspaces.Get(ctx, wsID)
	if err != nil {
		return fmt.Errorf("provider sync: poll workspace: get: %w", err)
	}
	if ws.Kind == domain.WorkspaceKindHome {
		return nil
	}
	state, err := u.engine.PollOnView(ctx, wsID, ws.WorktreePath, ws.Branch)
	if err != nil {
		return fmt.Errorf("provider sync: poll workspace: poll: %w", err)
	}
	return u.SyncFromState(ctx, wsID, state, time.Now())
}

func (u *providerSyncUsecase) SyncFromState(
	ctx context.Context,
	wsID string,
	state engineprovider.ProviderState,
	now time.Time,
) error {
	current, err := u.workspaces.Get(ctx, wsID)
	if err != nil {
		return fmt.Errorf("provider sync: sync from state: get workspace: %w", err)
	}

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

	if current.Status == domain.WorkspaceStatusLocked {
		return nil
	}
	if state.PR != nil &&
		state.PR.Status == "open" &&
		state.PR.TargetBranch != "" &&
		current.ParentID == "" {
		u.maybeReparentFromPR(ctx, current, state.PR.TargetBranch)
	}
	return nil
}

func (u *providerSyncUsecase) maybeReparentFromPR(
	ctx context.Context,
	ws domain.Workspace,
	targetBranch string,
) {
	all, err := u.workspaces.List(ctx)
	if err != nil {
		return
	}
	byID := make(map[string]domain.Workspace, len(all))
	for _, w := range all {
		byID[w.ID] = w
	}
	for _, candidate := range all {
		if candidate.RepoID != ws.RepoID || candidate.Branch != targetBranch {
			continue
		}
		if candidate.ID == ws.ID {
			return
		}
		if isAncestor(byID, ws.ID, candidate.ID) {
			slog.WarnContext(ctx, "provider: skip auto-reparent that would create a cycle",
				"workspace_id", ws.ID, "candidate_id", candidate.ID, "target_branch", targetBranch)
			return
		}
		if _, err := u.workspaces.SetParentFromPR(ctx, ws.ID, candidate.ID); err != nil {
			slog.WarnContext(ctx, "provider: auto-reparent from PR failed",
				"workspace_id", ws.ID, "parent_id", candidate.ID, "err", err)
		}
		return
	}
}

func isAncestor(byID map[string]domain.Workspace, ancestorID, nodeID string) bool {
	visited := make(map[string]bool)
	cur := nodeID
	for cur != "" && !visited[cur] {
		visited[cur] = true
		w, ok := byID[cur]
		if !ok {
			return false
		}
		if w.ParentID == ancestorID {
			return true
		}
		cur = w.ParentID
	}
	return false
}
