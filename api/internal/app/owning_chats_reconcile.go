package app

import (
	"context"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type owningChatMinter interface {
	MintOwningChat(ctx context.Context, chatID, parentWorkspaceID string) (string, error)
	AttachOwningWorkspace(ctx context.Context, chatID string, ws domain.Workspace) error
	DiscardOwningChat(ctx context.Context, chatID string) error
}

func reconcileOwningChats(
	ctx context.Context,
	workspaces []domain.Workspace,
	chats []domain.Chat,
	minter owningChatMinter,
) {
	owned := make(map[string]bool, len(chats))
	byWorkspace := make(map[string][]domain.Chat, len(chats))
	for _, c := range chats {
		if c.WorkspaceID == "" {
			continue
		}
		byWorkspace[c.WorkspaceID] = append(byWorkspace[c.WorkspaceID], c)
		if c.OwnsWorkspace {
			owned[c.WorkspaceID] = true
		}
	}
	for _, ws := range workspaces {
		if ws.Status == domain.WorkspaceStatusDeleted || owned[ws.ID] {
			continue
		}
		if owner, ok := legacyOwner(byWorkspace[ws.ID], sharedGround(ws)); ok {
			recordLegacyOwner(ctx, minter, owner.ID, ws)
			continue
		}
		mintOwningChat(ctx, minter, ws)
	}
}

func recordLegacyOwner(
	ctx context.Context,
	minter owningChatMinter,
	chatID string,
	ws domain.Workspace,
) {
	if err := minter.AttachOwningWorkspace(ctx, chatID, ws); err != nil {
		slog.ErrorContext(ctx, "app: boot: record the legacy owning chat of a workspace",
			"workspace_id", ws.ID, "chat_id", chatID, "err", err)
		return
	}
	slog.InfoContext(ctx, "app: boot: recorded the legacy owning chat of a workspace",
		"workspace_id", ws.ID, "chat_id", chatID)
}

func mintOwningChat(
	ctx context.Context,
	minter owningChatMinter,
	ws domain.Workspace,
) {
	chatID, err := minter.MintOwningChat(ctx, "", ws.ParentID)
	if err != nil {
		slog.ErrorContext(ctx, "app: boot: mint the owning chat of an unowned workspace",
			"workspace_id", ws.ID, "err", err)
		return
	}
	if err := minter.AttachOwningWorkspace(ctx, chatID, ws); err != nil {
		slog.ErrorContext(ctx, "app: boot: attach an owning chat to its workspace",
			"workspace_id", ws.ID, "chat_id", chatID, "err", err)
		if dErr := minter.DiscardOwningChat(ctx, chatID); dErr != nil {
			slog.ErrorContext(ctx, "app: boot: discard an owning chat a failed attach left",
				"chat_id", chatID, "err", dErr)
		}
		return
	}
	slog.InfoContext(ctx, "app: boot: minted the owning chat an interrupted create never attached",
		"workspace_id", ws.ID, "chat_id", chatID)
}

func sharedGround(ws domain.Workspace) bool {
	return ws.IsDefault || ws.RepoID == "" || ws.Status == domain.WorkspaceStatusLocked
}

func legacyOwner(
	rows []domain.Chat,
	shared bool,
) (domain.Chat, bool) {
	sameWorkspace := make(map[string]bool, len(rows))
	for _, row := range rows {
		sameWorkspace[row.ID] = true
	}
	var owner domain.Chat
	found := false
	for _, row := range rows {
		if !legacyCandidate(row, sameWorkspace, shared) {
			continue
		}
		if !found || !preferredLegacyOwner(owner, row) {
			owner, found = row, true
		}
	}
	return owner, found
}

func legacyCandidate(
	row domain.Chat,
	sameWorkspace map[string]bool,
	shared bool,
) bool {
	switch {
	case row.Type == domain.ChatTypeBranch:
		return true
	case row.WorkspaceID != "" && row.ParentID == row.WorkspaceID:
		return false
	case row.ParentID != "" && sameWorkspace[row.ParentID]:
		return false
	case shared && (row.Title != "" || row.TitleLocked):
		return false
	default:
		return true
	}
}

func preferredLegacyOwner(
	held domain.Chat,
	challenger domain.Chat,
) bool {
	if (held.Type == domain.ChatTypeBranch) != (challenger.Type == domain.ChatTypeBranch) {
		return held.Type == domain.ChatTypeBranch
	}
	if c := held.CreatedAt.Compare(challenger.CreatedAt); c != 0 {
		return c < 0
	}
	return held.ID < challenger.ID
}
