package usecases

import (
	"context"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type worktreeChildCreator struct {
	worktree worktreeCascade
}

type worktreeCascade interface {
	CreateChild(ctx context.Context, in workspace.CreateChildInput) (domain.Workspace, error)
	CheckNewBranch(ctx context.Context, parentID, branch string) error
	DeleteCascade(ctx context.Context, rootID string, consent domain.DeleteConsent) error
	DeletionPlan(ctx context.Context, rootID string) ([]string, error)
	WorkAtRisk(ctx context.Context, rootIDs []string) ([]domain.WorkAtRisk, error)
}

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

func (w worktreeChildCreator) CreateBranchOutWorkspace(
	ctx context.Context,
	forkParentID string,
	branch string,
) (domain.Workspace, error) {
	ownWorktree := true
	return w.worktree.CreateChild(ctx, workspace.CreateChildInput{
		ParentID:       forkParentID,
		Branch:         branch,
		OwnWorktree:    &ownWorktree,
		FromParentHead: true,
	})
}

func (w worktreeChildCreator) CheckBranchOutName(ctx context.Context, forkParentID, branch string) error {
	return w.worktree.CheckNewBranch(ctx, forkParentID, branch)
}

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

func (w worktreeChildCreator) DiscardChildWorkspace(
	ctx context.Context,
	workspaceID string,
) error {
	return w.worktree.DeleteCascade(ctx, workspaceID, domain.KeepWorkAtRisk)
}

func (w worktreeChildCreator) DeleteWorkspace(
	ctx context.Context,
	workspaceID string,
	consent domain.DeleteConsent,
) error {
	return w.worktree.DeleteCascade(ctx, workspaceID, consent)
}

func (w worktreeChildCreator) DeletionPlan(ctx context.Context, workspaceID string) ([]string, error) {
	return w.worktree.DeletionPlan(ctx, workspaceID)
}

func (w worktreeChildCreator) WorkAtRisk(ctx context.Context, workspaceIDs []string) ([]domain.WorkAtRisk, error) {
	return w.worktree.WorkAtRisk(ctx, workspaceIDs)
}
