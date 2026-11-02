package tree

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
	dtree "github.com/char2cs/crowbar/api/internal/domain/tree"
)

// The Node-backed write dispatch: minting, ordering or placing one row.

// writeHomeNode is writeRow's Node-backed body (plan.go), against Nodes
// instead of Chats. snapshot.freshIDs is a walk-discovery signal, not ground
// truth — a row filed under an owning chat the BFS can't walk through reads
// as fresh though its Node row exists — so a fresh id is verified with a
// GetNode before deciding Create vs SetPlacement.
func (u *chatFolderUsecase) writeHomeNode(
	ctx context.Context,
	snapshot *treeSnapshot,
	row *domain.Chat,
) (*domain.Chat, error) {
	kind := dtree.KindChat
	switch row.Type {
	case domain.ChatTypeFolder:
		kind = dtree.KindFolder
	case domain.ChatTypeChat, domain.ChatTypeBranch, domain.ChatTypeWorkflow:
		// kind is already NodeKindChat.
	case nodePhantomType:
		// A repo's own Node row riding through this Chat-shaped snapshot,
		// not a member of the domain.ChatType enum.
		kind = dtree.KindRepo
	case workspaceAnchorType:
		// A workspace anchor's own Node row; distinct from nodePhantomType
		// only so a fresh mint lands on the right Kind.
		kind = dtree.KindWorkspace
	}
	if snapshot.freshIDs[row.ID] {
		if _, err := u.nodes.GetNode(ctx, row.ID); err == nil {
			// A real row exists despite the walk missing it -- SetPlacement,
			// not Create, is the correct write once that is known.
			if err := u.nodes.SetPlacement(ctx, row.ID, row.ParentID, row.Order); err != nil {
				return nil, fmt.Errorf("agent chat folder: node place %s: %w", row.ID, err)
			}
			return row, nil
		}
		if _, err := u.nodes.Create(ctx, row.ID, kind, row.ParentID, row.Order); err != nil {
			return nil, fmt.Errorf("agent chat folder: node create %s: %w", row.ID, err)
		}
		return row, nil
	}
	if !snapshot.reparented(row.ID) {
		if err := u.nodes.SetOrder(ctx, row.ID, row.Order); err != nil {
			return nil, fmt.Errorf("agent chat folder: node order %s: %w", row.ID, err)
		}
		return row, nil
	}
	if err := u.nodes.SetPlacement(ctx, row.ID, row.ParentID, row.Order); err != nil {
		return nil, fmt.Errorf("agent chat folder: node place %s: %w", row.ID, err)
	}
	return row, nil
}
