package tree

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
	dtree "github.com/char2cs/crowbar/api/internal/domain/tree"
)

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
	case nodePhantomType:
		kind = dtree.KindRepo
	case workspaceAnchorType:
		kind = dtree.KindWorkspace
	}
	if snapshot.freshIDs[row.ID] {
		if _, err := u.nodes.GetNode(ctx, row.ID); err == nil {
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
