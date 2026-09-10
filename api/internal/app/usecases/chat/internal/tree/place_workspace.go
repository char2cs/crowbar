package tree

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// PlaceWorkspace moves a workspace's own row within its repo's tree —
// addressed by workspaceID (the frontend's one call for EVERY worktree-
// owning row's drag, drop-actions.ts's planTreeRowDrop, unconditional on
// lock status), but NOT always writing workspaceID's own Node row: see
// nodeID's own doc below for why an ordinary fork resolves through its
// owning chat instead (2026-09-09, fixed same day as this route shipped —
// caught live, dragging an ordinary fork PATCHed 200 and never moved).
// Shares checkWorkspaceMove/checkFolderContainer's repo-scoped golden rule
// rather than checkChatMove's same-workspace one — see checkWorkspaceMove's
// own doc for why.
//
// It refuses with apperr.ErrNotFound only for a workspaceID RepoOf cannot
// resolve to a real repo — a nonexistent id, or the project's own HOME
// workspace (RepoOf answers "" for it; it is never a member of a repo's
// tree at all, and the frontend already never calls this for it — see
// planTreeRowDrop's own defaultWorkspaceId guard).
//
// A row whose Node has never been touched (a pre-existing row older than
// the Node migration, or simply one whose position this call is the very
// first write to) degrades to a zero-value ParentID/Order exactly like a
// fresh mint: globalSnapshotAround's subjectIsNodeBacked marks it
// snapshot.freshIDs when mergeForest's own walk never discovered a Node row
// for it either, and persist's dispatch (writeHomeNode) mints one via
// Nodes.Create rather than trying to move a row that was never there.
func (u *chatFolderUsecase) PlaceWorkspace(
	ctx context.Context,
	workspaceID string,
	in PlaceInput,
) (domain.Chat, []domain.Chat, error) {
	repoID, err := u.workspaces.RepoOf(ctx, workspaceID)
	if err != nil {
		return domain.Chat{}, nil, fmt.Errorf("agent chat folder: place workspace %s: %w", workspaceID, err)
	}
	if repoID == "" {
		return domain.Chat{}, nil, fmt.Errorf(
			"agent chat folder: place workspace %s: %w", workspaceID, apperr.ErrNotFound)
	}
	// nodeID is whichever Node row the PANEL actually reads this row's
	// position from, not always workspaceID itself. mergeHomeNode's own
	// NodeKindWorkspace case excludes an ordinary fork's workspace-anchor
	// Node from the merge entirely (RendersAsBranch false — "already
	// represented 1:1 by the chat that owns it") in favour of ITS chat-kind
	// Node instead; only a LOCKED branch, whose owning chat carries no
	// Node of its own (placeOwningRow, never Node-backed), is read from its
	// workspace-anchor Node. Writing workspaceID unconditionally moved a
	// fork's abandoned, never-rendered anchor row while the panel kept
	// showing its untouched chat-kind Node's stale position — caught live:
	// dragging an ordinary fork PATCHed 200 and never visibly moved.
	nodeID := workspaceID
	var current domain.Chat
	if renders, rErr := u.workspaces.RendersAsBranch(ctx, workspaceID); rErr == nil && !renders {
		if rows, cErr := u.chats.ListByWorkspace(ctx, workspaceID); cErr == nil {
			if owner, ok := domain.ResolveOwningChat(rows); ok {
				nodeID = owner.ID
				// The REAL chat, not a synthetic workspaceAnchorType stand-in:
				// an ordinary fork already has an honest domain.Chat (its own
				// WorkspaceID, Type, RepoID) — workspaceAnchorView's ID ==
				// WorkspaceID convention would otherwise mislabel this row's
				// WorkspaceID as its own chat id.
				current, err = u.correctHomePlacement(ctx, owner)
				if err != nil {
					return domain.Chat{}, nil, err
				}
			}
		}
	}
	if current.ID == "" {
		n, nErr := u.nodes.GetNode(ctx, nodeID)
		if nErr != nil {
			n = domain.Node{}
		}
		current = workspaceAnchorView(nodeID, n)
	}
	snapshot, err := u.globalSnapshotAround(ctx, current)
	if err != nil {
		return domain.Chat{}, nil, err
	}
	destination := current.ParentID
	if in.ParentID != nil {
		destination = *in.ParentID
	}
	if mErr := u.checkWorkspaceMove(ctx, snapshot, repoID, nodeID, destination); mErr != nil {
		return domain.Chat{}, nil, mErr
	}
	if wErr := guardNotWorking(subtreeIDsOf(nodeID, snapshot.rows), u.work); wErr != nil {
		return domain.Chat{}, nil, wErr
	}
	u.replace(snapshot, nodeID, current.ParentID, destination, in.Order, false)
	written, err := u.persist(ctx, snapshot)
	if err != nil {
		return domain.Chat{}, nil, err
	}
	return *snapshot.placedRow(nodeID), without(written, nodeID), nil
}
