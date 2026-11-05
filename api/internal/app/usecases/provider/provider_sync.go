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
	List(ctx context.Context) ([]domain.Workspace, error)
	SetParentFromPR(ctx context.Context, id, parentID string) (domain.Workspace, error)
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
// the background-sweep callback.
type Usecase interface {
	// PollWorkspace loads the workspace, calls PollOnView, and applies the result.
	PollWorkspace(
		ctx context.Context,
		wsID string,
	) error

	// SyncFromState maps a provider.ProviderState to a workspace.ProviderInput and
	// issues SyncProviderState; the background sweep invokes it.
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
// A project home has no remote to ask, so it is never polled.
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

// SyncFromState maps a provider.ProviderState to a workspace.ProviderInput and
// issues SyncProviderState; the background sweep invokes it.
func (u *providerSyncUsecase) SyncFromState(
	ctx context.Context,
	wsID string,
	state engineprovider.ProviderState,
	now time.Time,
) error {
	// Load the current workspace to read ParentID and RepoID.
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

	// A locked branch is a tree root and never reparented whatever a PR says, guarded on its own
	// so it survives any relaxing of the parent-state precondition below.
	if current.Status == domain.WorkspaceStatusLocked {
		return nil
	}
	// Auto-reparent only for an OPEN PR targeting a branch while the workspace has no parent: a
	// long-closed PR must not reshape the tree on the first poll after import.
	if state.PR != nil &&
		state.PR.Status == "open" &&
		state.PR.TargetBranch != "" &&
		current.ParentID == "" {
		u.maybeReparentFromPR(ctx, current, state.PR.TargetBranch)
	}
	return nil
}

// maybeReparentFromPR parents ws under the same-repo workspace on targetBranch, skipping a
// self-parent and any candidate whose ancestor chain reaches ws (a cycle spins every tree walk).
// A failed reparent is logged so it is visible rather than silently leaving the tree unparented.
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
			// PR targets its own branch — never self-parent.
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

// isAncestor reports whether ancestorID appears in nodeID's ParentID chain.
// The visited set bounds the walk so a pre-existing cycle in the data can't hang
// it (defensive: the reparent guards exist precisely to keep the tree acyclic).
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
