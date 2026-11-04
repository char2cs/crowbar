package hierarchy

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/cascade"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func (u *hierarchyUsecase) guardReparent(
	ctx context.Context,
	child domain.Workspace,
	newParent domain.Workspace,
) error {
	if child.ID == newParent.ID {
		return ErrSelfParent
	}
	if newParent.Provisioning == domain.WorkspacePlaceholder {
		return ErrParentUnprovisioned
	}
	// A locked (protected) branch is a valid re-parent target: create already seeds children under one.
	hasKids, err := u.childHasChildren(ctx, child.ID)
	if err != nil {
		return fmt.Errorf("reparent: leaf check: %w", err)
	}
	if hasKids {
		return ErrChildHasChildren
	}
	if workingErr := u.guardNotWorking(ctx, child.ID); workingErr != nil {
		return workingErr
	}
	// A row that owns a worktree cannot move across repos (a different checkout entirely); a row with no
	// worktree of its own can.
	if child.RepoID != newParent.RepoID && child.Provisioning.HasWorktree() {
		return ErrCrossRepoWorktreeMove
	}
	return nil
}

// guardNotWorking refuses a move over a workspace subtree that owns a working
// chat (invariant 5), walking the subtree with cascade.Plan as the chat tree
// does. A nil chatObserver makes it a no-op, like TerminalReaper.
func (u *hierarchyUsecase) guardNotWorking(
	ctx context.Context,
	rootID string,
) error {
	if u.chatObserver == nil {
		return nil
	}
	all, err := u.workspaces.List(ctx)
	if err != nil {
		return fmt.Errorf("guard not working: list: %w", err)
	}
	for _, wsID := range subtreeWorkspaceIDs(rootID, all) {
		working, workingErr := u.anyChatWorkingIn(ctx, wsID)
		if workingErr != nil {
			return workingErr
		}
		if working {
			return ErrWorkspaceWorking
		}
	}
	return nil
}

// anyChatWorkingIn reports whether any chat owned by workspaceID is currently
// working.
func (u *hierarchyUsecase) anyChatWorkingIn(
	ctx context.Context,
	workspaceID string,
) (bool, error) {
	chats, err := u.chatObserver.ListChatsByWorkspace(ctx, workspaceID)
	if err != nil {
		return false, fmt.Errorf("guard not working: list chats: %w", err)
	}
	for _, c := range chats {
		busy, err := u.chatObserver.ChatBusy(ctx, c.ID)
		if err != nil {
			return false, fmt.Errorf("guard not working: %w", err)
		}
		if busy {
			return true, nil
		}
	}
	return false, nil
}

// subtreeWorkspaceIDs returns rootID and every workspace below it. Unlike the delete cascade it never
// skips a locked workspace, which can still own a working chat.
func subtreeWorkspaceIDs(
	rootID string,
	all []domain.Workspace,
) []string {
	nodes := make([]cascade.Node, 0, len(all))
	for _, ws := range all {
		nodes = append(nodes, cascade.Node{ID: ws.ID, Parent: ws.ParentID})
	}
	return cascade.Plan(rootID, nodes)
}

func (u *hierarchyUsecase) childHasChildren(
	ctx context.Context,
	childID string,
) (bool, error) {
	all, err := u.workspaces.List(ctx)
	if err != nil {
		return false, err
	}
	for _, ws := range all {
		// A self-loop (ws.ParentID == ws.ID) must not count the node as its own
		// child, or a corrupted self-parented workspace becomes unreparentable.
		if ws.ParentID == childID && ws.ID != childID {
			return true, nil
		}
	}
	return false, nil
}
