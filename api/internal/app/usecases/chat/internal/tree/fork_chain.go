package tree

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// The one boundary a WORKSPACE placement owes beyond the repo scope every row
// shares: its own fork chain. Organisation and git lineage are separate edges
// here — which is exactly why one can be dragged out from under the other.

// checkForkChainSplit refuses a workspace placement that carries its row out of its fork parent's space:
// the sidebar always draws a fork under its fork parent, so such a drop would succeed and visibly do
// nothing (reparenting is the route for that). Only a container change is checked: a reorder splits nothing.
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

// checkOwnedForkChain is checkForkChainSplit for a chat that owns a repo
// workspace: moving that chat moves the workspace's row, so the chat route
// owes the workspace route's guard.
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

// isRepoGround: "" and the repo's default checkout both name the repo root,
// where a fork with no visible parent belongs.
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

// forkAnchorAt answers the workspace a row placed at id would fork from: the nearest ancestor-or-self with a
// WorkspaceID, "" at the root — CwdWorkspaceID's rule, which minted the fork parent it is judged against.
// Unlike nearestWorkspaceAnchor it stops at any WorkspaceID, since an ordinary fork is drawn by its chat.
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

// livePlacementOf answers the container a row is in now: its Node's ParentID when it has one (Chat.ParentID
// is frozen then), else its own ParentID.
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
