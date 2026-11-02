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

// heardChat is a chat as the event that changed it left it: the read model the
// workspace's owner is resolved from lists it only once its projection runs.
type heardChat struct {
	chat domain.Chat
	gone bool
}

// over is rows, the chats of wsID, with h's own row in its place; nil h is rows.
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

// ownerMoved republishes each workspace a chat's bind gave or took its owner:
// the workspace row names its owning chat, which no workspace event carries.
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

// placedChat is chat at its Tree placement: the aggregate no longer carries it,
// and a fork's workspace row is drawn where its owning chat is.
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

// RepublishOwnedBy republishes the workspace chatID owns once the chat moved to
// p: a fork's row is drawn where its owning chat is.
func (c *Container) RepublishOwnedBy(ctx context.Context, chatID string, p tree.Placement) {
	chat, err := c.AgentChat.GetChat(ctx, chatID)
	if err != nil || !chat.OwnsWorkspace || chat.WorkspaceID == "" {
		return
	}
	chat.ParentID, chat.Order = p.ParentID, p.Order
	c.republishHearing(ctx, chat.WorkspaceID, &heardChat{chat: chat})
}

// ownerForgotten republishes the workspace a deleted owning chat leaves.
func (c *Container) ownerForgotten(ctx context.Context, chat domain.Chat) {
	if chat.OwnsWorkspace && chat.WorkspaceID != "" {
		c.republishHearing(ctx, chat.WorkspaceID, &heardChat{chat: chat, gone: true})
	}
}

// republishHearing is Republish with heard folded over the workspace's chats.
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

// revFrame is a workspace event's row and the rev taken before it was read.
type revFrame struct {
	row dto.WorkspaceDTO
	rev int64
}

func (c *Container) revFrame(ctx context.Context, ws domain.Workspace) revFrame {
	rev := c.nextRev()
	return revFrame{row: c.enrichFrame(ctx, ws), rev: rev}
}

// nextRev is a rev above every row read before it. Each workspace row, an
// event's or a republish's, is read after taking one, so a read that a later
// one overtook carries the lower rev and the hub drops it.
func (c *Container) nextRev() int64 {
	return c.revs.Add(1)
}
