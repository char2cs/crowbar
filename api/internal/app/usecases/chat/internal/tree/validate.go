package tree

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// The checks every move passes before anything is written. Folder ops
// (repo-wide, no workspace of their own) and chat ops (workspace-scoped) carry
// different container rules, so each gets its own pair below.

// checkFolderMove refuses a move onto a container that does not exist, or lies
// inside the moved row's own subtree.
func (u *chatFolderUsecase) checkFolderMove(
	ctx context.Context,
	snapshot *treeSnapshot,
	id string,
	destination string,
) error {
	if destination == id {
		return fmt.Errorf("agent chat folder: move %s onto itself: %w", id, ErrCycle)
	}
	if err := u.checkFolderContainer(ctx, snapshot, destination); err != nil {
		return err
	}
	if snapshot.plan.Reaches(destination, id) {
		return fmt.Errorf("agent chat folder: move %s under %s: %w", id, destination, ErrCycle)
	}
	return nil
}

// checkFolderContainer validates a folder's parent id: "" is the panel root,
// and anything else must be a row that exists. A folder carries no workspace of
// its own, so there is no boundary left to check a parent against here — that
// enforcement is the repo-forest walk stage 3 adds, not this task's storage
// retype.
func (u *chatFolderUsecase) checkFolderContainer(
	ctx context.Context,
	snapshot *treeSnapshot,
	parentID string,
) error {
	if parentID == "" {
		return nil
	}
	if snapshot.row(parentID) != nil {
		return nil
	}
	if _, err := u.chats.Get(ctx, parentID); err != nil {
		return fmt.Errorf("agent chat folder: parent %s: %w", parentID, err)
	}
	return nil
}

// checkChatMove refuses a chat move onto a container that does not exist,
// belongs to another workspace, or lies inside the moved chat's own subtree.
func (u *chatFolderUsecase) checkChatMove(
	ctx context.Context,
	snapshot *treeSnapshot,
	workspaceID string,
	id string,
	destination string,
) error {
	if destination == id {
		return fmt.Errorf("agent chat folder: move %s onto itself: %w", id, ErrCycle)
	}
	if err := u.checkChatContainer(ctx, snapshot, workspaceID, destination); err != nil {
		return err
	}
	if snapshot.plan.Reaches(destination, id) {
		return fmt.Errorf("agent chat folder: move %s under %s: %w", id, destination, ErrCycle)
	}
	return nil
}

// checkChatContainer validates a chat's parent id: "" is the panel root, a
// FOLDER or a BRANCH is accepted unconditionally (neither carries a workspace
// to conflict with — a branch row is a process boundary, not a workspace one),
// and a CHAT must belong to workspaceID. A row that resolves to a DIFFERENT
// workspace is reported as a cross-workspace edge rather than as a missing
// row, because the two are fixed in different ways.
func (u *chatFolderUsecase) checkChatContainer(
	ctx context.Context,
	snapshot *treeSnapshot,
	workspaceID string,
	parentID string,
) error {
	if parentID == "" {
		return nil
	}
	if row := snapshot.row(parentID); row != nil {
		return checkParentKind(*row, workspaceID, parentID)
	}
	row, err := u.chats.Get(ctx, parentID)
	if err != nil {
		return fmt.Errorf("agent chat folder: parent %s: %w", parentID, err)
	}
	return checkParentKind(row, workspaceID, parentID)
}

// checkParentKind is the second half of checkChatContainer, split out because
// both the snapshot-membership path and the keyed-lookup fallback answer the
// same question about the row once they have it.
//
// The keyed read heals the chat read model for the one id it was asked about;
// the workspace list only heals a model that is entirely empty — so the
// authoritative answer here can name a row the snapshot's list did not carry.
// Refusing it would reject a drop onto a chat the user can see.
func checkParentKind(
	row domain.Chat,
	workspaceID string,
	parentID string,
) error {
	if row.Type == domain.ChatTypeFolder || row.Type == domain.ChatTypeBranch {
		return nil
	}
	if row.WorkspaceID == workspaceID {
		return nil
	}
	return fmt.Errorf(
		"agent chat folder: parent %s belongs to workspace %s, not %s: %w",
		parentID, row.WorkspaceID, workspaceID, ErrCrossWorkspace,
	)
}
