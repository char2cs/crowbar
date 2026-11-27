package tree

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func (u *chatFolderUsecase) checkForkChainSplit(
	ctx context.Context,
	snapshot *treeSnapshot,
	move workspaceMove,
) error {
	if move.destination == move.origin {
		return nil
	}
	forkParentID, err := u.workspaces.VisibleForkParent(ctx, move.workspaceID)
	if err != nil {
		return fmt.Errorf("agent chat folder: place workspace %s: %w", move.workspaceID, err)
	}
	anchor, err := u.forkAnchorAt(ctx, snapshot, move.destination)
	if err != nil {
		return fmt.Errorf(
			"agent chat folder: place workspace %s under %s: %w",
			move.workspaceID, move.destination, err,
		)
	}
	if anchor == forkParentID || u.isRepoGround(ctx, anchor) && u.isRepoGround(ctx, forkParentID) {
		return nil
	}
	return fmt.Errorf(
		"agent chat folder: place workspace %s under %s, whose fork parent is %q, not %q: %w",
		move.workspaceID, move.destination, anchor, forkParentID, ErrForkChain,
	)
}

func (u *chatFolderUsecase) checkOwnedForkChain(
	ctx context.Context,
	snapshot *treeSnapshot,
	chat domain.Chat,
	destination string,
) error {
	if !chat.OwnsWorkspace || chat.WorkspaceID == "" {
		return nil
	}
	repoID, err := u.workspaces.RepoOf(ctx, chat.WorkspaceID)
	if err != nil {
		return fmt.Errorf("agent chat folder: place %s: repo of %s: %w", chat.ID, chat.WorkspaceID, err)
	}
	if repoID == "" {
		return nil
	}
	if def, err := u.workspaces.DefaultWorkspaceOf(ctx, repoID); err == nil && def == chat.WorkspaceID {
		return nil
	}
	return u.checkForkChainSplit(ctx, snapshot, workspaceMove{
		repoID: repoID, workspaceID: chat.WorkspaceID, nodeID: chat.ID,
		origin: chat.ParentID, destination: destination,
	})
}

func (u *chatFolderUsecase) isRepoGround(ctx context.Context, workspaceID string) bool {
	if workspaceID == "" {
		return true
	}
	repoID, err := u.workspaces.RepoOf(ctx, workspaceID)
	if err != nil || repoID == "" {
		return false
	}
	def, err := u.workspaces.DefaultWorkspaceOf(ctx, repoID)
	return err == nil && def == workspaceID
}

func (u *chatFolderUsecase) forkAnchorAt(
	ctx context.Context,
	snapshot *treeSnapshot,
	id string,
) (string, error) {
	seen := map[string]bool{}
	for id != "" && !seen[id] {
		seen[id] = true
		row, err := u.resolveRow(ctx, snapshot, id)
		if err != nil {
			return "", fmt.Errorf("resolve %s: %w", id, err)
		}
		if row.WorkspaceID != "" {
			return row.WorkspaceID, nil
		}
		id = u.livePlacementOf(ctx, *row)
	}
	return "", nil
}

func (u *chatFolderUsecase) livePlacementOf(
	ctx context.Context,
	row domain.Chat,
) string {
	n, err := u.nodes.GetNode(ctx, row.ID)
	if err != nil {
		return row.ParentID
	}
	return n.ParentID
}
