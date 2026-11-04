package hierarchy

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/cascade"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// DeleteRepoWorkspaces takes every row of the repo, locked ones included, but never forces: removeOne
// keeps branches Crowbar did not create and uncommitted work in locked worktrees. Unconsented unique
// work stops the walk first; a failed tombstone is returned so the repo row stays for a re-drive.
func (u *hierarchyUsecase) DeleteRepoWorkspaces(
	ctx context.Context,
	repo domain.Repository,
	consent domain.DeleteConsent,
) error {
	all, err := u.workspaces.List(ctx)
	if err != nil {
		return fmt.Errorf("delete repo workspaces: list: %w", err)
	}
	ref := repoRef{path: repo.Path, defaultBranch: repo.DefaultBranch}
	doomed := repoWorkspaces(repo.ID, all)
	if lossErr := u.refuseLoss(ctx, consent, doomed, ref, all); lossErr != nil {
		return fmt.Errorf("delete repo workspaces: %w", lossErr)
	}
	var errs []error
	for _, ws := range doomed {
		if removeErr := u.removeOne(ctx, ws, ref, all); removeErr != nil {
			errs = append(errs, fmt.Errorf("remove %s: %w", ws.ID, removeErr))
		}
	}
	// Reported only after every other workspace had its turn: the repo must not
	// go while a workspace of it is still live.
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("delete repo workspaces: %w", err)
	}
	return nil
}

// RepoWorkAtRisk reports what DeleteRepoWorkspaces would destroy without
// consent, so a repo delete can refuse before it records its intent.
func (u *hierarchyUsecase) RepoWorkAtRisk(
	ctx context.Context,
	repo domain.Repository,
) ([]domain.WorkAtRisk, error) {
	all, err := u.workspaces.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("repo work at risk: list: %w", err)
	}
	ref := repoRef{path: repo.Path, defaultBranch: repo.DefaultBranch}
	return u.assess(ctx, repoWorkspaces(repo.ID, all), ref, all)
}

// repoWorkspaces lists every live workspace of a repo, locked ones included,
// each subtree root followed by its descendants in cascade order.
func repoWorkspaces(
	repoID string,
	all []domain.Workspace,
) []domain.Workspace {
	index := indexByID(all)
	mine := make([]cascade.Node, 0, len(all))
	for _, ws := range all {
		if ws.RepoID == repoID && ws.Status != domain.WorkspaceStatusDeleted {
			// Locked is left false: the whole repo goes, so no lock stops the walk.
			mine = append(mine, cascade.Node{ID: ws.ID, Parent: ws.ParentID})
		}
	}
	var ordered []domain.Workspace
	for _, n := range mine {
		if parent, ok := index[n.Parent]; ok && parent.RepoID == repoID &&
			parent.Status != domain.WorkspaceStatusDeleted {
			continue // taken by its parent's cascade
		}
		ordered = append(ordered, liveRows(cascade.Plan(n.ID, mine), index)...)
	}
	return ordered
}
