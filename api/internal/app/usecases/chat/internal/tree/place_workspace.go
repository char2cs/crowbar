package tree

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

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
		return domain.Chat{}, nil, fmt.Errorf("agent chat folder: place workspace %s: %w", workspaceID, problem.ErrNotFound)
	}
	nodeID := workspaceID
	var current domain.Chat
	if renders, rErr := u.workspaces.RendersAsBranch(ctx, workspaceID); rErr == nil && !renders {
		if rows, cErr := u.chats.ListByWorkspace(ctx, workspaceID); cErr == nil {
			if owner, ok := domain.ResolveOwningChat(rows); ok {
				nodeID = owner.ID
				current, err = u.correctHomePlacement(ctx, owner)
				if err != nil {
					return domain.Chat{}, nil, err
				}
			}
		}
	}
	if current.ID == "" {
		n, nErr := u.nodes.GetNode(ctx, nodeID)
		if nErr != nil && problem.CodeOf(nErr) != problem.NotFound {
			return domain.Chat{}, nil, fmt.Errorf("agent chat folder: place workspace %s: read node: %w", nodeID, nErr)
		}
		current, err = u.anchorView(ctx, nodeID, n)
	}
	if err != nil {
		return domain.Chat{}, nil, err
	}
	snapshot, err := u.globalSnapshotAround(ctx, current)
	if err != nil {
		return domain.Chat{}, nil, err
	}
	destination := current.ParentID
	if in.ParentID != nil {
		destination = *in.ParentID
	}
	if err := u.ensureWorkspaceAnchor(ctx, destination); err != nil {
		return domain.Chat{}, nil, err
	}
	if mErr := u.checkWorkspaceMove(ctx, snapshot, workspaceMove{
		repoID:      repoID,
		workspaceID: workspaceID,
		nodeID:      nodeID,
		origin:      current.ParentID,
		destination: destination,
	}); mErr != nil {
		return domain.Chat{}, nil, mErr
	}
	if wErr := guardNotWorking(ctx, snapshot.subtreeIDs(nodeID), u.busy); wErr != nil {
		return domain.Chat{}, nil, wErr
	}
	u.replace(snapshot, nodeID, current.ParentID, destination, in.Order, false)
	written, err := u.persist(ctx, snapshot)
	if err != nil {
		return domain.Chat{}, nil, err
	}
	return *snapshot.placedRow(nodeID), without(written, nodeID), nil
}
