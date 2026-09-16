package tree

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// createImportedWorktreeChat is CreateChat's WorktreeImport branch, and it is
// deliberately the SAME shape as createOwnWorktreeChat (chats.go): mint the
// chat, place it, and only then attach a worktree to it.
//
// Import used to be the one worktree-provisioning path that did none of this.
// It created a workspace row straight from a discovered git branch, in another
// usecase, with no chat minted in the same breath — so a branch could land on
// disk owned by nothing, addressable by nothing, and visible nowhere (spec §0).
// Reusing this scaffold is what makes that unrepresentable rather than merely
// reconciled after the fact: the chat exists BEFORE the workspace does, and a
// failure anywhere after the mint takes the chat back out.
func (u *chatFolderUsecase) createImportedWorktreeChat(
	ctx context.Context,
	providerID string,
	parentID string,
	spec ImportSpec,
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
	chatID, err := u.agent.MintChat(ctx, "")
	if err != nil {
		return "", domain.Workspace{}, "", fmt.Errorf("agent chat folder: import chat: %w", err)
	}
	// Filed from a global count of its level rather than through placeChat: an
	// imported row is an OWNING row, and it leaves the workspace-less scope
	// placeChat plans against as soon as its worktree is attached. See
	// placeOwningRow (owning_chat.go).
	if pErr := u.placeOwningRow(ctx, chatID, parentID); pErr != nil {
		return "", domain.Workspace{}, "", u.discard(ctx, chatID, pErr)
	}
	ws, runnerID, err := u.agent.SpawnChatWithImportedWorktree(ctx, chatID, providerID, spec)
	if err != nil {
		return "", domain.Workspace{}, "", u.discard(ctx, chatID, err)
	}
	return chatID, ws, runnerID, nil
}

// ImportBranchAsChat is createImportedWorktreeChat as a BATCH importer needs
// it: no provider (so no vendor CLI is launched for a row nobody has opened),
// no explicit chat parent (the placement follows the spec's git lineage), and
// the workspace id handed straight back.
//
// The id comes from the create itself rather than from a read-back of the chat
// that now owns it, and that is deliberate: the chat read model is an
// asynchronous projection, so a lookup taken immediately after this write can
// still be serving the row as it stood before the workspace was attached.
func (u *chatFolderUsecase) ImportBranchAsChat(
	ctx context.Context,
	spec ImportSpec,
) (string, string, error) {
	chatID, ws, _, err := u.createImportedWorktreeChat(ctx, "", "", spec)
	if err != nil {
		return "", "", err
	}
	return chatID, ws.ID, nil
}

// importPlacement answers where an imported row is BORN, from the only thing a
// batch importer actually knows: the git lineage.
//
// An explicit parentID always wins — that is a caller naming a chat outright.
// Otherwise the spec's ParentWorkspaceID IS the placement id (see
// owningChatOf): a batch import resolves its parents as WORKSPACES (a PR base
// branch is a branch, not a conversation), and every workspace's own
// Node{Kind:workspace} row shares the SAME sibling-order space a chat's
// placement does, so no translation is needed at all.
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
