package repositories

import (
	"context"
	"errors"
	"log/slog"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
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
	heard := &heardChat{chat: after}
	if before.OwnsWorkspace && before.WorkspaceID != "" && before.WorkspaceID != after.WorkspaceID {
		c.republishHearing(ctx, before.WorkspaceID, heard)
	}
	if after.OwnsWorkspace && after.WorkspaceID != "" {
		c.republishHearing(ctx, after.WorkspaceID, heard)
	}
}

// ownerForgotten republishes the workspace a deleted owning chat leaves.
func (c *Container) ownerForgotten(ctx context.Context, chat domain.Chat) {
	if chat.OwnsWorkspace && chat.WorkspaceID != "" {
		c.republishHearing(ctx, chat.WorkspaceID, &heardChat{chat: chat, gone: true})
	}
}

// republishHearing is Republish with heard folded over the workspace's chats.
// The row goes out at the version announced before it was read, so a read that
// a newer event's frame overtook, or the tombstone's, is dropped as stale.
func (c *Container) republishHearing(ctx context.Context, wsID string, heard *heardChat) {
	rev := max(c.workspaceVersion(wsID), 1)
	ws, err := c.Workspace.Get(ctx, wsID)
	if err != nil {
		if !errors.Is(err, problem.ErrNotFound) {
			slog.WarnContext(ctx, "repositories: republish owner: get workspace", "workspace_id", wsID, "err", err)
		}
		return
	}
	c.publishWorkspace(c.enrichFrameHearing(ctx, ws, heard), rev)
}
