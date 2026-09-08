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

	// A protected (locked) branch is a branch-tree root and is never reparented,
	// no matter what a PR says — the same invariant the user-facing reparent path
	// enforces (locked rows are undraggable; guardReparent rejects a locked
	// parent). It is guarded independently so it holds even if the parent-state
	// precondition below is later relaxed (e.g. to follow a retargeted PR).
	if current.Status == domain.WorkspaceStatusLocked {
		return nil
	}
	// Auto-reparent wires the workspace under the sibling its PR targets, but only
	// for an OPEN PR (a closed/merged PR is stale history and must not reshape the
	// tree) that targets a branch, and only while the workspace has no parent yet.
	// Without the open-PR guard a long-closed master→develop PR would nest the
	// branch under develop on the first poll after import.
	if state.PR != nil &&
		state.PR.Status == "open" &&
		state.PR.TargetBranch != "" &&
		current.ParentID == "" {
		u.maybeReparentFromPR(ctx, current, state.PR.TargetBranch)
	}
	return nil
}

// maybeReparentFromPR looks up the workspace in the same repo whose branch
// matches targetBranch and parents ws under it.
//
// Two corruptions must never happen, even though SyncFromState already guards
// ParentID=="" upstream:
//   - Self-parent: a PR that targets its own branch makes candidate==ws. Pointing
//     a node at itself detaches it from the tree and makes it unreparentable (the
//     same self-loop the user-facing SetParent guard rejects). Skip it.
//   - Cycle: if ws is already an ancestor of the candidate (the candidate's
//     ParentID chain reaches ws), parenting ws under the candidate closes a loop,
//     and any later tree walk (ancestor resolution, merge-base, prune) spins
//     forever. Walk the candidate's chain with a visited set and skip if ws appears.
//
// The reparent error is logged rather than discarded so a failed write is visible
// in production instead of silently leaving the tree unparented.
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
