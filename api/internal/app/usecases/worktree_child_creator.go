package usecases

import (
	"context"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// worktreeChildCreator adapts the worktree hierarchy usecase into the agent
// usecase's WorktreeCreator seam (internal/app/usecases/chat.WorktreeCreator):
// Promote names only the fork parent it forks from, and this fills in the rest
// of workspace.CreateChildInput the way every other spontaneous create does —
// leaving RepoID/RepoPath/RemoteURL/ParentBranch blank so CreateChild's own
// parent-inherited defaulting resolves them, and Branch blank so it generates
// and collision-checks a provisional name (model spec §4.1).
//
// OwnWorktree is the one field NOT left to that defaulting: resolveInherited's
// default is "inherit whether the PARENT owns a worktree" (model spec §4.1's
// taxonomy rule for an ordinary create), but promotion's entire point is
// giving the chat a worktree of its own — a fork parent that is itself a
// workspace-less bubble must not silently promote this chat into another
// bubble. So it is forced true here, always.
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

// CreateImportedWorkspace implements agentusecase.WorktreeCreator. Unlike
// CreateChildWorkspace above it fills in NOTHING: an import already knows its
// repo, its branch and its git-lineage parent, because the caller discovered
// the branch in that repository and resolved its PR base before asking. Leaving
// any of it to CreateChild's parent-inherited defaulting would resolve it from
// a parent workspace an import rooted at the repo does not have.
//
// OwnWorktree is forced true for the same reason it is forced above: the
// taxonomy default is "inherit whether the parent owns a worktree", and an
// import whose lineage parent happens to be a bubble must still get a real
// worktree of its own — that is the entire request.
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

// DiscardChildWorkspace implements agentusecase.WorktreeCreator: it removes a
// workspace a promotion minted and then could not finish, through the SAME
// cascade a user-initiated workspace removal takes, so the worktree and the
// branch go together and nothing is left half-reaped. The cascade's own guards
// still apply — by the time this runs the chat has stopped pointing at the
// workspace, so its working-chat guard sees an empty workspace and passes.
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
