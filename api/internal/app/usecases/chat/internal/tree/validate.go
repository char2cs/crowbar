package tree

import (
	"context"
	"fmt"
)

// The three checks every move passes before anything is written.

// checkMove refuses a move onto a container that does not exist in the
// workspace, belongs to another workspace, or lies inside the moved row's own
// subtree.
func (u *chatFolderUsecase) checkMove(
	ctx context.Context,
	snapshot *treeSnapshot,
	workspaceID string,
	id string,
	destination string,
) error {
	if destination == id {
		return fmt.Errorf("agent chat folder: move %s onto itself: %w", id, ErrCycle)
	}
	if err := u.checkContainer(ctx, snapshot, workspaceID, destination); err != nil {
		return err
	}
	if snapshot.plan.Reaches(destination, id) {
		return fmt.Errorf("agent chat folder: move %s under %s: %w", id, destination, ErrCycle)
	}
	return nil
}

// checkContainer validates a parent id: "" is the panel root, and anything else
// must be one of this workspace's folders or chats. A row that exists under a
// DIFFERENT workspace is reported as a cross-workspace edge rather than as a
// missing row, because the two are fixed in different ways.
func (u *chatFolderUsecase) checkContainer(
	ctx context.Context,
	snapshot *treeSnapshot,
	workspaceID string,
	parentID string,
) error {
	if parentID == "" {
		return nil
	}
	if snapshot.folder(parentID) != nil || snapshot.chat(parentID) != nil {
		return nil
	}
	elsewhere, err := u.folders.FindByKey(ctx, parentID)
	if err != nil {
		return fmt.Errorf("agent chat folder: resolve parent %s: %w", parentID, err)
	}
	if elsewhere != nil {
		return fmt.Errorf("agent chat folder: parent %s is in another workspace: %w", parentID, ErrCrossWorkspace)
	}
	return u.checkChatContainer(ctx, workspaceID, parentID)
}

// checkChatContainer resolves a parent id the workspace's own rows did not
// answer for against the CHAT aggregate, which is keyed globally. A chat in
// another workspace is a cross-workspace edge; a lookup that fails is surfaced
// as it came, so a read failure reaches the caller as one rather than as a
// confident "no such row" the user would go looking for.
//
// A chat that resolves to THIS workspace is accepted rather than refused, and
// that case is real: the keyed read heals the chat read model for the one id it
// was asked about, while the workspace list only heals a model that is entirely
// empty — so the authoritative answer here can name a row the snapshot's list
// did not carry. Refusing it would reject a drop onto a chat the user can see.
func (u *chatFolderUsecase) checkChatContainer(
	ctx context.Context,
	workspaceID string,
	parentID string,
) error {
	chat, err := u.chats.GetChat(ctx, parentID)
	if err != nil {
		return fmt.Errorf("agent chat folder: parent %s: %w", parentID, err)
	}
	if chat.WorkspaceID == workspaceID {
		return nil
	}
	return fmt.Errorf(
		"agent chat folder: parent %s belongs to workspace %s, not %s: %w",
		parentID, chat.WorkspaceID, workspaceID, ErrCrossWorkspace,
	)
}
