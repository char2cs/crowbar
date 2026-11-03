package usecases

import (
	"context"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// worktreeChildCreator adapts the hierarchy usecase to chat.WorktreeCreator: promotion names only
// the fork parent and leaves the rest to CreateChild's inherited defaults. OwnWorktree is forced
// true, since promotion exists to give the chat its own worktree even under a workspace-less parent.
type worktreeChildCreator struct {
	worktree worktreeCascade
}

// worktreeCascade is the slice of the workspace usecase the promotion and the
// chat delete reach through worktreeChildCreator.
type worktreeCascade interface {
	CreateChild(ctx context.Context, in workspace.CreateChildInput) (domain.Workspace, error)
	DeleteCascade(ctx context.Context, rootID string, consent domain.DeleteConsent) error
	DeletionPlan(ctx context.Context, rootID string) ([]string, error)
	WorkAtRisk(ctx context.Context, rootIDs []string) ([]domain.WorkAtRisk, error)
}

// CreateChildWorkspace implements agentusecase.WorktreeCreator.
func (w worktreeChildCreator) CreateChildWorkspace(
	ctx context.Context,
	forkParentID string,
	branch string,
) (domain.Workspace, error) {
	ownWorktree := true
	return w.worktree.CreateChild(ctx, workspace.CreateChildInput{
		ParentID:    forkParentID,
		Branch:      branch,
		OwnWorktree: &ownWorktree,
	})
}

// CreateImportedWorkspace implements agentusecase.WorktreeCreator and fills in nothing: an import
// already knows repo, branch and lineage parent, which inherited defaults would get wrong.
// OwnWorktree is forced true for the same reason as above.
func (w worktreeChildCreator) CreateImportedWorkspace(
	ctx context.Context,
	spec agentusecase.ImportSpec,
) (domain.Workspace, error) {
	ownWorktree := true
	return w.worktree.CreateChild(ctx, workspace.CreateChildInput{
		RepoID:       spec.RepoID,
		ProjectID:    spec.ProjectID,
		RepoPath:     spec.RepoPath,
		RemoteURL:    spec.RemoteURL,
		Branch:       spec.Branch,
		ParentID:     spec.ParentWorkspaceID,
		ParentBranch: spec.ParentBranch,
		ForceLocked:  spec.ForceLocked,
		OwnWorktree:  &ownWorktree,
	})
}

// DiscardChildWorkspace implements agentusecase.WorktreeCreator, removing a workspace a promotion
// could not finish through the same cascade a user removal takes, so worktree and branch go together.
func (w worktreeChildCreator) DiscardChildWorkspace(
	ctx context.Context,
	workspaceID string,
) error {
	return w.worktree.DeleteCascade(ctx, workspaceID, domain.KeepWorkAtRisk)
}

// DeleteWorkspace implements agentusecase.TreeWorkspaceReaper: a chat delete's
// teardown of a worktree it owned, with the consent the user gave that delete.
func (w worktreeChildCreator) DeleteWorkspace(
	ctx context.Context,
	workspaceID string,
	consent domain.DeleteConsent,
) error {
	return w.worktree.DeleteCascade(ctx, workspaceID, consent)
}

// DeletionPlan implements agentusecase.TreeWorkspaceReaper.
func (w worktreeChildCreator) DeletionPlan(ctx context.Context, workspaceID string) ([]string, error) {
	return w.worktree.DeletionPlan(ctx, workspaceID)
}

// WorkAtRisk implements agentusecase.TreeWorkspaceReaper.
func (w worktreeChildCreator) WorkAtRisk(ctx context.Context, workspaceIDs []string) ([]domain.WorkAtRisk, error) {
	return w.worktree.WorkAtRisk(ctx, workspaceIDs)
}
