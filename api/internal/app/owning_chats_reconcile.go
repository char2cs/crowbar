package app

import (
	"context"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// owningChatMinter is the chat-first saga's own verbs (the Chats-panel tree
// usecase): mint and place a chat, point it at its workspace, take it back.
type owningChatMinter interface {
	MintOwningChat(ctx context.Context, parentWorkspaceID string) (string, error)
	AttachOwningWorkspace(ctx context.Context, chatID string, ws domain.Workspace) error
	DiscardOwningChat(ctx context.Context, chatID string) error
}

// reconcileOwningChats is the boot half of invariant D4: every live workspace
// has exactly one owning chat, whatever step of its creation a crash
// interrupted.
//
// Creation mints the chat, writes the workspace, then attaches the two; a crash
// between the last two leaves a workspace no chat owns — unaddressable, since
// every worktree surface is chat-keyed. That gap used to be closed on READ, by
// minting an owner inside a GET (EnsureOwner). It is closed here, once, at boot,
// before anything is served, and reads stay reads.
//
// One pass over the workspace and chat read models; per-row failures are
// logged, never fail boot.
func reconcileOwningChats(
	ctx context.Context,
	workspaces []domain.Workspace,
	chats []domain.Chat,
	minter owningChatMinter,
) {
	owned := make(map[string]bool, len(chats))
	for _, c := range chats {
		if c.OwnsWorkspace && c.WorkspaceID != "" {
			owned[c.WorkspaceID] = true
		}
	}
	for _, ws := range workspaces {
		if ws.Status == domain.WorkspaceStatusDeleted || owned[ws.ID] {
			continue
		}
		chatID, err := minter.MintOwningChat(ctx, ws.ParentID)
		if err != nil {
			slog.ErrorContext(ctx, "app: boot: mint the owning chat of an unowned workspace",
				"workspace_id", ws.ID, "err", err)
			continue
		}
		if err := minter.AttachOwningWorkspace(ctx, chatID, ws); err != nil {
			slog.ErrorContext(ctx, "app: boot: attach an owning chat to its workspace",
				"workspace_id", ws.ID, "chat_id", chatID, "err", err)
			if dErr := minter.DiscardOwningChat(ctx, chatID); dErr != nil {
				slog.ErrorContext(ctx, "app: boot: discard an owning chat a failed attach left",
					"chat_id", chatID, "err", dErr)
			}
			continue
		}
		slog.InfoContext(ctx, "app: boot: minted the owning chat an interrupted create never attached",
			"workspace_id", ws.ID, "chat_id", chatID)
	}
}
