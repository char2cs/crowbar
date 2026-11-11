package tree

import (
	"context"
	"fmt"
	"slices"
)

// The check that stands between a cascading chat delete and a worktree somebody
// else is still working in.

// WorkspaceHolders answers every chat resolving to a workspace. A worktree is shared by many chats by
// design, so a chat naming a workspace proves it uses it, not that it owns it alone; declared here so
// "who holds this worktree" has one answer, the same resolver the chat-scoped routes use.
type WorkspaceHolders interface {
	ChatsForWorkspace(
		ctx context.Context,
		workspaceID string,
	) ([]string, error)
}

// heldElsewhere reports whether any chat outside the doomed subtree still resolves to workspaceID, which
// decides whether a delete may reap it. A failed read fails the delete: guessing would assume sole
// ownership and could destroy a worktree a surviving chat still uses.
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

// doomedSet indexes the subtree a delete is about to purge, so the holder check
// can subtract it in one pass per workspace rather than scanning the slice.
func doomedSet(
	ids []string,
) map[string]bool {
	doomed := make(map[string]bool, len(ids))
	for _, id := range ids {
		doomed[id] = true
	}
	return doomed
}
