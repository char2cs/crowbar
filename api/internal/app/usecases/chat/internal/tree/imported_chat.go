package tree

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func (u *chatFolderUsecase) createImportedWorktreeChat(
	ctx context.Context,
	chatID string,
	providerID string,
	parentID string,
	spec ImportSpec,
	open bool,
) (string, domain.Workspace, string, error) {
	parentID, err := u.importPlacement(ctx, parentID, spec)
	if err != nil {
		return "", domain.Workspace{}, "", err
	}
	if parentID != "" {
		if err := u.checkNewChatParent(ctx, "", parentID, true); err != nil {
			return "", domain.Workspace{}, "", err
		}
	}
	chatID, err = u.agent.MintChat(ctx, chatID, "", providerID, "")
	if err != nil {
		return "", domain.Workspace{}, "", fmt.Errorf("agent chat folder: import chat: %w", err)
	}
	if pErr := u.placeOwningRow(ctx, chatID, parentID); pErr != nil {
		return chatID, domain.Workspace{}, "", pErr
	}
	opened := ""
	if open {
		opened = providerID
	}
	ws, runnerID, err := u.agent.SpawnChatWithImportedWorktree(ctx, chatID, opened, spec)
	if err != nil {
		return chatID, domain.Workspace{}, "", err
	}
	return chatID, ws, runnerID, nil
}

func (u *chatFolderUsecase) ImportBranchAsChat(
	ctx context.Context,
	chatID string,
	spec ImportSpec,
) (string, string, error) {
	providerID, err := u.agent.DefaultProviderID(ctx)
	if err != nil {
		return "", "", fmt.Errorf("agent chat folder: import chat: default provider: %w", err)
	}
	minted, ws, _, err := u.createImportedWorktreeChat(ctx, chatID, providerID, "", spec, false)
	return minted, ws.ID, err
}

func (u *chatFolderUsecase) importPlacement(
	ctx context.Context,
	parentID string,
	spec ImportSpec,
) (string, error) {
	if parentID != "" {
		return parentID, nil
	}
	return u.owningChatOf(ctx, spec.ParentWorkspaceID)
}
