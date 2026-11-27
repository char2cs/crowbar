package repositories

import (
	"context"
	"errors"
	"log/slog"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

type heardChat struct {
	chat domain.Chat
	gone bool
}

func (h *heardChat) over(rows []domain.Chat, wsID string) []domain.Chat {
	if h == nil {
		return rows
	}
	out := make([]domain.Chat, 0, len(rows)+1)
	for _, row := range rows {
		if row.ID != h.chat.ID {
			out = append(out, row)
		}
	}
	if !h.gone && h.chat.WorkspaceID == wsID {
		out = append(out, h.chat)
	}
	return out
}

func (c *Container) ownerMoved(ctx context.Context, evt asynxModels.Event[domain.Chat]) {
	before, after := evt.PreviousAggregate, evt.Aggregate
	heard := &heardChat{chat: c.placedChat(ctx, after)}
	if before.OwnsWorkspace && before.WorkspaceID != "" && before.WorkspaceID != after.WorkspaceID {
		c.republishHearing(ctx, before.WorkspaceID, heard)
	}
	if after.OwnsWorkspace && after.WorkspaceID != "" {
		c.republishHearing(ctx, after.WorkspaceID, heard)
	}
}

func (c *Container) placedChat(ctx context.Context, chat domain.Chat) domain.Chat {
	if c.Placement == nil {
		return chat
	}
	p, err := c.Placement.ChatPlacement(ctx, chat.ID)
	if err != nil {
		slog.WarnContext(ctx, "repositories: republish owner: chat placement", "chat_id", chat.ID, "err", err)
	}
	chat.ParentID, chat.Order = p.ParentID, p.Order
	return chat
}

func (c *Container) atItsNode(ctx context.Context, chat domain.Chat) domain.Chat {
	if c.Placement == nil {
		return chat
	}
	n, err := c.Placement.GetNode(ctx, chat.ID)
	if err != nil {
		if !errors.Is(err, tree.ErrUnknownNode) {
			slog.WarnContext(ctx, "repositories: owner placement: chat node", "chat_id", chat.ID, "err", err)
		}
		return chat
	}
	chat.ParentID, chat.Order = n.ParentID, n.Order
	return chat
}

func (c *Container) RepublishOwnedBy(ctx context.Context, chatID string, p tree.Placement) {
	chat, err := c.AgentChat.GetChat(ctx, chatID)
	if err != nil || !chat.OwnsWorkspace || chat.WorkspaceID == "" {
		return
	}
	chat.ParentID, chat.Order = p.ParentID, p.Order
	c.republishHearing(ctx, chat.WorkspaceID, &heardChat{chat: chat})
}

func (c *Container) ownerForgotten(ctx context.Context, chat domain.Chat) {
	if chat.OwnsWorkspace && chat.WorkspaceID != "" {
		c.republishHearing(ctx, chat.WorkspaceID, &heardChat{chat: chat, gone: true})
	}
}

func (c *Container) republishHearing(ctx context.Context, wsID string, heard *heardChat) {
	rev := c.nextRev()
	ws, err := c.Workspace.Get(ctx, wsID)
	if err != nil {
		if !errors.Is(err, problem.ErrNotFound) {
			slog.WarnContext(ctx, "repositories: republish owner: get workspace", "workspace_id", wsID, "err", err)
		}
		return
	}
	c.publishWorkspace(c.enrichFrameHearing(ctx, ws, heard), rev)
}

type revFrame struct {
	row dto.WorkspaceDTO
	rev int64
}

func (c *Container) revFrame(ctx context.Context, ws domain.Workspace) revFrame {
	rev := c.nextRev()
	return revFrame{row: c.enrichFrame(ctx, ws), rev: rev}
}

func (c *Container) nextRev() int64 {
	return c.revs.Add(1)
}
