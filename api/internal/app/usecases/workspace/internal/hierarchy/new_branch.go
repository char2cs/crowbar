package hierarchy

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// CheckNewBranch refuses a name a new branch cut under parentID could not take: one git
// rejects, one the repo already has (locally or on origin), or one a workspace holds.
// "" passes, since the create then generates a free name.
func (u *hierarchyUsecase) CheckNewBranch(ctx context.Context, parentID, branch string) error {
	if branch == "" {
		return nil
	}
	parent, err := u.workspaces.Get(ctx, parentID)
	if err != nil {
		return fmt.Errorf("check new branch: get parent: %w", err)
	}
	repoPath, err := u.repoPathFor(ctx, parent)
	if err != nil {
		return fmt.Errorf("check new branch: %w", err)
	}
	if err := u.git.CheckBranchName(ctx, repoPath, branch); err != nil {
		return err
	}
	held, err := u.branchWorkspaceExists(ctx, parent.RepoID, branch)
	if err != nil {
		return fmt.Errorf("check new branch: %w", err)
	}
	local, err := u.git.LocalBranchExists(ctx, repoPath, branch)
	if err != nil {
		return fmt.Errorf("check new branch: %w", err)
	}
	onOrigin, err := u.git.RemoteTrackingBranchExists(ctx, repoPath, branch)
	if err != nil {
		return fmt.Errorf("check new branch: %w", err)
	}
	if held || local || onOrigin {
		return &problem.Error{
			Code:    problem.Conflict,
			Message: fmt.Sprintf("%q already exists", branch),
			Err:     problem.ErrConflict,
		}
	}
	return nil
}
