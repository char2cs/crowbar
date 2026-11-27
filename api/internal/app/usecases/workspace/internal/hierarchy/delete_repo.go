package hierarchy

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/cascade"
	"github.com/char2cs/crowbar/api/internal/domain"
)

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
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("delete repo workspaces: %w", err)
	}
	return nil
}

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

func repoWorkspaces(
	repoID string,
	all []domain.Workspace,
) []domain.Workspace {
	index := indexByID(all)
	mine := make([]cascade.Node, 0, len(all))
	for _, ws := range all {
		if ws.RepoID == repoID && ws.Status != domain.WorkspaceStatusDeleted {
			mine = append(mine, cascade.Node{ID: ws.ID, Parent: ws.ParentID})
		}
	}
	var ordered []domain.Workspace
	for _, n := range mine {
		if parent, ok := index[n.Parent]; ok && parent.RepoID == repoID &&
			parent.Status != domain.WorkspaceStatusDeleted {
			continue
		}
		ordered = append(ordered, liveRows(cascade.Plan(n.ID, mine), index)...)
	}
	return ordered
}
