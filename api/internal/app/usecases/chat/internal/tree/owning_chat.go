package tree

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// The chat-first primitive for the creation paths that do their own git work.
//
// CreateChat's own WorktreeImport branch (imported_chat.go) is the front door
// for importing ONE branch, and it owns the whole sequence. The paths here are
// the ones that cannot hand their creation over that way: adopting the repo's
// existing folder in place, provisioning a locked worktree for a protected
// branch, recording a placeholder for a branch some other checkout is holding,
// and the project-level home row that has no repo and no branch at all. Each
// already knows exactly which row it wants and has done the git work to earn
// it; what none of them had was a chat.
//
// So they get the three verbs that make the chat come FIRST, and keep their own
// rollback — which they must, because only they know what they built on disk:
//
//	chatID, err := chats.MintOwningChat(ctx, parentWorkspaceID)
//	ws, err := <git work + workspace row>          // fails → DiscardOwningChat
//	err := chats.AttachOwningWorkspace(ctx, chatID, ws)
//
// Splitting it this way rather than taking a callback keeps the git work, and
// the rollback that understands it, in the package that owns it — the chat
// usecase never learns what a worktree is — while still making a workspace with
// no owning chat unrepresentable: the row that owns it is minted before it, and
// discarded with it.

// MintOwningChat mints the chat that is about to own a workspace and places it
// under parentWorkspaceID's own Node row.
//
// parentWorkspaceID is a WORKSPACE id — the git-lineage parent the caller
// already resolved — and it is ALSO the placement id: every workspace mints
// its own Node{Kind:workspace} row, keyed by its own id, the instant it is
// created (2026-09-08 sidebar-placement-unification Task 7), so the new chat
// simply hangs its ParentID off that same id (see owningChatOf) — no
// proxy-chat lookup to resolve, and no separate join to keep in sync with a
// workspace's own git lineage.
//
// An empty parentWorkspaceID, or one whose Node row does not exist (an id no
// live workspace answers to), places the new chat at the panel root — the
// same answer forkParentOf gives a workspace whose recorded parent is no
// longer there.
func (u *chatFolderUsecase) MintOwningChat(
	ctx context.Context,
	parentWorkspaceID string,
) (string, error) {
	parentChatID, err := u.owningChatOf(ctx, parentWorkspaceID)
	if err != nil {
		return "", err
	}
	chatID, err := u.agent.MintChat(ctx, "")
	if err != nil {
		return "", fmt.Errorf("agent chat folder: mint owning chat: %w", err)
	}
	if pErr := u.placeOwningRow(ctx, chatID, parentChatID); pErr != nil {
		return "", u.discard(ctx, chatID, pErr)
	}
	return chatID, nil
}

// placeOwningRow files a freshly minted owning row at the end of its level,
// taking the index from a GLOBAL read of the forest, deliberately not through
// placeChat.
//
// placeChat plans against workspaceSnapshot, which for the workspace-less scope
// an owning row is minted in reads ONLY the rows that still have no workspace.
// That is right for a bubble, and wrong here: an owning row leaves that scope
// the moment it is attached, so a second one planning against it cannot see the
// first, and every row in a repo import would be renumbered against a level it
// can only partly see. The result is a panel root where two rows hold order 0
// and the next drop index means nothing.
//
// Counting the whole level instead is the same answer: the first free index in
// that sibling space.
func (u *chatFolderUsecase) placeOwningRow(
	ctx context.Context,
	chatID string,
	parentChatID string,
) error {
	rows, err := u.chats.ListChats(ctx)
	if err != nil {
		return fmt.Errorf("agent chat folder: place owning row: %w", err)
	}
	order := 0
	for _, row := range rows {
		if row.ID != chatID && row.ParentID == parentChatID {
			order++
		}
	}
	if _, err := u.chats.SetPlacement(ctx, chatID, parentChatID, order); err != nil {
		return fmt.Errorf("agent chat folder: place owning row %s: %w", chatID, err)
	}
	return nil
}

// AttachOwningWorkspace points a minted owning chat at the workspace it was
// minted for.
func (u *chatFolderUsecase) AttachOwningWorkspace(
	ctx context.Context,
	chatID string,
	ws domain.Workspace,
) error {
	if err := u.agent.AttachWorkspace(ctx, chatID, ws.ID); err != nil {
		return fmt.Errorf("agent chat folder: attach workspace %s to %s: %w", ws.ID, chatID, err)
	}
	return nil
}

// DiscardOwningChat takes a minted owning chat back out, for a caller whose own
// workspace creation then failed. It is the compensating half of MintOwningChat
// and exists for the same reason WorktreeCreator carries its own discard: the
// chat is created before the thing it owns, so an abandoned create leaves a row
// pointing at nothing unless somebody takes it away again.
func (u *chatFolderUsecase) DiscardOwningChat(
	ctx context.Context,
	chatID string,
) error {
	if err := u.agent.PurgeChat(ctx, chatID); err != nil {
		return fmt.Errorf("agent chat folder: discard owning chat %s: %w", chatID, err)
	}
	return nil
}

// owningChatOf resolves the placement id a new row filed under workspaceID
// hangs off, or "" when workspaceID is empty or unknown.
//
// It answers directly off workspaceID's own Node row (2026-09-08
// sidebar-placement-unification Task 9): every workspace mints one,
// keyed by its own id, unconditionally at creation, so that id IS the
// answer — there is no proxy chat left to resolve through. The GetNode call
// is only an EXISTENCE check (a garbage or since-deleted workspace id must
// still degrade to the panel root rather than file a row under an id
// nothing answers to), not a lookup of anything besides workspaceID itself.
func (u *chatFolderUsecase) owningChatOf(
	ctx context.Context,
	workspaceID string,
) (string, error) {
	if workspaceID == "" {
		return "", nil
	}
	if _, err := u.nodes.GetNode(ctx, workspaceID); err != nil {
		return "", nil
	}
	return workspaceID, nil
}
