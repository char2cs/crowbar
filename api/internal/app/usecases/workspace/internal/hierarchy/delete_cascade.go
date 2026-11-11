package hierarchy

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/cascade"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// DeleteCascade erases rootID and its unlocked descendants, refusing what the
// Chats panel's delete would refuse; a locked descendant it keeps is re-hung so
// no lineage names a workspace that is gone.
func (u *hierarchyUsecase) DeleteCascade(
	ctx context.Context,
	rootID string,
	consent domain.DeleteConsent,
) error {
	all, err := u.workspaces.List(ctx)
	if err != nil {
		return fmt.Errorf("delete cascade: list: %w", err)
	}
	root, doomed, err := u.planCascade(ctx, rootID, all)
	if err != nil {
		return err
	}
	repo := u.repoRefFor(ctx, root.RepoID)
	if lossErr := u.refuseLoss(ctx, consent, doomed, repo, all); lossErr != nil {
		return fmt.Errorf("delete cascade: %w", lossErr)
	}
	for _, ws := range doomed {
		if removeErr := u.removeOne(ctx, ws, repo, all); removeErr != nil {
			return fmt.Errorf("delete cascade: remove %s: %w", ws.ID, removeErr)
		}
	}
	return u.rehangSurvivors(ctx, doomed, all)
}

// DeletionPlan answers what DeleteCascade(rootID) would remove, or the refusal
// it would answer, without touching anything — so a caller deleting several
// aggregates can refuse before its first effect.
func (u *hierarchyUsecase) DeletionPlan(
	ctx context.Context,
	rootID string,
) ([]string, error) {
	all, err := u.workspaces.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("deletion plan: list: %w", err)
	}
	_, doomed, err := u.planCascade(ctx, rootID, all)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(doomed))
	for _, ws := range doomed {
		ids = append(ids, ws.ID)
	}
	return ids, nil
}

// planCascade applies DeleteCascade's guards and returns its root and the
// live rows it takes, deepest first.
func (u *hierarchyUsecase) planCascade(
	ctx context.Context,
	rootID string,
	all []domain.Workspace,
) (domain.Workspace, []domain.Workspace, error) {
	index := indexByID(all)
	root, ok := index[rootID]
	// A tombstone awaiting its reactor is already gone: a second delete
	// event would run the teardown twice.
	if !ok || root.Status == domain.WorkspaceStatusDeleted {
		return root, nil, fmt.Errorf("delete cascade: workspace %s: %w", rootID, problem.ErrNotFound)
	}
	switch {
	case root.Status == domain.WorkspaceStatusLocked:
		return root, nil, ErrWorkspaceLocked
	case root.IsDefault:
		return root, nil, ErrWorkspaceIsDefault
	case root.Kind == domain.WorkspaceKindHome:
		return root, nil, ErrWorkspaceIsHome
	}
	if workingErr := u.guardNotWorking(ctx, rootID); workingErr != nil {
		return root, nil, workingErr
	}
	return root, liveRows(cascade.Plan(rootID, nodesFrom(all)), index), nil
}

// rehangSurvivors moves each kept workspace whose fork parent the cascade took
// under that parent's nearest surviving ancestor, so no lineage names a
// workspace that is gone.
func (u *hierarchyUsecase) rehangSurvivors(
	ctx context.Context,
	doomed []domain.Workspace,
	all []domain.Workspace,
) error {
	gone := make(map[string]string, len(doomed))
	for _, ws := range doomed {
		gone[ws.ID] = ws.ParentID
	}
	for _, ws := range all {
		_, taken := gone[ws.ID]
		if _, orphaned := gone[ws.ParentID]; taken || !orphaned || ws.Status == domain.WorkspaceStatusDeleted {
			continue
		}
		parent := ws.ParentID
		for next, ok := gone[parent]; ok; next, ok = gone[parent] {
			parent = next
		}
		if _, err := u.workspaces.Reparent(ctx, ws.ID, parent, ws.ForkPointSha, u.now()); err != nil {
			return fmt.Errorf("delete cascade: rehang %s: %w", ws.ID, err)
		}
	}
	return nil
}

// WorkAtRisk reports what deleting each of rootIDs (with its cascade) would
// destroy that exists nowhere else — the list a delete without consent refuses
// over. Unknown or already-deleted roots contribute nothing.
func (u *hierarchyUsecase) WorkAtRisk(
	ctx context.Context,
	rootIDs []string,
) ([]domain.WorkAtRisk, error) {
	all, err := u.workspaces.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("work at risk: list: %w", err)
	}
	index := indexByID(all)
	seen := map[string]bool{}
	var risks []domain.WorkAtRisk
	for _, rootID := range rootIDs {
		root, ok := index[rootID]
		if !ok || seen[rootID] {
			continue
		}
		var doomed []domain.Workspace
		for _, ws := range liveRows(cascade.Plan(rootID, nodesFrom(all)), index) {
			if !seen[ws.ID] {
				seen[ws.ID] = true
				doomed = append(doomed, ws)
			}
		}
		found, assessErr := u.assess(ctx, doomed, u.repoRefFor(ctx, root.RepoID), all)
		if assessErr != nil {
			return nil, assessErr
		}
		risks = append(risks, found...)
	}
	return risks, nil
}

// liveRows resolves ids to their rows in order, skipping tombstones.
func liveRows(
	ids []string,
	index map[string]domain.Workspace,
) []domain.Workspace {
	rows := make([]domain.Workspace, 0, len(ids))
	for _, id := range ids {
		if ws, ok := index[id]; ok && ws.Status != domain.WorkspaceStatusDeleted {
			rows = append(rows, ws)
		}
	}
	return rows
}

func nodesFrom(
	all []domain.Workspace,
) []cascade.Node {
	nodes := make([]cascade.Node, 0, len(all))
	for _, ws := range all {
		nodes = append(nodes, cascade.Node{
			ID:     ws.ID,
			Parent: ws.ParentID,
			Locked: ws.Status == domain.WorkspaceStatusLocked,
		})
	}
	return nodes
}

func indexByID(
	all []domain.Workspace,
) map[string]domain.Workspace {
	index := make(map[string]domain.Workspace, len(all))
	for _, ws := range all {
		index[ws.ID] = ws
	}
	return index
}
