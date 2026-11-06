package tree

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Creation paths that do their own git work (repo folder, locked worktree, placeholder, project
// home) call MintOwningChat before the workspace, DiscardOwningChat if the git work fails and
// AttachOwningWorkspace after, so no workspace exists unowned while rollback stays theirs.

// MintOwningChat mints the chat about to own a workspace under parentWorkspaceID's Node row (an
// empty or unknown parent places it at the root); chatID "" mints a fresh id. It records the
// default provider: a workspace's row is prompted without the user ever picking an agent.
func (u *chatFolderUsecase) MintOwningChat(
	ctx context.Context,
	chatID string,
	parentWorkspaceID string,
) (string, error) {
	parentChatID, err := u.owningChatOf(ctx, parentWorkspaceID)
	if err != nil {
		return "", err
	}
	providerID, err := u.agent.DefaultProviderID(ctx)
	if err != nil {
		return "", fmt.Errorf("agent chat folder: mint owning chat: default provider: %w", err)
	}
	chatID, err = u.agent.MintChat(ctx, chatID, "", providerID, "")
	if err != nil {
		return "", fmt.Errorf("agent chat folder: mint owning chat: %w", err)
	}
	if pErr := u.placeOwningRow(ctx, chatID, parentChatID); pErr != nil {
		return "", u.discard(ctx, chatID, pErr)
	}
	return chatID, nil
}

// placeOwningRow files a freshly minted owning row at the next free index of its level, counted globally
// rather than through placeChat: the row leaves the workspace-less scope once attached, so rows planned in
// that scope would not see each other and several would claim the same index.
func (u *chatFolderUsecase) placeOwningRow(
	ctx context.Context,
	chatID string,
	parentChatID string,
) error {
	order, err := u.owningRowSlot(ctx, chatID, parentChatID)
	if err != nil {
		return err
	}
	if _, err := u.chats.SetPlacement(ctx, chatID, parentChatID, order); err != nil {
		return fmt.Errorf("agent chat folder: place owning row %s: %w", chatID, err)
	}
	return nil
}

// owningRowSlot answers the first free index in the level parentID names. A workspace parent is counted
// through the scope/plan machinery, since its level may be another id's (a default checkout's is the repo
// root); any other container is a direct count. The row itself is dropped first: it already sits at the root.
func (u *chatFolderUsecase) owningRowSlot(
	ctx context.Context,
	chatID string,
	parentID string,
) (int, error) {
	live, err := u.workspaces.Exists(ctx, parentID)
	if err != nil || !live {
		return u.rowsFiledUnder(ctx, chatID, parentID)
	}
	snapshot, sErr := u.workspaceSnapshotAround(ctx, parentID, domain.Chat{}, parentID)
	if sErr != nil {
		return 0, sErr
	}
	snapshot.drop(chatID)
	return snapshot.plan.NextSlot(snapshot.canonical(parentID)), nil
}

// rowsFiledUnder counts the Chat rows already filed under parentID, chatID
// excluded.
func (u *chatFolderUsecase) rowsFiledUnder(
	ctx context.Context,
	chatID string,
	parentID string,
) (int, error) {
	rows, err := u.chats.ListChats(ctx)
	if err != nil {
		return 0, fmt.Errorf("agent chat folder: place owning row: %w", err)
	}
	order := 0
	for _, row := range rows {
		if row.ID != chatID && row.ParentID == parentID {
			order++
		}
	}
	return order, nil
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

// DiscardOwningChat takes a minted owning chat back out when the caller's workspace creation failed, so an
// abandoned create leaves no row pointing at nothing.
func (u *chatFolderUsecase) DiscardOwningChat(
	ctx context.Context,
	chatID string,
) error {
	if err := u.agent.PurgeChat(ctx, chatID); err != nil {
		return fmt.Errorf("agent chat folder: discard owning chat %s: %w", chatID, err)
	}
	return nil
}

// owningChatOf resolves the placement id a row filed under workspaceID hangs off: workspaceID itself, whose
// anchor Node is ensured first, or "" (the panel root) when workspaceID is empty or has no Node row.
func (u *chatFolderUsecase) owningChatOf(
	ctx context.Context,
	workspaceID string,
) (string, error) {
	if workspaceID == "" {
		return "", nil
	}
	if err := u.ensureWorkspaceAnchor(ctx, workspaceID); err != nil {
		return "", err
	}
	if _, err := u.nodes.GetNode(ctx, workspaceID); err != nil {
		return "", nil
	}
	return workspaceID, nil
}
