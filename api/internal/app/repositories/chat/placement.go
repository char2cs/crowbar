package chat

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

// Placements is where each chat's own placement lives (the Tree aggregate);
// the chat aggregate no longer stores it, and this repository reads it back
// onto every chat it returns.
type Placements interface {
	ChatPlacement(ctx context.Context, chatID string) (tree.Placement, error)
	ChatPlacements(ctx context.Context) (map[string]tree.Placement, error)
	SetChatPlacement(ctx context.Context, chatID, workspaceID string, p tree.Placement) error
	SetChatOrder(ctx context.Context, chatID, workspaceID string, order int) error
	// AdoptChat files chatID under its workspace's project once it has one.
	AdoptChat(ctx context.Context, chatID, workspaceID string) error
	DropChat(ctx context.Context, chatID string) error
}

func (r *eventSourced) SetPlacement(
	ctx context.Context,
	chatID string,
	parentID string,
	order int,
) (domain.Chat, error) {
	chat, err := r.LoadChat(ctx, chatID)
	if errors.Is(err, ErrNotFound) {
		return domain.Chat{}, fmt.Errorf("agentchat: set placement: no chat: %w", asynxModels.ErrValidation)
	}
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: set placement: %w", err)
	}
	p := tree.Placement{ParentID: parentID, Order: order}
	if err := r.placements.SetChatPlacement(ctx, chatID, chat.WorkspaceID, p); err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: set placement: %w", err)
	}
	chat.ParentID, chat.Order = parentID, order
	return chat, nil
}

// SetOrder reports a chat already gone as problem.ErrNotFound, apart from a
// caller's mistake: a densify plans a level from one snapshot and a chat a
// concurrent delete purged needs no order any more.
func (r *eventSourced) SetOrder(
	ctx context.Context,
	chatID string,
	order int,
) (domain.Chat, error) {
	chat, err := r.LoadChat(ctx, chatID)
	if errors.Is(err, ErrNotFound) {
		return domain.Chat{}, fmt.Errorf("agentchat: set order: %w", problem.ErrNotFound)
	}
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: set order: %w", err)
	}
	if err := r.placements.SetChatOrder(ctx, chatID, chat.WorkspaceID, order); err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: set order: %w", err)
	}
	chat.Order = order
	return chat, nil
}

// placed is chat with its placement read back on.
func (r *eventSourced) placed(ctx context.Context, chat domain.Chat) (domain.Chat, error) {
	p, err := r.placements.ChatPlacement(ctx, chat.ID)
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: placement of %s: %w", chat.ID, err)
	}
	chat.ParentID, chat.Order = p.ParentID, p.Order
	return chat, nil
}

// placedAll is placed for a list, in one read of every placement.
func (r *eventSourced) placedAll(ctx context.Context, chats []domain.Chat) ([]domain.Chat, error) {
	all, err := r.placements.ChatPlacements(ctx)
	if err != nil {
		return nil, fmt.Errorf("agentchat: placements: %w", err)
	}
	for i := range chats {
		p := all[chats[i].ID]
		chats[i].ParentID, chats[i].Order = p.ParentID, p.Order
	}
	return chats, nil
}

// placedWatch puts each event's chat in its place before it is announced, so
// a frame never draws a chat at the root it is not in.
func (r *eventSourced) placedWatch(ctx context.Context, watch WatchFunc) WatchFunc {
	if watch == nil {
		return nil
	}
	ctx = context.WithoutCancel(ctx)
	return func(e ChatEvent) {
		if !e.Forgotten {
			p, err := r.placements.ChatPlacement(ctx, e.ChatID)
			if err != nil {
				slog.WarnContext(ctx, "agentchat: placement of an announced chat", "chat_id", e.ChatID, "err", err)
			}
			e.Chat.ParentID, e.Chat.Order = p.ParentID, p.Order
		}
		watch(e)
	}
}
