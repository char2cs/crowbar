package tree

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
	dtree "github.com/char2cs/crowbar/api/internal/domain/tree"
)

// The Node-backed write dispatch: minting, ordering or placing one row.

// writeHomeNode is writeRow's Node-backed body. freshIDs only means the walk found no Node row, not that
// none exists (a row under an unwalked owning chat reads as fresh), so a fresh id is checked with GetNode
// before choosing Create over SetPlacement.
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
