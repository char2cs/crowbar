package hierarchy

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/holder"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/provision"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// RetryProvision re-provisions a placeholder workspace in place.
func (u *hierarchyUsecase) RetryProvision(
	ctx context.Context,
	wsID string,
) (domain.Workspace, error) {
	ws, err := u.workspaces.Get(ctx, wsID)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("retry provision: get workspace: %w", err)
	}
	repoPath, err := u.repoPathFor(ctx, ws)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("retry provision: repo path: %w", err)
	}
	home, err := u.crowbarHome()
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("retry provision: crowbar home: %w", err)
	}
	outcome, err := holder.Resolve(ctx, u.git, repoPath, ws.Branch, home)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("retry provision: resolve holder: %w", err)
	}
	if outcome.Kind == holder.HeldByHome || outcome.Kind == holder.HeldByExternal {
		return domain.Workspace{}, fmt.Errorf("%w (%s at %s)", ErrBranchStillHeld, ws.Branch, outcome.HeldByPath)
	}
	// git will not check a branch out twice, so a managed holder makes provisioning impossible: refuse with
	// the reason rather than fail deep in materialize.
	if outcome.Kind == holder.HeldByManaged {
		return domain.Workspace{}, fmt.Errorf(
			"%w (%s at %s)", ErrBranchHeldByManagedWorkspace, ws.Branch, outcome.HeldByPath,
		)
	}
	path, err := u.deriveWorktreePath(ctx, home, ws.ProjectID, ws.RepoID, "", ws.Branch)
	if err != nil {
		return domain.Workspace{}, err
	}
	// A branch on origin is checked out AT origin's ref, as an import does; a local-only one uses the
	// local ref rather than strand the placeholder.
	startSha, err := provision.ExistingBranch(ctx, u.git, repoPath, ws.Branch, path)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("retry provision: %w", err)
	}
	provisioned, err := u.workspaces.ProvisionInPlace(ctx, ws.ID, path, startSha)
	if err != nil {
		// The worktree is on disk but the row never landed — clean it up so a
		// later retry isn't blocked by the orphaned worktree.
		if rmErr := u.git.WorktreeRemove(ctx, repoPath, path, true); rmErr != nil {
			slog.WarnContext(ctx, "retry provision: cleanup worktree after failed provision",
				"worktree", path, "err", rmErr)
		}
		return domain.Workspace{}, fmt.Errorf("retry provision: persist: %w", err)
	}
	return provisioned, nil
}

// DetachHolder frees a live holder off a placeholder's branch with consent, then re-provisions in place.
// A detach failure returns before any ClearBranch or Retry, so there is never partial state.
func (u *hierarchyUsecase) DetachHolder(
	ctx context.Context,
	wsID string,
) (domain.Workspace, error) {
	ws, err := u.workspaces.Get(ctx, wsID)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("detach holder: get workspace: %w", err)
	}
	repoPath, err := u.repoPathFor(ctx, ws)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("detach holder: repo path: %w", err)
	}
	home, err := u.crowbarHome()
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("detach holder: crowbar home: %w", err)
	}
	outcome, err := holder.Resolve(ctx, u.git, repoPath, ws.Branch, home)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("detach holder: resolve holder: %w", err)
	}
	if outcome.Kind == holder.HeldByHome || outcome.Kind == holder.HeldByExternal { //nolint:nestif // consent-gated holder detach + home-branch clear; flattening risks the detach/clear ordering
		if dErr := u.git.DetachWorktree(ctx, outcome.HeldByPath); dErr != nil {
			return domain.Workspace{}, fmt.Errorf("detach holder: detach %s: %w", outcome.HeldByPath, dErr)
		}
		if outcome.Kind == holder.HeldByHome {
			homeID, ok, hErr := u.repoHomeWorkspaceID(ctx, ws.RepoID)
			if hErr != nil {
				return domain.Workspace{}, fmt.Errorf("detach holder: find home: %w", hErr)
			}
			if ok {
				if _, cErr := u.workspaces.ClearBranch(ctx, homeID); cErr != nil {
					return domain.Workspace{}, fmt.Errorf("detach holder: clear home branch: %w", cErr)
				}
			}
		}
	}
	return u.RetryProvision(ctx, wsID)
}
