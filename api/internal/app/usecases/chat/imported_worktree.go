package chat

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// SpawnChatWithImportedWorktree fills chatID's empty workspace slot with an EXISTING
// branch and opens providerID on it (OpenChat). Unlike the fork path, spec already names
// the fork parent, so no walk is taken. An empty providerID opens nothing (batch import).
func (u *Usecase) SpawnChatWithImportedWorktree(
	ctx context.Context,
	chatID string,
	providerID string,
	spec tree.ImportSpec,
) (domain.Workspace, string, error) {
	ws, err := u.worktree.CreateImportedWorkspace(ctx, spec)
	if err != nil {
		return domain.Workspace{}, "", fmt.Errorf("import chat: create workspace: %w", err)
	}
	if _, err := u.chats.SetWorkspace(ctx, chatID, ws.ID); err != nil {
		return domain.Workspace{}, "", u.discardUnclaimedWorkspace(ctx, ws.ID,
			fmt.Errorf("import chat: set workspace: %w", err))
	}
	if providerID == "" {
		return ws, "", nil
	}
	runnerID, err := u.runners.OpenChat(ctx, chatID, providerID)
	if err != nil {
		return domain.Workspace{}, "", u.discardMintedWorkspace(ctx, chatID, ws.ID,
			fmt.Errorf("import chat: open: %w", err))
	}
	return ws, runnerID, nil
}

// AttachWorkspace points an already-minted chat at the workspace it owns: the bare
// slot write for creation paths that build their own workspace.
func (u *Usecase) AttachWorkspace(
	ctx context.Context,
	chatID string,
	workspaceID string,
) error {
	if _, err := u.chats.SetWorkspace(ctx, chatID, workspaceID); err != nil {
		return fmt.Errorf("agent: attach workspace %s to chat %s: %w", workspaceID, chatID, err)
	}
	return nil
}
