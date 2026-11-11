package hierarchy

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// adoptMainWorktree registers the repository's main worktree as a workspace
// without creating a new git worktree. It is used when the requested branch is
// the repository's default branch and there is no parent workspace.
func (u *hierarchyUsecase) adoptMainWorktree(
	ctx context.Context,
	in CreateChildInput,
) (domain.Workspace, error) {
	startSha, err := u.git.RevParse(ctx, in.RepoPath, "HEAD")
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("create child: adopt main worktree: rev-parse HEAD: %w", err)
	}
	locked, err := u.resolveLocked(ctx, in.RepoPath, in.Branch)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("create child: adopt main worktree: locked: %w", err)
	}
	ws, err := u.workspaces.Create(ctx, workspace.CreateInput{
		// Deterministic: a concurrent second adopt of the same folder is refused
		// by the aggregate itself rather than minting a second default.
		ID:           workspace.RepoHomeID(in.RepoID),
		RepoID:       in.RepoID,
		ProjectID:    in.ProjectID,
		Branch:       in.Branch,
		WorktreePath: in.RepoPath,
		ForkPointSha: startSha,
		ParentID:     in.ParentID,
		Protected:    locked || in.ForceLocked,
		// The adopted main worktree IS the repo's default workspace. Marking it
		// keeps IsDefault reliable for the one-managed-workspace-per-branch guard,
		// which must never count the default.
		IsDefault:    true,
		Provisioning: domain.WorkspaceShared,
	}, u.now())
	if err != nil {
		return domain.Workspace{}, err
	}
	if nErr := u.mintWorkspaceNode(ctx, ws.ID); nErr != nil {
		u.discardWorkspaceRow(ctx, ws.ID, "adopt main worktree")
		return domain.Workspace{}, nErr
	}
	return ws, nil
}

// repoHomeWorkspaceID returns the id of the repo's default (home) workspace; its absence is what lets
// adoptMainWorktree run at most once per repo.
func (u *hierarchyUsecase) repoHomeWorkspaceID(
	ctx context.Context,
	repoID string,
) (string, bool, error) {
	all, err := u.workspaces.List(ctx)
	if err != nil {
		return "", false, err
	}
	for _, w := range all {
		if w.RepoID == repoID && w.IsDefault && w.Status != domain.WorkspaceStatusDeleted {
			return w.ID, true, nil
		}
	}
	return "", false, nil
}
