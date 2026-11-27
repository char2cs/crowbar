package tree

import (
	"context"
	"fmt"
	"slices"
)

type WorkspaceHolders interface {
	ChatsForWorkspace(
		ctx context.Context,
		workspaceID string,
	) ([]string, error)
}

func (u *chatFolderUsecase) heldElsewhere(
	ctx context.Context,
	workspaceID string,
	doomed map[string]bool,
) (bool, error) {
	holders, err := u.holders.ChatsForWorkspace(ctx, workspaceID)
	if err != nil {
		return false, fmt.Errorf("chats holding worktree %s: %w", workspaceID, err)
	}
	return slices.ContainsFunc(holders, func(id string) bool { return !doomed[id] }), nil
}

func doomedSet(
	ids []string,
) map[string]bool {
	doomed := make(map[string]bool, len(ids))
	for _, id := range ids {
		doomed[id] = true
	}
	return doomed
}
