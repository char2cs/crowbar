package tree

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// createImportedWorktreeChat is CreateChat's WorktreeImport branch: mint the chat, place it, then attach the
// worktree, so a branch never lands on disk owned by nothing. A failure after the mint returns the minted
// chat; the caller decides its fate (a single create discards it, a batch keeps it). open opens providerID
// on it; otherwise the provider is only recorded.
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
	// An imported row is an owning row, so it is filed by placeOwningRow, not placeChat.
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

// ImportBranchAsChat is createImportedWorktreeChat for a batch importer: the default provider recorded but
// not opened (no CLI for an unopened row), placement from the spec's git lineage, and the workspace id taken
// from the create itself, since a read-back from the async projection may predate the attach.
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

// importPlacement answers where an imported row is born: an explicit parentID, else the spec's parent
// workspace's own Node row, which shares the sibling space a chat's placement does.
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
