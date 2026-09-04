package tree

import (
	"context"
	"fmt"
	"slices"
)

// The check that stands between a cascading chat delete and a worktree somebody
// else is still working in.

// WorkspaceHolders is the narrow read port reapWorktrees needs: every chat id
// currently resolving to a workspace, itself included when it owns the
// worktree.
//
// It exists because Chat.WorkspaceID answers a NARROWER question than the reap
// was reading it as. It says which workspace a chat is anchored to — not that
// the chat is the only one anchored there. A worktree is many-chats-to-one by
// design (spec §3): ordinary sibling conversations legitimately share one, and
// every shared read this refactor added (git, review, files, search, identity)
// fans out over exactly that set. So a row naming a workspace is evidence the
// chat USES it, never evidence it owns it alone.
//
// Declared here rather than imported from usecases/worktree (law 3, law 4):
// worktree.ChatsForWorkspace already answers exactly this, and the container
// hands the tree the same adapter the chat-scoped routes fan out through, so
// "who holds this worktree" has one answer in the daemon rather than two that
// could drift.
type WorkspaceHolders interface {
	ChatsForWorkspace(
		ctx context.Context,
		workspaceID string,
	) ([]string, error)
}

// heldElsewhere reports whether any chat OUTSIDE the doomed subtree still
// resolves to workspaceID — the question that decides whether a delete may tear
// that worktree down.
//
// The subtraction is what makes it correct in both directions. Every row inside
// the subtree is about to be purged, so its hold on the workspace is expiring
// with it and must not count; anything left over is a chat that will still
// exist, still pointing at the worktree, when the delete finishes. Empty
// remainder means nothing survives to name the directory — reap it, or produce
// the §0 orphan. Non-empty means a sibling is still working there.
//
// A failed read is a failed DELETE, never a guess. The whole point of asking is
// that the answer is not derivable from the doomed rows alone, so a caller that
// swallowed the error would be back to assuming sole ownership — the exact
// assumption that destroyed a shared worktree.
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
