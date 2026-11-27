package tree

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
)

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

func (u *chatFolderUsecase) DiscardOwningChat(
	ctx context.Context,
	chatID string,
) error {
	if err := u.agent.PurgeChat(ctx, chatID); err != nil {
		return fmt.Errorf("agent chat folder: discard owning chat %s: %w", chatID, err)
	}
	return nil
}

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
