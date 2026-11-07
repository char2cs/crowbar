package chat

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
)

// SpawnChatWithOwnWorktree fills a just-minted chat's empty workspace slot with a
// fresh worktree forked from its resolved fork parent and opens providerID on it
// (OpenChat: a CLI now only on the terminal).
func (u *Usecase) SpawnChatWithOwnWorktree(
	ctx context.Context,
	chatID string,
	providerID string,
	branch string,
) (string, string, error) {
	forkParentID, ok, err := tree.ResolveForkParent(ctx, u.chats, u.folders, u.nodes, u.repoRoots, chatID)
	if err != nil {
		return "", "", fmt.Errorf("create chat: resolve fork parent: %w", err)
	}
	if !ok {
		return "", "", fmt.Errorf("create chat %s: %w", chatID, ErrNoForkParent)
	}
	ws, err := u.worktree.CreateChildWorkspace(ctx, forkParentID, branch)
	if err != nil {
		return "", "", fmt.Errorf("create chat: create workspace: %w", err)
	}
	if _, err := u.chats.SetWorkspace(ctx, chatID, ws.ID); err != nil {
		return "", "", u.discardUnclaimedWorkspace(ctx, ws.ID,
			fmt.Errorf("create chat: set workspace: %w", err))
	}
	runnerID, err := u.runners.OpenChat(ctx, chatID, providerID)
	if err != nil {
		return "", "", u.discardMintedWorkspace(ctx, chatID, ws.ID,
			fmt.Errorf("create chat: open: %w", err))
	}
	return ws.ID, runnerID, nil
}

// discardUnclaimedWorkspace discards a minted workspace no row was pointed at.
// Best-effort and logged; the caller gets cause, the failure that actually happened.
func (u *Usecase) discardUnclaimedWorkspace(
	ctx context.Context,
	workspaceID string,
	cause error,
) error {
	if err := u.worktree.DiscardChildWorkspace(ctx, workspaceID); err != nil {
		slog.WarnContext(ctx, "agent: create chat: discard the workspace nothing came to own",
			"workspace_id", workspaceID, "err", err)
	}
	return cause
}

// discardMintedWorkspace discards a workspace chatID already points at, even if the
// clear fails: the caller purges chatID on error, so a kept
// workspace would be unreachable. The clear only narrows the window a reader sees it.
func (u *Usecase) discardMintedWorkspace(
	ctx context.Context,
	chatID string,
	workspaceID string,
	cause error,
) error {
	if _, err := u.chats.SetWorkspace(ctx, chatID, ""); err != nil {
		slog.WarnContext(ctx, "agent: create chat: clear the workspace slot before discard (best-effort, continuing)",
			"chat_id", chatID, "workspace_id", workspaceID, "err", err)
	}
	if err := u.worktree.DiscardChildWorkspace(ctx, workspaceID); err != nil {
		slog.WarnContext(ctx, "agent: create chat: discard the workspace nothing came to own",
			"chat_id", chatID, "workspace_id", workspaceID, "err", err)
	}
	return cause
}
